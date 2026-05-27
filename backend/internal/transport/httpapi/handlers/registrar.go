// Package handlers — Registrar is the minimal mux-like surface handlers
// register routes against. Satisfied by *http.ServeMux and *router.Recorder.
//
// Registrar 是 handler 注册路由的最小 mux 接口;*http.ServeMux 与 *router.Recorder 都实现。
package handlers

import "net/http"

// Registrar is implemented by *http.ServeMux and *router.Recorder.
//
// Registrar 由 *http.ServeMux 与 *router.Recorder 实现。
type Registrar interface {
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
	Handle(pattern string, h http.Handler)
}

// Compile-time guard: *http.ServeMux satisfies Registrar.
var _ Registrar = (*http.ServeMux)(nil)
