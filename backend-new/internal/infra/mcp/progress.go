package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func progressFrom(ctx context.Context) func(string) {
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
