// Package webhook is the HTTP-webhook trigger listener (path + optional secret + 10MB body cap).
//
// Package webhook 是 HTTP webhook 触发器（路径 + 可选 secret + 10MB body 上限）。
package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

const MaxBodyBytes = 10 * 1024 * 1024

// OnFireFunc fires on each accepted webhook request; caller wires to scheduler.StartRun.
//
// OnFireFunc 每次 accept 的 webhook 请求触发；调用方接 scheduler.StartRun。
type OnFireFunc func(workflowID, nodeID string, input map[string]any)

// Listener manages webhook registrations against one shared http.ServeMux.
//
// Listener 管理 webhook 注册，与外部共享一个 http.ServeMux。
type Listener struct {
	mu       sync.Mutex
	mux      *http.ServeMux
	registry map[string]registration
	keys     map[string]string
	lastFire map[string]time.Time
	onFire   OnFireFunc
	log      *zap.Logger
}

type registration struct {
	WorkflowID string
	NodeID     string
	Method     string
	Secret     string
}

// New constructs a Listener bound to the given mux.
//
// New 构造 Listener 并绑定到给定 mux。
func New(mux *http.ServeMux, log *zap.Logger, onFire OnFireFunc) *Listener {
	return &Listener{
		mux:      mux,
		registry: make(map[string]registration),
		keys:     make(map[string]string),
		lastFire: make(map[string]time.Time),
		onFire:   onFire,
		log:      log.Named("trigger.webhook"),
	}
}

// Register mounts a webhook at /api/v1/webhooks/{workflowID}/{path}; conflicts → ErrPathConflict.
//
// Register 在 /api/v1/webhooks/{workflowID}/{path} 挂载 webhook；冲突返 ErrPathConflict。
func (l *Listener) Register(spec triggerdomain.Spec) error {
	subpath, _ := spec.Config["path"].(string)
	method, _ := spec.Config["method"].(string)
	secret, _ := spec.Config["secret"].(string)

	if subpath == "" {
		return fmt.Errorf("triggerwebhookinfra.Register: %w: empty path", triggerdomain.ErrPathConflict)
	}
	if method == "" {
		method = http.MethodPost
	}
	method = strings.ToUpper(method)

	full := webhookFullPath(spec.WorkflowID, subpath)
	key := spec.WorkflowID + "/" + spec.NodeID

	l.mu.Lock()
	defer l.mu.Unlock()

	if other, taken := l.registry[full]; taken && (other.WorkflowID != spec.WorkflowID || other.NodeID != spec.NodeID) {
		return fmt.Errorf("triggerwebhookinfra.Register: %w: %s already registered to %s/%s",
			triggerdomain.ErrPathConflict, full, other.WorkflowID, other.NodeID)
	}

	if oldPath, ok := l.keys[key]; ok && oldPath != full {
		// stdlib mux can't unmount; leftover route 404s via registry miss.
		// stdlib mux 不能 unmount，残留路由经 registry miss 自然 404。
		delete(l.registry, oldPath)
	}

	method = strings.ToUpper(method)
	reg := registration{
		WorkflowID: spec.WorkflowID,
		NodeID:     spec.NodeID,
		Method:     method,
		Secret:     secret,
	}
	if _, alreadyMounted := l.registry[full]; !alreadyMounted {
		l.mux.HandleFunc(full, l.handleWebhook(full))
	}
	l.registry[full] = reg
	l.keys[key] = full
	return nil
}

// Unregister removes a webhook (mux entry stays but starts 404'ing).
//
// Unregister 删 webhook（mux 上仍在但开始返 404）。
func (l *Listener) Unregister(workflowID, nodeID string) {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	if path, ok := l.keys[key]; ok {
		delete(l.registry, path)
		delete(l.keys, key)
	}
}

// State returns the runtime state for one trigger.
//
// State 返某 trigger 的运行时状态。
func (l *Listener) State(workflowID, nodeID string) triggerdomain.State {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	st := triggerdomain.State{
		WorkflowID: workflowID, NodeID: nodeID,
		Kind: triggerdomain.KindWebhook, Status: triggerdomain.StateIdle,
	}
	if _, ok := l.keys[key]; ok {
		st.Status = triggerdomain.StateActive
	}
	if last, ok := l.lastFire[key]; ok {
		t := last
		st.LastFiredAt = &t
	}
	return st
}

func (l *Listener) handleWebhook(fullPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		reg, ok := l.registry[fullPath]
		l.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}

		if !strings.EqualFold(r.Method, reg.Method) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if reg.Secret != "" {
			gotSecret := r.Header.Get("X-Webhook-Secret")
			if gotSecret == "" {
				gotSecret = r.URL.Query().Get("token")
			}
			if gotSecret != reg.Secret {
				http.Error(w, "secret mismatch", http.StatusUnauthorized)
				return
			}
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		if len(body) > MaxBodyBytes {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}

		input := map[string]any{
			"firedAt": time.Now(),
			"method":  r.Method,
			"path":    fullPath,
			"headers": flattenHeaders(r.Header),
		}
		if len(body) > 0 {
			var payload any
			if err := json.Unmarshal(body, &payload); err == nil {
				input["body"] = payload
			} else {
				input["bodyRaw"] = string(body)
			}
		}

		key := reg.WorkflowID + "/" + reg.NodeID
		l.mu.Lock()
		l.lastFire[key] = time.Now()
		l.mu.Unlock()

		// Fire async + recover so handler stays responsive on slow/panicking onFire.
		// 异步 fire + recover，handler 不被慢/panic 拖累。
		go func() {
			defer func() {
				if r := recover(); r != nil {
					l.log.Error("webhook onFire panic",
						zap.String("workflowID", reg.WorkflowID),
						zap.String("nodeID", reg.NodeID),
						zap.Any("recover", r))
				}
			}()
			l.onFire(reg.WorkflowID, reg.NodeID, input)
		}()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

// webhookFullPath builds the mux path for a workflow + subpath.
//
// webhookFullPath 拼 webhook 的 mux 路径。
func webhookFullPath(workflowID, subpath string) string {
	subpath = strings.TrimPrefix(subpath, "/")
	return "/api/v1/webhooks/" + workflowID + "/" + subpath
}
