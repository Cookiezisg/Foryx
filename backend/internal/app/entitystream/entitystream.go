// Package entitystream is the producer-side helper for the entities SSE stream (SSE-C): it emits a
// scoped node (open → delta* → close, or a point Signal) to a stream Bridge, anchored at an entity
// (function/handler/agent/workflow/mcp/control/approval/document/skill/trigger). It is the single
// primitive behind every entities-stream activity:
//
//   - forge — the loop mirrors a create/edit tool_call's content delta here (entity panel fills live)
//   - run   — an entity's Service tees its execution output here (entity panel's live terminal)
//   - fire  — a trigger emits a point Signal here (the trigger panel's firing log)
//
// Single responsibility: emit ONE node to ONE (bridge, scope). The dual-write to the messages
// stream (when a run/forge also happens inside a chat tool_call) is composed by the caller
// (io.MultiWriter / a second emit), never fanned out here — keeping this primitive simple.
//
// Package entitystream 是 entities SSE 流（SSE-C）的生产侧助手：把一个带 scope 的节点（open→delta*→close
// 或点 Signal）发到 stream Bridge、锚在某实体上。它是 entities 流一切活动背后的唯一原语：forge（loop 把
// create/edit tool_call 的内容 delta 镜像到这）、run（实体 Service 把执行输出 tee 到这）、fire（trigger 发点
// 信号）。单一职责：往一个 (bridge, scope) 发一个节点；到 messages 流的 dual-write 由调用方组合
// （io.MultiWriter / 二次 emit），绝不在此 fan-out，保持原语简单。
package entitystream

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"go.uber.org/zap"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// The three entities-stream node types (Node.Type), one per activity:
//
// 三种 entities 流节点型（Node.Type），每种一类活动：
const (
	NodeForge = "forge" // an entity's content being written (loop mirrors a create/edit tool_call)
	NodeRun   = "run"   // an entity's execution intermediate (a Service tees stdout / yields / a sub-loop)
	NodeFire  = "fire"  // a trigger firing — a point Signal
)

type bridgeKey struct{}

// WithBridge seeds the entities Bridge into ctx so a loop / Service that already threads ctx can
// reach it without a dedicated dependency. (Run-side Services may instead hold the Bridge by
// injection; both reach the same Bus.) No bridge in ctx → emitters are no-ops.
//
// WithBridge 把 entities Bridge 种进 ctx，使已穿 ctx 的 loop / Service 无需专门依赖即可取到。（run 侧
// Service 也可注入持有；都指向同一 Bus。）ctx 无 bridge → emitter 皆 no-op。
func WithBridge(ctx context.Context, b streamdomain.Bridge) context.Context {
	if b == nil {
		return ctx
	}
	return context.WithValue(ctx, bridgeKey{}, b)
}

// BridgeFrom returns the entities Bridge seeded by WithBridge, or nil.
//
// BridgeFrom 返回 WithBridge 种入的 entities Bridge，或 nil。
func BridgeFrom(ctx context.Context) streamdomain.Bridge {
	b, _ := ctx.Value(bridgeKey{}).(streamdomain.Bridge)
	return b
}

type runScopeKey struct{}

// WithRunScope marks ctx as "this loop run IS an entity's execution" so the loop's block emitter
// mirrors every frame onto the entities stream scoped here (SSE-C: an agent run shows its ReAct
// trace live on the agent panel). Set by the run Service alongside WithBridge.
//
// WithRunScope 标记 ctx「本次 loop run 即某实体的执行」，使 loop 的 block emitter 把每帧镜像到此 scope 的
// entities 流（SSE-C：agent run 在 agent 面板实时显示 ReAct 轨迹）。由 run Service 与 WithBridge 一同设。
func WithRunScope(ctx context.Context, scope streamdomain.Scope) context.Context {
	if scope.ID == "" {
		return ctx
	}
	return context.WithValue(ctx, runScopeKey{}, scope)
}

// RunScopeFrom returns the entity run scope set by WithRunScope (ok=false if unset).
//
// RunScopeFrom 返回 WithRunScope 设的实体 run scope（未设则 ok=false）。
func RunScopeFrom(ctx context.Context) (streamdomain.Scope, bool) {
	s, ok := ctx.Value(runScopeKey{}).(streamdomain.Scope)
	return s, ok
}

