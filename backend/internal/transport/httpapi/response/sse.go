// sse.go — Helper for serving HTTP Server-Sent Events streams.
// Centralises the boilerplate (4 standard headers + initial flush + 15-second
// keep-alive ping + ctx-driven shutdown) so each SSE handler only needs to
// describe what to write per item.
//
// sse.go — HTTP SSE 流的服务 helper。集中样板（4 个标准 header + 预 flush +
// 15 秒 keep-alive + ctx 驱动退出），每个 SSE handler 只需描述每条 item
// 的写入逻辑。
package response

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// keepAliveInterval is the gap between SSE keep-alive pings. 15s is short
// enough to keep most reverse proxies (nginx default 60s idle timeout) and
// browsers happy without flooding logs.
//
// keepAliveInterval 是 SSE keep-alive ping 的间隔。15 秒足够避开多数反向代理
// （nginx 默认 60s 空闲超时）和浏览器的断连，同时不会刷屏日志。
const keepAliveInterval = 15 * time.Second

// StreamSSE serves an HTTP Server-Sent Events response. It writes the four
// standard SSE headers, sends the initial flush, runs an optional prelude
// (e.g. ring-buffer replay), then interleaves items from `items` with 15s
// keep-alive pings until the request context is cancelled or `items` closes.
//
// onEvent writes the wire-format payload for each item; StreamSSE flushes
// after. onPrelude is called once before the subscription loop; pass nil
// when no prelude is needed.
//
// Errors returned by onEvent are silently ignored — wire-write errors
// generally mean the client disconnected mid-response, the status code is
// already sent, and the loop's next iteration / ctx-cancel will tear down
// cleanly. Loggers belong to the caller; onEvent can log inside itself.
//
// StreamSSE 服务一次 HTTP SSE 响应。写入 4 个标准 SSE header，预 flush，
// 跑可选 prelude（如环形缓冲回放），随后在 items 通道事件与 15 秒
// keep-alive ping 间交替，直到 request ctx 取消或 items 关闭。
//
// onEvent 负责写每个 item 的 wire 载荷，StreamSSE 在其后 flush。
// onPrelude 在订阅循环开始前调用一次；不需要时传 nil。
//
// onEvent 返回的错误被静默忽略——wire 写错误通常意味客户端中途断开，
// 状态码已发出无可挽回，下次迭代或 ctx 取消会干净拆解。
// 日志属调用方所有；onEvent 可以在自身内部 log。
func StreamSSE[T any](
	w http.ResponseWriter,
	r *http.Request,
	onPrelude func(io.Writer),
	items <-chan T,
	onEvent func(io.Writer, T) error,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "streaming not supported", nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// X-Accel-Buffering: no — disable nginx / reverse-proxy buffering so
	// tokens reach the client immediately.
	// X-Accel-Buffering: no——禁用 nginx / 反向代理缓冲，让 token 立刻到达客户端。
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if onPrelude != nil {
		onPrelude(w)
		flusher.Flush()
	}

	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case item, ok := <-items:
			if !ok {
				return
			}
			_ = onEvent(w, item)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
