// dev.go — HTTP handlers for the /dev/* route group, registered only when
// the server is started with --dev. Provides: static file serving for the
// integration console, a log SSE stream, a read-only SQL endpoint, and a
// YAML test-collection loader.
//
// dev.go — /dev/* 路由组的 HTTP handler，仅在 --dev 启动时注册。
// 提供：integration console 的静态文件服务、日志 SSE 流、只读 SQL 端点、
// YAML 测试集合加载器。
package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// DevHandler serves all /dev/* endpoints.
//
// DevHandler 提供所有 /dev/* 端点。
type DevHandler struct {
	db             *gorm.DB
	broadcaster    *loggerinfra.LogBroadcaster
	collectionsDir string
	integrationDir string
	port           int
	tools          []toolapp.Tool
	llmFactory     *llminfra.Factory
	log            *zap.Logger
}

// NewDevHandler wires DevHandler dependencies.
//
// NewDevHandler 装配 DevHandler 依赖。
func NewDevHandler(
	db *gorm.DB,
	broadcaster *loggerinfra.LogBroadcaster,
	collectionsDir, integrationDir string,
	port int,
	tools []toolapp.Tool,
	llmFactory *llminfra.Factory,
	log *zap.Logger,
) *DevHandler {
	return &DevHandler{
		db:             db,
		broadcaster:    broadcaster,
		collectionsDir: collectionsDir,
		integrationDir: integrationDir,
		port:           port,
		tools:          tools,
		llmFactory:     llmFactory,
		log:            log,
	}
}

// Register attaches dev routes. Specific method+path patterns take priority
// over the trailing-slash catch-all for static files.
//
// Register 挂载 dev 路由。带方法的精确路径模式优先于静态文件的通配路由。
func (h *DevHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /dev/logs", h.StreamLogs)
	mux.HandleFunc("POST /dev/sql", h.QuerySQL)
	mux.HandleFunc("GET /dev/collections", h.ListCollections)
	mux.HandleFunc("GET /dev/tools", h.ListTools)
	mux.HandleFunc("POST /dev/invoke", h.InvokeTool)
	// TE-9 Info tab data sources
	mux.HandleFunc("GET /dev/info", h.Info)
	mux.HandleFunc("GET /dev/forgify-home", h.ForgifyHome)
	// TE-4b mock LLM endpoints; nil-tolerant when --dev didn't wire
	// the llmFactory (shouldn't happen in practice — dev mode always
	// has it — but keeps the helper exit clean during refactor).
	// TE-4b mock LLM 端点；llmFactory 没接时 nil-tolerant。
	if h.llmFactory != nil {
		mux.HandleFunc("POST /dev/mock-llm/scripts", h.MockLLMPushScripts)
		mux.HandleFunc("GET /dev/mock-llm/queue", h.MockLLMQueue)
		mux.HandleFunc("DELETE /dev/mock-llm/scripts", h.MockLLMClear)
		mux.HandleFunc("GET /dev/mock-llm/last-prompt", h.MockLLMLastPrompt)
		// TE-5a LLM trace endpoint — recorder is set during --dev boot
		// (main.go calls llmFactory.SetTracer(...)). Nil-tolerant.
		// TE-5a LLM trace 端点——recorder 在 --dev boot 时设
		// （main.go 调 llmFactory.SetTracer）。容忍 nil。
		mux.HandleFunc("GET /dev/llm-trace", h.LLMTrace)
	}
	// Static files: /dev/static/style.css, /dev/static/js/app.js, etc.
	// no-cache so browser always fetches the latest version during dev.
	// no-cache 避免浏览器缓存旧版本文件。
	noCache := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			next.ServeHTTP(w, r)
		})
	}
	mux.Handle("/dev/static/", noCache(http.StripPrefix("/dev/static/", http.FileServer(http.Dir(h.integrationDir)))))
	// Catch-all: serve index.html for /dev/ and any unmatched sub-path.
	mux.HandleFunc("/dev/", h.ServeIndex)
}

// ── GET /dev/ ─────────────────────────────────────────────────────────────────

