// Package router — Recorder wraps *http.ServeMux to record (method, path) pairs.
// Lets /dev/routes return real registered routes via reflection-free record.
//
// Recorder 包装 *http.ServeMux,在 HandleFunc 时记录 method+path,
// /dev/routes 反向取列表,根本上消除手维护清单 drift。
package router

import (
	"net/http"
	"strings"
	"sync"

	handlershttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/handlers"
)

// Route is one recorded registration.
//
// Route 是一次注册记录。
type Route struct {
	Method string
	Path   string
}

// Recorder wraps a mux and intercepts HandleFunc to record entries.
//
// Recorder 包装 mux,截获 HandleFunc 记录条目。
type Recorder struct {
	mux    *http.ServeMux
	mu     sync.RWMutex
	routes []Route
}

// NewRecorder wraps mux so registrations are recorded in addition to forwarded.
//
// NewRecorder 包装 mux,注册同时写入记录。
func NewRecorder(mux *http.ServeMux) *Recorder {
	return &Recorder{mux: mux, routes: make([]Route, 0, 64)}
}

// HandleFunc records (method, path) then forwards to underlying mux.
//
// Go 1.22+ ServeMux syntax: "GET /path" or pure "/path" (any method).
func (r *Recorder) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	method, path := parsePattern(pattern)
	r.mu.Lock()
	r.routes = append(r.routes, Route{Method: method, Path: path})
	r.mu.Unlock()
	r.mux.HandleFunc(pattern, h)
}

// Handle records (method, path) then forwards.
//
// Handle 同 HandleFunc,接 http.Handler。
func (r *Recorder) Handle(pattern string, h http.Handler) {
	method, path := parsePattern(pattern)
	r.mu.Lock()
	r.routes = append(r.routes, Route{Method: method, Path: path})
	r.mu.Unlock()
	r.mux.Handle(pattern, h)
}

// List returns a snapshot of recorded routes.
//
// List 返回记录的路由快照。
func (r *Recorder) List() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Route, len(r.routes))
	copy(out, r.routes)
	return out
}

func parsePattern(p string) (method, path string) {
	p = strings.TrimSpace(p)
	if i := strings.IndexByte(p, ' '); i > 0 {
		return p[:i], strings.TrimSpace(p[i+1:])
	}
	return "ANY", p
}

// RecorderAdapter wraps *Recorder to implement handlershttpapi.RouteSource.
// Converts []Route → []handlershttpapi.RouteEntry so DevHandler can call Routes().
//
// RecorderAdapter 将 *Recorder 转为 handlershttpapi.RouteSource,
// 让 DevHandler 无需知道 router 内部类型。
type RecorderAdapter struct {
	rec *Recorder
}

func NewRecorderAdapter(rec *Recorder) *RecorderAdapter {
	return &RecorderAdapter{rec: rec}
}

func (a *RecorderAdapter) Routes() []handlershttpapi.RouteEntry {
	raw := a.rec.List()
	out := make([]handlershttpapi.RouteEntry, len(raw))
	for i, r := range raw {
		out[i] = handlershttpapi.RouteEntry{Method: r.Method, Path: r.Path}
	}
	return out
}

// Compile-time guard: *RecorderAdapter satisfies handlershttpapi.RouteSource.
var _ handlershttpapi.RouteSource = (*RecorderAdapter)(nil)

// Compile-time guard via anonymous interface — *Recorder satisfies the same
// shape as handlers.Registrar without router/ importing handlers/, which
// would create a cycle.
//
// 用匿名接口替代直接引 handlers.Registrar 做编译期校验,避免 router→handlers 反向 import。
var _ interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
	Handle(string, http.Handler)
} = (*Recorder)(nil)
