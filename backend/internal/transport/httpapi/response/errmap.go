package response

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
)

// statusForKind maps a domain error Kind to its HTTP status — the canonical mapping
// declared on each Kind in domain/errors. This switch IS the entire domain→HTTP table:
// transport carries no per-error mapping and imports no business domain — the structured
// Error{Kind,Code} makes one Kind→status switch sufficient.
//
// statusForKind 把 domain 错误 Kind 映射到 HTTP status——domain/errors 每个 Kind 注释里的
// 权威映射。这个 switch 就是 domain→HTTP 的全部映射：transport 不持逐错误表、不 import 任何
// 业务 domain——结构化 Error{Kind,Code} 使一个 Kind→status switch 足矣。
func statusForKind(k errorspkg.Kind) int {
	switch k {
	case errorspkg.KindInvalid:
		return http.StatusBadRequest
	case errorspkg.KindUnauthorized:
		return http.StatusUnauthorized
	case errorspkg.KindNotFound:
		return http.StatusNotFound
	case errorspkg.KindConflict:
		return http.StatusConflict
	case errorspkg.KindUnprocessable:
		return http.StatusUnprocessableEntity
	case errorspkg.KindTooLarge:
		return http.StatusRequestEntityTooLarge
	case errorspkg.KindUnsupportedMedia:
		return http.StatusUnsupportedMediaType
	case errorspkg.KindRateLimited:
		return http.StatusTooManyRequests
	case errorspkg.KindBadGateway:
		return http.StatusBadGateway
	case errorspkg.KindUnavailable:
		return http.StatusServiceUnavailable
	case errorspkg.KindGatewayTimeout:
		return http.StatusGatewayTimeout
	case errorspkg.KindAccepted:
		return http.StatusAccepted
	case errorspkg.KindClientClosed:
		return 499
	case errorspkg.KindGone:
		return http.StatusGone
	case errorspkg.KindForbidden:
		return http.StatusForbidden
	default: // KindInternal + zero value → safest outcome
		return http.StatusInternalServerError
	}
}

// FromDomainError writes the N1 error envelope for err. A structured *errorspkg.Error
// maps via statusForKind(Kind) + its stable wire Code + Details. stdlib context errors are
// special-cased (the only non-Error sentinels transport knows). Anything else → 500 with a
// suppressed message (logged), never leaking internals.
//
// FromDomainError 为 err 写 N1 错误 envelope。结构化 *Error 经 statusForKind(Kind) + 稳定
// wire Code + Details 映射；stdlib context 错误特例（transport 唯一认识的非 Error sentinel）；
// 其余 → 500 隐藏原文（记日志），绝不泄露内部。
func FromDomainError(w http.ResponseWriter, log *zap.Logger, err error) {
	var de *errorspkg.Error
	if errors.As(err, &de) {
		Error(w, statusForKind(de.Kind), de.Code, de.Message, de.Details)
		return
	}
	switch {
	case errors.Is(err, context.Canceled):
		Error(w, 499, "CLIENT_CLOSED", "client closed request", nil)
	case errors.Is(err, context.DeadlineExceeded):
		Error(w, http.StatusGatewayTimeout, "REQUEST_TIMEOUT", "request timed out", nil)
	default:
		if log != nil {
			log.Error("unmapped error (defaulting to 500)", zap.Error(err))
		}
		Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", nil)
	}
}
