// Package errors holds the structured domain error type, its semantic Kind
// classification, and cross-domain sentinels. Per-domain sentinels live in their
// own packages but are built with this package's New + Kind, so transport reads
// Code/Kind/Message/Details directly — no central error-mapping table.
//
// Package errors 持有结构化 domain 错误类型、语义 Kind 分类、跨 domain sentinel。
// 各 domain 的 sentinel 放各自包，但都用本包的 New + Kind 构造，使 transport 直接读
// Code/Kind/Message/Details——无集中错误映射表。
package errors

// Kind is the semantic class of a domain error. transport maps each Kind to one
// HTTP status (the comment on each is the canonical mapping). The zero value is
// KindInternal, so an uninitialized Error defaults to the safest outcome (500) —
// never a silent 200 or 404.
//
// Kind 是 domain 错误的语义分类。transport 把每个 Kind 映射到一个 HTTP status
// （注释即权威映射）。零值是 KindInternal，未初始化的 Error 默认最安全（500），
// 绝不静默成 200 / 404。
type Kind int

const (
	KindInternal         Kind = iota // 500 Internal Server Error (unexpected)
	KindInvalid                      // 400 Bad Request
	KindUnauthorized                 // 401 Unauthorized
	KindNotFound                     // 404 Not Found
	KindConflict                     // 409 Conflict
	KindUnprocessable                // 422 Unprocessable Entity
	KindTooLarge                     // 413 Content Too Large
	KindUnsupportedMedia             // 415 Unsupported Media Type
	KindRateLimited                  // 429 Too Many Requests
	KindBadGateway                   // 502 Bad Gateway (upstream LLM/sandbox/MCP failed)
	KindUnavailable                  // 503 Service Unavailable
	KindGatewayTimeout               // 504 Gateway Timeout
	KindAccepted                     // 202 Accepted (async; e.g. approval required)
	KindClientClosed                 // 499 Client Closed Request
	KindGone                         // 410 Gone (resource existed but was evicted; e.g. SSE replay seq too old)
	KindForbidden                    // 403 Forbidden (request understood but refused; e.g. non-loopback Host)
)
