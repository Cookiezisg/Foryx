// Package webhook is the HTTP-webhook source listener (path + optional secret/HMAC + 10MB
// body cap), keyed by triggerID and mounted at /api/v1/webhooks/{triggerID}/{path}. It fires
// once per accepted request with an empty dedupKey (each request is a distinct fire).
//
// Package webhook 是 HTTP webhook source listener（路径 + 可选 secret/HMAC + 10MB body 上限），
// 按 triggerID 键、挂在 /api/v1/webhooks/{triggerID}/{path}。每次 accept 的请求触发一次、dedupKey 空。
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	triggerinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
)

// Signature algorithms. Only hmac-sha256-hex is implemented (GitHub's X-Hub-Signature-256).
//
// 签名算法。当前只实现 hmac-sha256-hex（GitHub `X-Hub-Signature-256`）。
const (
	SignatureAlgoHMACSHA256Hex = "hmac-sha256-hex"
	DefaultHMACSignatureHeader = "X-Hub-Signature-256"
	HMACSignaturePrefix        = "sha256="
)

// Listener manages webhook registrations against one shared http.ServeMux.
//
// Listener 管理 webhook 注册，与外部共享一个 http.ServeMux。
type Listener struct {
	mu       sync.Mutex
	mux      *http.ServeMux
	registry map[string]registration // key: full mux path
	paths    map[string]string       // key: triggerID → full mux path
	report   triggerinfra.ReportFunc
	log      *zap.Logger
}

type registration struct {
	TriggerID       string
	Method          string
	Secret          string
	SignatureAlgo   string // empty = plain X-Webhook-Secret eq-check; hmac-sha256-hex = HMAC verify
	SignatureHeader string // header to read the signature from; empty + algo set → DefaultHMACSignatureHeader
}

// New constructs a Listener bound to the given mux.
//
// New 构造 Listener 并绑定给定 mux。
func New(mux *http.ServeMux, log *zap.Logger, report triggerinfra.ReportFunc) *Listener {
	return &Listener{
		mux:      mux,
		registry: make(map[string]registration),
		paths:    make(map[string]string),
		report:   report,
		log:      log.Named("trigger.webhook"),
	}
}

// Register mounts a webhook at /api/v1/webhooks/{triggerID}/{path}; a path owned by another
// trigger returns a conflict error.
//
// Register 在 /api/v1/webhooks/{triggerID}/{path} 挂载 webhook；路径被另一 trigger 占用则返冲突。
func (l *Listener) Register(triggerID string, _ string, config map[string]any) error {
	subpath, _ := config["path"].(string)
	method, _ := config["method"].(string)
	secret, _ := config["secret"].(string)
	sigAlgo, _ := config["signatureAlgo"].(string)
	sigHeader, _ := config["signatureHeader"].(string)

	if subpath == "" {
		return fmt.Errorf("webhook.Register %s: empty path", triggerID)
	}
	if sigAlgo != "" && sigAlgo != SignatureAlgoHMACSHA256Hex {
		return fmt.Errorf("webhook.Register %s: unsupported signatureAlgo %q (only %q)", triggerID, sigAlgo, SignatureAlgoHMACSHA256Hex)
	}
	if sigAlgo != "" && secret == "" {
		return fmt.Errorf("webhook.Register %s: signatureAlgo requires secret", triggerID)
	}
	if method == "" {
		method = http.MethodPost
	}
	method = strings.ToUpper(method)
	if sigAlgo != "" && sigHeader == "" {
		sigHeader = DefaultHMACSignatureHeader
	}

	full := webhookFullPath(triggerID, subpath)

	l.mu.Lock()
	defer l.mu.Unlock()
	if other, taken := l.registry[full]; taken && other.TriggerID != triggerID {
		return fmt.Errorf("webhook.Register %s: %s already registered to %s", triggerID, full, other.TriggerID)
	}
	if oldPath, ok := l.paths[triggerID]; ok && oldPath != full {
		// stdlib mux can't unmount; leftover route 404s via registry miss.
		// stdlib mux 不能 unmount，残留路由经 registry miss 自然 404。
		delete(l.registry, oldPath)
	}
	if _, mounted := l.registry[full]; !mounted {
		l.mux.HandleFunc(full, l.handleWebhook(full))
	}
	l.registry[full] = registration{TriggerID: triggerID, Method: method, Secret: secret, SignatureAlgo: sigAlgo, SignatureHeader: sigHeader}
	l.paths[triggerID] = full
	return nil
}

