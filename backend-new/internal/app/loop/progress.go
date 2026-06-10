package loop

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"

	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ToolProgress returns a live progress writer bound to the executing tool_call. A tool calls it
// from Execute to surface its inner activity — bash stdout, the env-fix dependency-repair log, a
// handler method's yields — as a streaming `progress` block nested under its tool_call
// (Open.ParentID = tool_call id). The block is STREAM-ONLY: it is pushed straight onto the messages
// bus, never persisted to message_blocks and never fed back to the LLM (the final answer is the
// tool_result). It is nil-safe end to end: outside a streamed tool_call (no messages Bridge in ctx,
// or no tool_call id) every method is a no-op, so a tool that always emits progress stays correct
// under REST, tests, and non-streaming hosts.
//
// ToolProgress 返回绑定到当前 tool_call 的实时进度 writer。工具在 Execute 里调它，把内部活动——bash
// stdout、env-fix 改依赖 log、handler method 的 yield——作为流式 `progress` 块嵌在其 tool_call 下
// （Open.ParentID=tool_call id）。该块**仅流不持久**：直推 messages 总线，绝不落 message_blocks、绝不
// 回喂 LLM（最终答案是 tool_result）。端到端 nil 安全：不在流式 tool_call 里（ctx 无 messages Bridge
// 或无 tool_call id）则全方法 no-op，使「总是发进度」的工具在 REST / 测试 / 非流 host 下仍正确。
func ToolProgress(ctx context.Context) *ToolProgressWriter {
	id, _ := reqctxpkg.GetToolCallID(ctx)
	return &ToolProgressWriter{ctx: ctx, em: newEmitter(ctx, zap.NewNop()), parentID: id}
}

// ToolProgressWriter streams a tool's intermediate output as one `progress` block. It implements
// io.Writer (so bash output can pipe straight through io.MultiWriter / io.Copy) and lazily opens
// the block on the first non-empty write.
//
// ToolProgressWriter 把工具的中间输出流成一个 `progress` 块。实现 io.Writer（bash 输出可经
// io.MultiWriter / io.Copy 直通）、首次非空写时懒开该块。
type ToolProgressWriter struct {
	ctx      context.Context
	em       emitter
	parentID string

	mu      sync.Mutex
	blockID string // minted on first write; "" = block not opened
	snap    strings.Builder
}

// enabled reports whether writes will actually stream (a messages Bridge + a tool_call anchor).
//
// enabled 报告写入是否真会流（有 messages Bridge + tool_call 锚）。
func (w *ToolProgressWriter) enabled() bool {
	return w != nil && w.em.bridge != nil && w.parentID != ""
}

// Write implements io.Writer: emits p as a progress delta under the tool_call, opening the block
// on first use. It always reports the full length consumed (a disabled writer drops silently) so
// io.Copy / io.MultiWriter callers never see a short write or error.
//
// Write 实现 io.Writer：把 p 作为 progress delta 发到 tool_call 下，首次用时开块。总是报告吃下全长
// （禁用的 writer 静默丢弃），使 io.Copy / io.MultiWriter 不会见到短写或错误。
func (w *ToolProgressWriter) Write(p []byte) (int, error) {
	if !w.enabled() || len(p) == 0 {
		return len(p), nil
	}
	chunk := string(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.blockID == "" {
		w.blockID = idgenpkg.New("blk")
		w.em.open(w.ctx, w.blockID, w.parentID, messagesdomain.BlockTypeProgress, nil)
	}
	w.em.delta(w.ctx, w.blockID, chunk)
	w.snap.WriteString(chunk)
	return len(p), nil
}

// Print is a convenience for tools that emit discrete progress lines (env-fix attempts, install
// stages) rather than a byte stream.
//
// Print 是便利方法，给发离散进度行（env-fix 尝试、安装阶段）而非字节流的工具用。
func (w *ToolProgressWriter) Print(s string) { _, _ = w.Write([]byte(s)) }

// Close terminates the progress block with the accumulated snapshot (the reconnect source of
// truth, since deltas are ephemeral). No-op if nothing was ever written — a tool may safely
// `defer prog.Close()` whether or not it produced progress.
//
// Close 用累积快照结束 progress 块（delta 可丢，快照是重连真相）。从未写过则 no-op——工具无论是否产出
// 进度都可安全 `defer prog.Close()`。
func (w *ToolProgressWriter) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.blockID == "" {
		return
	}
	text := w.snap.String()
	snap := &streamdomain.Node{Type: messagesdomain.BlockTypeProgress, Content: streamdomain.JSONContent(progressContent{Text: text})}
	w.em.close(w.ctx, w.blockID, messagesdomain.StatusCompleted, snap, "")
	// Hand the finished progress block to the loop (if it set up a capture) so it persists with the
	// turn — parented to the tool_call, mirroring the tool_result. The LLM history projection is a
	// type whitelist, so persisting it never feeds it back to the model.
	//
	// 把完成的 progress 块交给 loop（若设了捕获槽）随回合持久化——挂在 tool_call 下（同 tool_result）。LLM
	// 历史投影是类型白名单，故持久化绝不回喂模型。
	if pcap := progressCaptureFrom(w.ctx); pcap != nil {
		pcap.add(messagesdomain.Block{
			ID:            w.blockID,
			Type:          messagesdomain.BlockTypeProgress,
			Content:       text,
			ParentBlockID: w.parentID,
			Status:        messagesdomain.StatusCompleted,
		})
	}
	w.blockID = ""
}

// progressContent is the progress block's snapshot payload.
//
// progressContent 是 progress 块的快照载荷。
type progressContent struct {
	Text string `json:"text"`
}

// progressCapture collects the progress blocks produced under one tool call so runOneTool can fold
// them into the turn's persisted blocks. A parallel tool batch gives each call its own capture, so
// the mutex only guards a single tool emitting from multiple goroutines.
//
// progressCapture 收集一次 tool 调用下产出的 progress 块，供 runOneTool 折进回合的持久化 blocks。并行
// 批里每个调用各有自己的 capture，故 mutex 只护单个 tool 从多 goroutine 发的情况。
type progressCapture struct {
	mu     sync.Mutex
	blocks []messagesdomain.Block
}

func (c *progressCapture) add(b messagesdomain.Block) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocks = append(c.blocks, b)
}

func (c *progressCapture) take() []messagesdomain.Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks
}

type progressCaptureKey struct{}

// withProgressCapture seeds a capture so progress blocks emitted under this ctx persist.
//
// withProgressCapture 埋一个 capture，使本 ctx 下发的 progress 块得以持久化。
func withProgressCapture(ctx context.Context, c *progressCapture) context.Context {
	return context.WithValue(ctx, progressCaptureKey{}, c)
}

func progressCaptureFrom(ctx context.Context) *progressCapture {
	c, _ := ctx.Value(progressCaptureKey{}).(*progressCapture)
	return c
}
