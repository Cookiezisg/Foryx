package mcp

import (
	"context"
	"fmt"
	"runtime"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// drainProgress yields the scheduler so the SDK's async notification handler flushes a call's
// already-queued progress lines to the sink before the caller unregisters the per-call token. Cheap
// by design: a no-progress call returns after a couple of yields; a call with progress drains its
// batch then returns once a yield produces nothing new (each signal means the sink already ran —
// same goroutine, sequential). maxYields is a hard backstop against a stuck handler goroutine.
//
// drainProgress 让出调度，使 SDK 异步通知处理器在调用方注销 per-call token 前把已入队的进度行刷进 sink。
// 设计上廉价：无进度调用几次让出即返回；有进度调用排空其批次、某次让出无新增即返回（收到信号即意味 sink
// 已跑——同 goroutine、顺序）。maxYields 作硬兜底，防 handler goroutine 卡死。
func drainProgress(seen <-chan struct{}) {
	const (
		maxYields  = 64
		quietLimit = 2 // consecutive yields with no new progress → drained
	)
	quiet := 0
	for range maxYields {
		select {
		case <-seen:
			quiet = 0
			continue
		default:
		}
		if quiet >= quietLimit {
			return
		}
		quiet++
		runtime.Gosched()
	}
}

type progressKey struct{}

// WithProgress attaches a sink that receives an MCP tool call's progress notifications as they
// arrive. The tool layer sets it (bound to the call's live UI stream); CallTool reads it and, when
// present, requests progress via a per-call token. No ctx value = no progress (REST / boot).
//
// WithProgress 挂一个 sink，接收 MCP 工具调用到来的进度通知。工具层设置它（绑到调用的实时 UI 流）；
// CallTool 读到则用 per-call token 请求进度。ctx 无此值 = 不要进度（REST / boot）。
func WithProgress(ctx context.Context, sink func(string)) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, sink)
}

// ProgressFrom returns the sink set by WithProgress (nil if unset) — exported so the app layer can
// wrap an existing sink (e.g. tee the chat sink AND the entities run terminal).
//
// ProgressFrom 返回 WithProgress 设的 sink（未设为 nil）——导出使 app 层能包既有 sink（如同时 tee chat sink
// 与 entities run 终端）。
func ProgressFrom(ctx context.Context) func(string) {
	sink, _ := ctx.Value(progressKey{}).(func(string))
	return sink
}

// onProgress forwards a server progress notification to the CallTool that registered its token.
// Session-global (one handler per client); an unmatched token is dropped (a stale / untracked call).
//
// onProgress 把 server 进度通知转给登记了该 token 的 CallTool。session 级全局；未匹配 token 丢弃。
func (c *client) onProgress(_ context.Context, req *mcpsdk.ProgressNotificationClientRequest) {
	if req == nil {
		return
	}
	tok, ok := req.Params.ProgressToken.(string)
	if !ok {
		return
	}
	v, ok := c.progress.Load(tok)
	if !ok {
		return
	}
	if sink, _ := v.(func(string)); sink != nil {
		sink(formatProgress(req.Params))
	}
}

// formatProgress renders one progress notification as a human line for the stream.
//
// formatProgress 把一条进度通知渲成一行人读文本，供流式推送。
func formatProgress(p *mcpsdk.ProgressNotificationParams) string {
	msg := p.Message
	if msg == "" {
		msg = "working…"
	}
	if p.Total > 0 {
		return fmt.Sprintf("%s (%.0f/%.0f)\n", msg, p.Progress, p.Total)
	}
	return msg + "\n"
}