// Unregister removes a webhook (the mux entry stays but starts 404'ing via registry miss).
//
// Unregister 删 webhook（mux 上仍在但经 registry miss 开始返 404）。
func (l *Listener) Unregister(triggerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if path, ok := l.paths[triggerID]; ok {
		delete(l.registry, path)
		delete(l.paths, triggerID)
	}
}

// Start is a no-op — routes mount on Register against the shared mux.
//
// Start 是 no-op——路由在 Register 时挂到共享 mux。
func (l *Listener) Start() {}

// Stop is a no-op — the mux lifecycle is owned by the HTTP server, not this listener.
//
// Stop 是 no-op——mux 生命周期归 HTTP server，不归此 listener。
func (l *Listener) Stop() {}

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
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes()+1))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		if int64(len(body)) > maxBodyBytes() {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// HMAC mode verifies the signature header against hmac_sha256(body, secret); plain mode
		// compares X-Webhook-Secret / ?token= to the configured secret.
		// HMAC 模式按 hmac_sha256(body, secret) 验签；明文模式按 secret 直比。
		if reg.Secret != "" {
			switch reg.SignatureAlgo {
			case SignatureAlgoHMACSHA256Hex:
				if !verifyHMACSHA256Hex(body, []byte(reg.Secret), r.Header.Get(reg.SignatureHeader)) {
					http.Error(w, "signature mismatch", http.StatusUnauthorized)
					return
				}
			default:
				got := r.Header.Get("X-Webhook-Secret")
				if got == "" {
					got = r.URL.Query().Get("token")
				}
				if got != reg.Secret {
					http.Error(w, "secret mismatch", http.StatusUnauthorized)
					return
				}
			}
		}

		payload := map[string]any{
			"firedAt": time.Now(),
			"method":  r.Method,
			"path":    fullPath,
			"headers": flattenHeaders(r.Header),
		}
		if len(body) > 0 {
			var parsed any
			if err := json.Unmarshal(body, &parsed); err == nil {
				payload["body"] = parsed
			} else {
				payload["bodyRaw"] = string(body)
			}
		}

		// Dedup key: body hash + a minute bucket. A network-level retry of the SAME request
		// (seconds apart) collapses onto one Firing per workflow (idx_trf_dedup), while a
		// legitimately repeated identical payload later (next minute on) fires again — the
		// UNIQUE is forever, so the key must not be the bare hash.
		// 去重键：body 哈希 + 分钟桶。同一请求的网络级重试（秒级间隔）按 workflow 折叠成一条
		// Firing（idx_trf_dedup）；之后（下一分钟起）合法重复的相同 payload 照常触发——UNIQUE
		// 是永久的，键不能只是裸哈希。
		sum := sha256.Sum256(body)
		dedup := hex.EncodeToString(sum[:8]) + "|" + time.Now().UTC().Format("200601021504")

		// Fire async + recover so the handler stays responsive on a slow/panicking onFire.
		// 异步 fire + recover，handler 不被慢/panic 拖累。
		triggerID := reg.TriggerID
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					l.log.Error("webhook report panic", zap.String("triggerID", triggerID), zap.Any("recover", rec))
				}
			}()
			l.report(triggerID, triggerinfra.Activity{Fired: true, Payload: payload, DedupKey: dedup})
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

func webhookFullPath(triggerID, subpath string) string {
	return "/api/v1/webhooks/" + triggerID + "/" + strings.TrimPrefix(subpath, "/")
}

// verifyHMACSHA256Hex constant-time compares the GitHub-style `sha256=<hex>` (or bare hex)
// signature against hmac_sha256(body, secret). Empty header → false; auto-strips the prefix.
//
// verifyHMACSHA256Hex 常量时间对比 `sha256=<hex>` 签名与 hmac_sha256(body, secret)；空 header → false。
func verifyHMACSHA256Hex(body, secret []byte, headerVal string) bool {
	if headerVal == "" {
		return false
	}
	gotBytes, err := hex.DecodeString(strings.TrimPrefix(headerVal, HMACSignaturePrefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(gotBytes, mac.Sum(nil))
}

var _ triggerinfra.Listener = (*Listener)(nil)

// maxBodyBytes reads the live webhook body cap (limits.Guards.WebhookBodyMaxMB).
//
// maxBodyBytes 读活动 webhook body 上限（limits.Guards.WebhookBodyMaxMB）。
func maxBodyBytes() int64 { return int64(limitspkg.Current().Guards.WebhookBodyMaxMB) << 20 }
