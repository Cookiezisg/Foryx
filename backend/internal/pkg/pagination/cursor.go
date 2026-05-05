// Package pagination parses cursor-based pagination params and encodes
// opaque continuation cursors as base64url(JSON) so the internal shape
// can evolve without bumping API versions.
//
// Package pagination 解析 cursor 分页参数并把续传 cursor 编码为 base64url(JSON)，
// 让内部结构能演化而不升级 API 版本。
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Params is the normalized pagination input.
//
// Params 是标准化的分页输入。
type Params struct {
	Cursor string
	Limit  int
}

// Cursor is the standard (created_at, id) tuple used by every store list
// endpoint — stable when timestamps collide. JSON tags are short ("c"/"i")
// to keep encoded cursors compact.
//
// Cursor 是所有 store 列表端点统一的 (created_at, id) 元组——时间戳相同
// 也能稳定分页。JSON tag 用短名（"c"/"i"）让编码后字符串紧凑。
type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

// Parse extracts pagination params from r's query string. Invalid values
// return errorsdomain.ErrInvalidRequest.
//
// Parse 从 r 的 query string 提取分页参数。非法值返 errorsdomain.ErrInvalidRequest。
func Parse(r *http.Request) (Params, error) {
	q := r.URL.Query()

	limit := DefaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Params{}, fmt.Errorf("limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest)
		}
		if n > MaxLimit {
			n = MaxLimit
		}
		limit = n
	}

	return Params{
		Cursor: q.Get("cursor"),
		Limit:  limit,
	}, nil
}

// EncodeCursor marshals v as base64url(JSON) for the nextCursor field.
// nil → "" (no more pages).
//
// EncodeCursor 把 v 编码为 base64url(JSON) 用于 nextCursor 字段。nil → ""（无下一页）。
func EncodeCursor(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor reverses EncodeCursor. Empty cursor is a no-op (v untouched);
// malformed cursors return errorsdomain.ErrInvalidRequest.
//
// DecodeCursor 是 EncodeCursor 的逆操作。空 cursor 为 no-op；
// 格式错误的 cursor 返 errorsdomain.ErrInvalidRequest。
func DecodeCursor(cursor string, v any) error {
	if cursor == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("decode cursor: %w", errorsdomain.ErrInvalidRequest)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("unmarshal cursor: %w", errorsdomain.ErrInvalidRequest)
	}
	return nil
}