// ServeIndex serves tester.html for all /dev/* paths not matched by a more
// specific route. We read manually instead of http.ServeFile to avoid the
// trailing-slash redirect that ServeFile triggers when the URL ends with "/".
//
// ServeIndex 为所有未被更具体路由匹配的 /dev/* 路径提供 tester.html。
// 手动读取而非 http.ServeFile，避免 URL 以 "/" 结尾时 ServeFile 触发的重定向。
func (h *DevHandler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(h.integrationDir, "tester.html"))
	if err != nil {
		http.Error(w, "tester.html not found — check --integration-dir", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Late write error means client disconnected mid-response; nothing
	// useful to do (status already sent). Intentionally ignored.
	// 写出错通常是客户端中途断开，状态码已发出无可挽回，故意忽略。
	_, _ = w.Write(data)
}

// ── GET /dev/logs ─────────────────────────────────────────────────────────────

// StreamLogs streams backend log entries as SSE. On connection it replays
// the ring buffer, then subscribes to new entries. SSE plumbing (headers,
// keep-alive, ctx-driven shutdown) is delegated to responsehttpapi.StreamSSE.
//
// StreamLogs 把后端日志条目以 SSE 形式推送。连接时先回放环形缓冲区，
// 然后订阅新条目。SSE 管线（header、keep-alive、ctx 驱动退出）委托给
// responsehttpapi.StreamSSE。
func (h *DevHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	ch, unsub := h.broadcaster.Subscribe()
	defer unsub()

	responsehttpapi.StreamSSE(w, r,
		func(out io.Writer) {
			for _, entry := range h.broadcaster.Ring() {
				fmt.Fprintf(out, "event: log\ndata: %s\n\n", entry)
			}
		},
		ch,
		func(out io.Writer, data []byte) error {
			_, err := fmt.Fprintf(out, "event: log\ndata: %s\n\n", data)
			return err
		},
	)
}

// ── POST /dev/sql ─────────────────────────────────────────────────────────────

type sqlRequest struct {
	SQL string `json:"sql"`
}

type sqlResponse struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

type sqlErrorResponse struct {
	Error string `json:"error"`
}

// QuerySQL executes a read-only SQL SELECT and returns columns + rows.
// Non-SELECT statements are rejected.
//
// QuerySQL 执行只读 SQL SELECT，返回列名和行数据。
// 非 SELECT 语句直接拒绝。
func (h *DevHandler) QuerySQL(w http.ResponseWriter, r *http.Request) {
	var req sqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDevJSON(w, http.StatusBadRequest, sqlErrorResponse{Error: "invalid JSON"})
		return
	}

	upper := strings.ToUpper(strings.TrimSpace(req.SQL))
	if !strings.HasPrefix(upper, "SELECT") {
		writeDevJSON(w, http.StatusBadRequest, sqlErrorResponse{Error: "只允许 SELECT 语句 / only SELECT statements allowed"})
		return
	}

	sqlDB, err := h.db.DB()
	if err != nil {
		writeDevJSON(w, http.StatusInternalServerError, sqlErrorResponse{Error: err.Error()})
		return
	}

	rows, err := sqlDB.QueryContext(r.Context(), req.SQL)
	if err != nil {
		writeDevJSON(w, http.StatusBadRequest, sqlErrorResponse{Error: err.Error()})
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		writeDevJSON(w, http.StatusInternalServerError, sqlErrorResponse{Error: err.Error()})
		return
	}

	var result [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		// Convert []byte values to string for JSON readability.
		// 把 []byte 转成 string，JSON 展示更可读。
		row := make([]any, len(vals))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		result = append(result, row)
	}

	writeDevJSON(w, http.StatusOK, sqlResponse{Columns: cols, Rows: result})
}

// ── GET /dev/collections ──────────────────────────────────────────────────────

// Collection mirrors the YAML structure of integration/collections/*.yaml.
//
// Collection 镜像 integration/collections/*.yaml 的 YAML 结构。
type Collection struct {
	Name        string       `yaml:"name"        json:"name"`
	Description string       `yaml:"description" json:"description"`
	Steps       []StepConfig `yaml:"steps"       json:"steps"`
}

