// Package handlers — /dev/* route group, registered only under --dev.
//
// Package handlers — /dev/* 路由组,仅 --dev 启动注册。
package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// RouteEntry mirrors router.Route at the handler-package boundary because
// handlers/ cannot import router/ (would create an import cycle).
//
// RouteEntry 在 handler 包侧镜像 router.Route;handlers 不能反向 import router,
// 此类型让 RouteSource 接口可以独立于 router 包定义。
type RouteEntry struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// RouteSource is satisfied by an adapter in the router package; kept as
// interface here to avoid the handlers → router import cycle.
//
// RouteSource 由 router 包内适配器实现;此处用接口避免 handlers→router 循环依赖。
type RouteSource interface {
	Routes() []RouteEntry
}

// DevHandler serves all /dev/* endpoints.
//
// DevHandler 提供所有 /dev/* 端点。
type DevHandler struct {
	db             *gorm.DB
	broadcaster    *loggerinfra.LogBroadcaster
	collectionsDir string
	integrationDir string
	forgifyHome    string
	port           int
	tools          []toolapp.Tool
	llmFactory     *llminfra.Factory
	shellManager   *shelltool.ProcessManager
	log            *zap.Logger
	buildID        string
	startedAt      time.Time
	recorder       RouteSource
}

func NewDevHandler(
	db *gorm.DB,
	broadcaster *loggerinfra.LogBroadcaster,
	collectionsDir, integrationDir, forgifyHome string,
	port int,
	tools []toolapp.Tool,
	llmFactory *llminfra.Factory,
	shellManager *shelltool.ProcessManager,
	log *zap.Logger,
	recorder RouteSource,
) *DevHandler {
	if recorder == nil {
		panic("handlers.NewDevHandler: RouteSource is nil")
	}
	return &DevHandler{
		db:             db,
		broadcaster:    broadcaster,
		collectionsDir: collectionsDir,
		integrationDir: integrationDir,
		forgifyHome:    forgifyHome,
		port:           port,
		buildID:        fmt.Sprintf("%d", time.Now().Unix()),
		startedAt:      time.Now(),
		tools:          tools,
		llmFactory:     llmFactory,
		shellManager:   shellManager,
		log:            log,
		recorder:       recorder,
	}
}

func (h *DevHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /dev/logs", h.StreamLogs)
	mux.HandleFunc("POST /dev/sql", h.QuerySQL)
	mux.HandleFunc("GET /dev/schema", h.Schema)
	mux.HandleFunc("GET /dev/collections", h.ListCollections)
	mux.HandleFunc("GET /dev/tools", h.ListTools)
	mux.HandleFunc("POST /dev/invoke", h.InvokeTool)
	mux.HandleFunc("GET /dev/info", h.Info)
	mux.HandleFunc("GET /dev/forgify-home", h.ForgifyHome)
	mux.HandleFunc("GET /dev/runtime", h.Runtime)
	mux.HandleFunc("GET /dev/routes", h.Routes)
	if h.shellManager != nil {
		mux.HandleFunc("GET /dev/bash-processes", h.BashProcesses)
	}
	if h.llmFactory != nil {
		mux.HandleFunc("POST /dev/mock-llm/scripts", h.MockLLMPushScripts)
		mux.HandleFunc("GET /dev/mock-llm/queue", h.MockLLMQueue)
		mux.HandleFunc("DELETE /dev/mock-llm/scripts", h.MockLLMClear)
		mux.HandleFunc("GET /dev/mock-llm/last-prompt", h.MockLLMLastPrompt)
		mux.HandleFunc("GET /dev/llm-trace", h.LLMTrace)
	}
	noCache := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			next.ServeHTTP(w, r)
		})
	}
	mux.Handle("/dev/static/", noCache(http.StripPrefix("/dev/static/", http.FileServer(http.Dir(h.integrationDir)))))
	mux.HandleFunc("/dev/", h.ServeIndex)
}

