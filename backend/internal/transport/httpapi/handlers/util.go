// util.go — small helpers shared across handlers in this package.
//
// util.go — 本包内 handler 共享的小工具。
package handlers

import "net/http"

// idAndAction splits a path segment shaped like "<id>:<action>". Returns the
// raw path segment as id, the action suffix, and a boolean reporting whether
// the colon delimiter was present. Used by the apikey and forge handlers to
// dispatch POST `/{id}:action` URLs (per N5 RESTful spec).
//
// idAndAction 把形如 "<id>:<action>" 的路径段拆开。返回 id、action 后缀，
// 以及表示是否含冒号分隔符的布尔。供 apikey / forge handler 分派
// POST `/{id}:action` 路由（按 N5 RESTful 规范）。
func idAndAction(r *http.Request, key string) (id, action string, ok bool) {
	raw := r.PathValue(key)
	for i := 0; i < len(raw); i++ {
		if raw[i] == ':' {
			return raw[:i], raw[i+1:], true
		}
	}
	return raw, "", false
}