// StepConfig is one HTTP step in a collection.
//
// StepConfig 是集合中的一个 HTTP 步骤。
type StepConfig struct {
	Name    string            `yaml:"name"    json:"name"`
	Method  string            `yaml:"method"  json:"method"`
	Path    string            `yaml:"path"    json:"path"`
	Body    map[string]any    `yaml:"body"    json:"body,omitempty"`
	Expect  *ExpectConfig     `yaml:"expect"  json:"expect,omitempty"`
	Capture map[string]string `yaml:"capture" json:"capture,omitempty"`
}

// ExpectConfig defines assertions on an HTTP response.
//
// ExpectConfig 定义对 HTTP 响应的断言。
type ExpectConfig struct {
	Status int `yaml:"status" json:"status"`
}

// ListCollections reads all *.yaml files from testend/collections and returns
// the parsed collection definitions as JSON. The frontend runner executes
// the steps — the backend only loads and parses.
//
// ListCollections 从 testend/collections 读取所有 *.yaml 文件，把解析好的
// 集合定义以 JSON 返回。步骤由前端执行，后端只负责加载和解析。
func (h *DevHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.collectionsDir)
	if err != nil {
		writeDevJSON(w, http.StatusOK, []Collection{})
		return
	}

	var cols []Collection
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.collectionsDir, e.Name()))
		if err != nil {
			h.log.Warn("dev: read collection file", zap.String("file", e.Name()), zap.Error(err))
			continue
		}
		var col Collection
		if err := yaml.Unmarshal(data, &col); err != nil {
			h.log.Warn("dev: parse collection yaml", zap.String("file", e.Name()), zap.Error(err))
			continue
		}
		cols = append(cols, col)
	}
	if cols == nil {
		cols = []Collection{}
	}
	writeDevJSON(w, http.StatusOK, cols)
}

// ── GET /dev/tools ────────────────────────────────────────────────────────────

type devToolSummary struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ListTools returns the name and description of every system tool registered
// with the agent, so the testend invoke panel can populate its dropdown.
//
// ListTools 返回注册到 agent 的每个 system tool 的名称和描述，
// 供 testend invoke 面板填充下拉列表。
func (h *DevHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	out := make([]devToolSummary, 0, len(h.tools))
	for _, t := range h.tools {
		out = append(out, devToolSummary{Name: t.Name(), Desc: t.Description()})
	}
	writeDevJSON(w, http.StatusOK, out)
}

// ── POST /dev/invoke ──────────────────────────────────────────────────────────

type invokeRequest struct {
	Tool string `json:"tool"`
	Args string `json:"args"` // JSON-encoded arguments for the tool
}

type invokeResponse struct {
	Output    string `json:"output"`
	OK        bool   `json:"ok"`
	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`
}

// InvokeTool directly runs a named system tool with caller-supplied JSON args.
// Useful for smoke-testing tools without going through the LLM agent.
//
// InvokeTool 用调用方提供的 JSON 参数直接运行指定 system tool，
// 无需经过 LLM agent，方便冒烟测试。
func (h *DevHandler) InvokeTool(w http.ResponseWriter, r *http.Request) {
	var req invokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDevJSON(w, http.StatusBadRequest, invokeResponse{Error: "invalid JSON"})
		return
	}
	if req.Tool == "" {
		writeDevJSON(w, http.StatusBadRequest, invokeResponse{Error: "tool is required"})
		return
	}
	if req.Args == "" {
		req.Args = "{}"
	}

	ctx := r.Context()
	var target toolapp.Tool
	for _, t := range h.tools {
		if t.Name() == req.Tool {
			target = t
			break
		}
	}
	if target == nil {
		writeDevJSON(w, http.StatusNotFound, invokeResponse{Error: "tool not found: " + req.Tool})
		return
	}

	start := time.Now()
	output, err := target.Execute(ctx, req.Args)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		writeDevJSON(w, http.StatusOK, invokeResponse{Output: output, OK: false, ElapsedMs: elapsed, Error: err.Error()})
		return
	}
	writeDevJSON(w, http.StatusOK, invokeResponse{Output: output, OK: true, ElapsedMs: elapsed})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeDevJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
