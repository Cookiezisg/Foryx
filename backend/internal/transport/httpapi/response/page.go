package response

import (
	"net/http"
	"strconv"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
)

// Pagination bounds (N4: every List endpoint is cursor-paged).
//
// 分页边界（N4：所有 List 接口走 cursor 分页）。
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// PageParams is a parsed list query: an opaque cursor + a clamped limit.
//
// PageParams 是解析后的 List 查询：不透明 cursor + 钳制后的 limit。
type PageParams struct {
	Cursor string
	Limit  int
}

// ParsePage reads ?cursor= and ?limit= from r. limit defaults to DefaultLimit and is
// clamped to [1, MaxLimit]; a non-numeric or <1 limit → ErrInvalidRequest. The cursor is
// kept opaque here — decode it with DecodeCursor once the keyset shape is known.
//
// ParsePage 读 ?cursor= 与 ?limit=。limit 默认 DefaultLimit、钳制到 [1, MaxLimit]；非数字或
// <1 → ErrInvalidRequest。cursor 在此保持不透明，拿到 keyset 形状后用 DecodeCursor 解。
func ParsePage(r *http.Request) (PageParams, error) {
	return ParsePageBounded(r, DefaultLimit, MaxLimit)
}

// ParsePageBounded is ParsePage with caller-supplied default + max — for surfaces with
// tighter bounds than the global 50/200 (search uses 20/50). Same clamp + error semantics,
// so no List endpoint hand-rolls limit parsing (N4).
//
// ParsePageBounded 是 ParsePage 的带界版,供比全局 50/200 更紧的面（search 用 20/50）。同一钳制
// + 错误语义,使无 List 端点手搓 limit 解析（N4）。
func ParsePageBounded(r *http.Request, defaultLimit, maxLimit int) (PageParams, error) {
	p := PageParams{Limit: defaultLimit}
	q := r.URL.Query()
	p.Cursor = q.Get("cursor")
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return PageParams{}, errorspkg.ErrInvalidRequest
		}
		p.Limit = n
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	return p, nil
}

// DecodeCursor decodes the cursor into v (a keyset struct); an empty cursor is a no-op
// (first page). A malformed cursor maps to ErrInvalidRequest (transport-friendly), so the
// pkg/pagination.ErrMalformedCursor never leaks past this layer.
//
// DecodeCursor 把 cursor 解码进 v（keyset struct）；空 cursor 为 no-op（首页）。格式错映射为
// ErrInvalidRequest，使 pkg/pagination.ErrMalformedCursor 不越过这层。
func (p PageParams) DecodeCursor(v any) error {
	if err := paginationpkg.DecodeCursor(p.Cursor, v); err != nil {
		return errorspkg.ErrInvalidRequest
	}
	return nil
}
