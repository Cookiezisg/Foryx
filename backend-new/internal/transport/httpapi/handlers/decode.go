package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// decodeJSON strictly decodes the request body into v (unknown fields rejected).
// A malformed body becomes ErrInvalidRequest wrapping the parse error, so
// response.FromDomainError renders a uniform 400 and handlers never inspect it.
//
// decodeJSON 严格解码请求体到 v（拒绝未知字段）。畸形体变 ErrInvalidRequest（包裹解析错误），
// 由 response.FromDomainError 统一渲染 400，handler 无需检查。
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errorsdomain.ErrInvalidRequest.WithCause(err)
	}
	return nil
}

// decodeJSONOptional is decodeJSON for endpoints whose body is OPTIONAL: an empty body leaves v at
// its zero value (no error); a present body is decoded strictly. For :action verbs like :trigger
// where the request payload may be omitted.
//
// decodeJSONOptional 是给「请求体可选」端点的 decodeJSON：空体留 v 为零值（不报错）；有体则严格解码。
// 用于 :trigger 等 payload 可省的 :action 动词。
func decodeJSONOptional(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty body — leave v zero-valued
		}
		return errorsdomain.ErrInvalidRequest.WithCause(err)
	}
	return nil
}
