package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// RequestLogger logs one line per request; must sit INSIDE Recover for 500 visibility.
//
// RequestLogger 每请求一行日志;必须在 Recover 内层才能看到 500 状态。
func RequestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newStatusRecorder(w)

			next.ServeHTTP(rec, r)

			log.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rec.status),
				zap.Int("bytes", rec.bytes),
				zap.Int64("elapsed_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

// statusRecorder wraps ResponseWriter to capture status + bytes for logging.
//
// statusRecorder 包装 ResponseWriter 记录状态码 + 字节数供日志读取。
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Flush delegates to the underlying ResponseWriter so SSE works through logger.
//
// Flush 委托底层 ResponseWriter,让 SSE 穿透 logger 中间件。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
