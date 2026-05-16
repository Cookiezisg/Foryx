package handlers

import "net/http"

// idAndAction splits "<id>:<action>" from r.PathValue(key); ok=false if no colon.
//
// idAndAction 把 r.PathValue(key) 拆成 "<id>:<action>",无冒号 ok=false。
func idAndAction(r *http.Request, key string) (id, action string, ok bool) {
	raw := r.PathValue(key)
	for i := 0; i < len(raw); i++ {
		if raw[i] == ':' {
			return raw[:i], raw[i+1:], true
		}
	}
	return raw, "", false
}
