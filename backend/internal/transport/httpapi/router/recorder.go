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

// Compile-time guard: *Recorder satisfies the handlers.Registrar shape.
// Anonymous interface avoids importing handlers/ from router/ (cycle prevention).
//
// 编译期断言:*Recorder 满足 handlers.Registrar 的结构形状。
// 用匿名接口避免 router 引 handlers 产生循环依赖。
var _ interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
	Handle(string, http.Handler)
} = (*Recorder)(nil)