// Replaces the hand-maintained dev_routes.go list, which drifted twice.
//
// 替代手维护的 dev_routes.go 清单(曾两次 drift);从 RouteSource 取实时注册。
func (h *DevHandler) Routes(w http.ResponseWriter, r *http.Request) {
	routes := h.recorder.Routes()
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Method != routes[j].Method {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	writeDevJSON(w, http.StatusOK, routes)
}

// ServeIndex serves the testend HTML entry; tries index.html then tester.html.
//
// ServeIndex 提供 testend 入口 HTML;先 index.html 再 tester.html。
func (h *DevHandler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	for _, name := range []string{"index.html", "tester.html"} {
		data, err = os.ReadFile(filepath.Join(h.integrationDir, name))
		if err == nil {
			break
		}
	}
	if err != nil {
		http.Error(w, "testend not built — run `make build-testend`", http.StatusNotFound)
		return
	}
	body := strings.ReplaceAll(string(data), "__BUILD__", h.buildID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(body))
}

// StreamLogs replays the ring buffer then streams new log entries as SSE.
//
// StreamLogs 先回放环形缓冲区,再以 SSE 推送新日志。
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

// QuerySQL executes a read-only SELECT and returns columns + rows.
//
// QuerySQL 执行只读 SELECT,返回列名和行。
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
			h.log.Warn("dev: row scan failed", zap.Error(err))
			continue
		}
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

type schemaColumn struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"notNull"`
	PK      bool   `json:"pk"`
	Default string `json:"default,omitempty"`
}

type schemaTable struct {
	Name     string         `json:"name"`
	RowCount int64          `json:"rowCount"`
	Columns  []schemaColumn `json:"columns"`
}

// Schema lists every user table with column definitions and row count.
//
// Schema 列出每个用户表的列定义和行数。
func (h *DevHandler) Schema(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := h.db.DB()
	if err != nil {
		writeDevJSON(w, http.StatusInternalServerError, sqlErrorResponse{Error: err.Error()})
		return
	}
	rows, err := sqlDB.QueryContext(r.Context(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		writeDevJSON(w, http.StatusInternalServerError, sqlErrorResponse{Error: err.Error()})
		return
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			h.log.Warn("dev: schema name scan failed", zap.Error(err))
			continue
		}
		names = append(names, n)
	}
	rows.Close()

	out := make([]schemaTable, 0, len(names))
	for _, name := range names {
		t := schemaTable{Name: name}
		_ = sqlDB.QueryRowContext(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM %q", name)).Scan(&t.RowCount)
		colRows, err := sqlDB.QueryContext(r.Context(), fmt.Sprintf("PRAGMA table_info(%q)", name))
		if err != nil {
			h.log.Warn("dev: PRAGMA table_info failed", zap.String("table", name), zap.Error(err))
		} else {
			for colRows.Next() {
				var (
					cid          int
					cname, ctype string
					notNull, pk  int
					dflt         any
				)
				if err := colRows.Scan(&cid, &cname, &ctype, &notNull, &dflt, &pk); err != nil {
					h.log.Warn("dev: column scan failed", zap.String("table", name), zap.Error(err))
					continue
				}
				col := schemaColumn{Name: cname, Type: ctype, NotNull: notNull == 1, PK: pk == 1}
				if dflt != nil {
					col.Default = fmt.Sprintf("%v", dflt)
				}
				t.Columns = append(t.Columns, col)
			}
			colRows.Close()
		}
		out = append(out, t)
	}
	writeDevJSON(w, http.StatusOK, out)
}

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

// ListCollections returns parsed *.yaml collection definitions.
//
// ListCollections 返回解析好的集合 YAML 定义。
func (h *DevHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.collectionsDir)
	if err != nil {
		h.log.Warn("dev: read collections dir", zap.String("dir", h.collectionsDir), zap.Error(err))
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

type devToolSummary struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ListTools returns name + description for every registered system tool.
//
// ListTools 返每个注册 system tool 的名称和描述。
func (h *DevHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	out := make([]devToolSummary, 0, len(h.tools))
	for _, t := range h.tools {
		out = append(out, devToolSummary{Name: t.Name(), Desc: t.Description()})
	}
	writeDevJSON(w, http.StatusOK, out)
}

type invokeRequest struct {
	Tool string `json:"tool"`
	Args string `json:"args"`
}

type invokeResponse struct {
	Output    string `json:"output"`
	OK        bool   `json:"ok"`
	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`
}

// InvokeTool directly runs a named tool with caller-supplied JSON args.
//
// InvokeTool 直接用调用方 JSON 参数运行指定 tool。
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

func writeDevJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