// Writer streams one node (open → delta* → close) to bridge, scoped to an entity. It implements
// io.Writer so a run's stdout / yields pipe straight through; the node opens lazily on first write.
// nil-safe: a disabled writer (no bridge / no scope id) drops everything and Close is a no-op.
//
// Writer 把一个节点（open→delta*→close）流到 bridge、锚在某实体。实现 io.Writer 使 run 的 stdout/yield 直通；
// 首写时懒开节点。nil 安全：禁用的 writer（无 bridge / 无 scope id）丢弃一切、Close 为 no-op。
type Writer struct {
	ctx      context.Context
	bridge   streamdomain.Bridge
	scope    streamdomain.Scope
	nodeType string
	open     json.RawMessage // open-frame node content (e.g. {op:"edit"} for forge)
	log      *zap.Logger

	mu     sync.Mutex
	nodeID string // minted on first write; "" = not opened
	snap   strings.Builder
}

// New builds a Writer over (bridge, scope, nodeType). open is the node's open-frame content (may be
// nil). A nil bridge or empty scope id yields a disabled (no-op) writer, so callers never branch.
//
// New 基于 (bridge, scope, nodeType) 构造 Writer。open 是节点 open 帧内容（可 nil）。bridge 为 nil 或 scope
// id 空 → 禁用（no-op）writer，调用方无需分支。
func New(ctx context.Context, bridge streamdomain.Bridge, scope streamdomain.Scope, nodeType string, open json.RawMessage) *Writer {
	return &Writer{ctx: ctx, bridge: bridge, scope: scope, nodeType: nodeType, open: open, log: zap.NewNop()}
}

func (w *Writer) enabled() bool { return w != nil && w.bridge != nil && w.scope.ID != "" }

// Write implements io.Writer: opens the node on first non-empty write, then pushes p as a delta.
// Always reports the full length consumed so io.MultiWriter / io.Copy never see a short write.
//
// Write 实现 io.Writer：首次非空写时开节点，再把 p 作为 delta 推。总报告吃下全长，使 io.MultiWriter / io.Copy
// 不见短写。
func (w *Writer) Write(p []byte) (int, error) {
	if !w.enabled() || len(p) == 0 {
		return len(p), nil
	}
	chunk := string(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nodeID == "" {
		w.nodeID = idgenpkg.New("blk")
		w.publish(streamdomain.Open{Node: streamdomain.Node{Type: w.nodeType, Content: w.open}})
	}
	w.publish(streamdomain.Delta{Chunk: chunk})
	w.snap.WriteString(chunk)
	return len(p), nil
}

// Close terminates the node with status + a final result snapshot (deltas are ephemeral, so the
// close snapshot is the reconnect truth). No-op if nothing was ever written.
//
// Close 用 status + 最终快照结束节点（delta 可丢，close 快照是重连真相）。从未写过则 no-op。
func (w *Writer) Close(status string, result json.RawMessage) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nodeID == "" {
		return
	}
	w.publish(streamdomain.Close{Status: status, Result: &streamdomain.Node{Type: w.nodeType, Content: result}})
	w.nodeID = ""
}

func (w *Writer) publish(frame streamdomain.Frame) {
	if _, err := w.bridge.Publish(w.ctx, streamdomain.Event{Scope: w.scope, ID: w.nodeID, Frame: frame}); err != nil {
		w.log.Warn("entitystream publish failed", zap.String("scope", w.scope.String()), zap.Error(err))
	}
}

// Signal emits one point-in-time node to bridge scoped to an entity — for events with no streaming
// body (a trigger firing, a flowrun node tick). ephemeral picks the delivery class: true = lossy,
// no-backpressure, seq 0, never buffered (the DB row is the reconnect truth — fire/tick panels
// re-fetch their durable log); false = durable, enters the replay ring. nil-safe.
//
// Signal 往 bridge 发一个点状节点、锚某实体——给无流式体的事件（trigger fire、flowrun 节点 tick）。
// ephemeral 选投递类：true = 可丢、无背压、seq 0、不入 buffer（DB 行才是重连真相——fire/tick 面板重连时
// 重拉其 durable 日志）；false = durable、进 replay 环。nil 安全。
func Signal(ctx context.Context, bridge streamdomain.Bridge, scope streamdomain.Scope, nodeType string, content json.RawMessage, ephemeral bool) {
	if bridge == nil || scope.ID == "" {
		return
	}
	_, _ = bridge.Publish(ctx, streamdomain.Event{
		Scope: scope,
		ID:    idgenpkg.New("sig"),
		Frame: streamdomain.Signal{Node: streamdomain.Node{Type: nodeType, Content: content}, Ephemeral: ephemeral},
	})
}
