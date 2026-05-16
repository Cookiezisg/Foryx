package middleware

import (
	"net/http"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// NotFound is the unmatched-URL fallback returning an N1 error envelope.
//
// NotFound 是 unmatched URL 的兜底,返 N1 错误 envelope。
func NotFound(w http.ResponseWriter, r *http.Request) {
	responsehttpapi.Error(w, http.StatusNotFound,
		"NOT_FOUND",
		"route not found: "+r.URL.Path,
		nil)
}
