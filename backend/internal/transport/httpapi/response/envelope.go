// Package response provides envelope-shaped HTTP response helpers.
//
// Package response 提供 envelope 格式的 HTTP 响应辅助函数。
package response

import (
	"encoding/json"
	"net/http"
)

// envelope is the on-wire shape; exactly one of Data/Error is non-nil.
//
// envelope 是线上响应形状,Data/Error 恰有一个非 nil。
type envelope struct {
	Data       any        `json:"data,omitempty"`
	Error      *errorBody `json:"error,omitempty"`
	NextCursor *string    `json:"nextCursor,omitempty"`
	HasMore    *bool      `json:"hasMore,omitempty"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Success writes {"data": body} with the given status.
//
// Success 写出 {"data": body} 及给定状态码。
func Success(w http.ResponseWriter, status int, body any) {
	writeJSON(w, status, envelope{Data: body})
}

// Created is Success(w, 201, body).
//
// Created 是 Success(w, 201, body) 的快捷方式。
func Created(w http.ResponseWriter, body any) {
	Success(w, http.StatusCreated, body)
}

// NoContent writes HTTP 204 with no body.
//
// NoContent 写出 HTTP 204,无响应体。
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Paged writes {data, nextCursor, hasMore}; empty nextCursor = last page.
//
// Paged 写出分页列表;nextCursor 为空表示最后一页。
func Paged(w http.ResponseWriter, items any, nextCursor string, hasMore bool) {
	env := envelope{Data: items, HasMore: &hasMore}
	if nextCursor != "" {
		env.NextCursor = &nextCursor
	}
	writeJSON(w, http.StatusOK, env)
}

// Error writes a structured error envelope for handler-detected errors.
//
// Error 写出结构化错误 envelope,用于 handler 内部发现的错误。
func Error(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, envelope{Error: &errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
