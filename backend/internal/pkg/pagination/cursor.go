// Package pagination parses cursor params and encodes opaque base64url(JSON) cursors.
//
// Package pagination 解析 cursor 参数并编码 base64url(JSON) 不透明 cursor。
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

// Cursor is the standard (created_at, id) tuple; short JSON tags keep encoded cursors compact.
//
// Cursor 是标准 (created_at, id) 元组；JSON tag 用短名保持编码紧凑。
type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

// Parse extracts pagination params from r; invalid values return ErrInvalidRequest.
//
// Parse 从 r 提取分页参数；非法值返 ErrInvalidRequest。
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

// EncodeCursor marshals v as base64url(JSON); nil maps to "" (no more pages).
//
// EncodeCursor 把 v 编码为 base64url(JSON)；nil 返 ""（无下一页）。
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

// DecodeCursor reverses EncodeCursor; empty cursor is a no-op, malformed returns ErrInvalidRequest.
//
// DecodeCursor 是 EncodeCursor 的逆操作；空 cursor 为 no-op，格式错返 ErrInvalidRequest。
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
