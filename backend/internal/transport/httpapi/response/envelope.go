// Package response provides envelope-shaped HTTP response helpers (the N1 contract:
// success → {"data": ...}, failure → {"error": {code, message, details}}).
//
// Package response 提供 envelope 格式的 HTTP 响应辅助（N1 契约：成功 {"data"}、
// 失败 {"error":{code,message,details}}）。
package response

import (
	"encoding/json"
	"net/http"
	"reflect"
)

// envelope is the on-wire shape; exactly one of Data/Error is non-nil.
//
// envelope 是线上响应形状，Data/Error 恰有一个非 nil。
type envelope struct {
	Data       any        `json:"data,omitempty"`
	Error      *errorBody `json:"error,omitempty"`
	NextCursor *string    `json:"nextCursor,omitempty"`
	HasMore    *bool      `json:"hasMore,omitempty"`
}

// pagedEnvelope is the on-wire shape for a paginated list. Unlike envelope, Data is NOT omitempty:
// an empty page MUST serialize as {"data": []}, never null or an absent key — clients iterate data
// and a nil/absent value would NPE them (N4). The shared envelope's omitempty (needed so an error
// response omits data) would drop an empty list, hence a dedicated type.
//
// pagedEnvelope 是分页列表的线上形状。与 envelope 不同，Data **不** omitempty：空页必须序列化成
// {"data": []}、绝不 null 或缺键——client 会遍历 data、nil/缺会让它崩（N4）。共享 envelope 的 omitempty
// （为让错误响应省略 data 而设）会把空列表丢掉，故另立此类型。
type pagedEnvelope struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"nextCursor,omitempty"`
	HasMore    bool    `json:"hasMore"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Success writes {"data": body} with the given status. A nil typed slice body is normalized to [] (it
// would otherwise serialize to JSON null) so a NON-paged list endpoint never flips between [] (populated)
// and null (empty) and break a client's `for (x of resp.data)` — the non-paged sibling of Paged's
// empty-slice guarantee (F170). A non-slice body (single object) passes through untouched.
//
// Success 写出 {"data": body} 及给定状态码。nil 类型化 slice body 归一成 [](否则序列化成 JSON null），使
// **非分页**列表端点不在 []（有值）与 null（空）间翻转、不崩客户端 for-of——Paged 空-slice 保证的非分页兄弟
// （F170）。非 slice body（单对象）原样透传。
func Success(w http.ResponseWriter, status int, body any) {
	writeJSON(w, status, envelope{Data: emptySliceIfNil(body)})
}

// Created is Success(w, 201, body).
//
// Created 是 Success(w, 201, body) 的快捷方式。
func Created(w http.ResponseWriter, body any) {
	Success(w, http.StatusCreated, body)
}

// NoContent writes HTTP 204 with no body.
//
// NoContent 写出 HTTP 204，无响应体。
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Paged writes {data, nextCursor, hasMore}; empty nextCursor = last page.
//
// Paged 写出分页列表；nextCursor 为空表示最后一页。
func Paged(w http.ResponseWriter, items any, nextCursor string, hasMore bool) {
	env := pagedEnvelope{Data: emptySliceIfNil(items), HasMore: hasMore}
	if nextCursor != "" {
		env.NextCursor = &nextCursor
	}
	writeJSON(w, http.StatusOK, env)
}

// emptySliceIfNil replaces a nil slice with an empty slice of the same type, so a paged list always
// marshals as [] (N4), never null. A nil typed slice held in an interface otherwise serialises to
// JSON null. Non-slice / non-nil inputs pass through untouched.
//
// emptySliceIfNil 把 nil slice 换成同类型空 slice，使分页列表恒序列化成 []（N4）、绝不 null。装在 interface
// 里的 nil 类型化 slice 否则会序列化成 JSON null。非 slice / 非 nil 输入原样透传。
func emptySliceIfNil(items any) any {
	v := reflect.ValueOf(items)
	if v.Kind() == reflect.Slice && v.IsNil() {
		return reflect.MakeSlice(v.Type(), 0, 0).Interface()
	}
	return items
}

// Error writes a structured error envelope for handler-detected errors.
//
// Error 写出结构化错误 envelope，用于 handler 内部发现的错误。
func Error(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, envelope{Error: &errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
