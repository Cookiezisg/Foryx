// Package subagent is the recursive-subconversation engine: the Subagent (Task) tool and
// fork-mode skills call Spawn to run an isolated sub-agent over a focused task and get its final
// answer back synchronously. A subagent ≈ a recursive chat: it owns no table (its turn persists
// as a sub-message in the PARENT conversation, tagged SubagentID, its blocks nested under the
// spawning tool_call via E3), inherits the parent's effective (workspace dialogue) model, and
// cannot spawn further subagents (depth 1). It reuses the shared ReAct engine (app/loop) with a
// hybrid host: agentHost's prompt-history + static tool whitelist, plus chatHost's persist +
// message_stop on a detached context.
//
// Package subagent 是递归子对话引擎：Subagent（Task）工具与 fork 模式 skill 调 Spawn 在一段聚焦任务
// 上跑隔离子 agent 并同步拿回最终答案。subagent ≈ 递归 chat：无自己的表（回合作为 sub-message 落
// 父对话、带 SubagentID、blocks 经 E3 嵌派它的 tool_call 下），承袭父 effective（workspace dialogue）
// 模型，且不能再派 subagent（深度 1）。它复用共享 ReAct 引擎（app/loop）配混血 host：agentHost 的
// prompt 历史 + 静态工具白名单，加 chatHost 的 detached 落盘 + message_stop。
package subagent

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// attrParentBlockID is the sub-message Attrs key holding the spawning tool_call's block id —
// the anchor a reload uses to nest the subagent subtree under its tool_call (the live stream
// carries the same link as the message node's Open.ParentID).
//
// attrParentBlockID 是 sub-message Attrs 里存派它的 tool_call block id 的键——reload 据此把
// subagent 子树嵌在其 tool_call 下（live 流由 message 节点的 Open.ParentID 携同一链接）。
const attrParentBlockID = "parentBlockId"

// Bundle is a ready-to-run LLM client + pre-filled base Request, self-contained so subagent
// doesn't import chatapp. The M7 adapter resolves it (model.Resolve(ScenarioDialogue, …)).
//
// Bundle 是即用 LLM client + 预填 base Request，自包含使 subagent 不引 chatapp。M7 适配器解析它。
type Bundle struct {
	Client   llminfra.Client
	Request  llminfra.Request
	Provider string
}

// ----- DIP ports -----

// ModelResolver yields the model a subagent runs on — the workspace dialogue model, which is the
// parent's effective model in the common (no per-conversation override) case. Inheriting an
// explicit conv.ModelOverride is deferred (it would cross the pkg→domain boundary in reqctx).
//
// ModelResolver 给出 subagent 跑的模型——workspace dialogue 模型，在常见（无 per-conversation
// override）情形即父的 effective model。承袭显式 conv.ModelOverride 延后（会越 reqctx 的 pkg→domain）。
type ModelResolver interface {
	Resolve(ctx context.Context) (Bundle, error)
}

// ToolsProvider returns the parent tool set a subagent's type filters down from. What it
// contains (resident-only vs full) is the M7 wiring's call; filterTools just applies the
// per-type allow-list + strips the Subagent tool.
//
// ToolsProvider 返回 subagent 类型据以过滤的父工具集。它含什么（仅 resident vs 全量）由 M7 装配定；
// filterTools 只套类型白名单 + 剔 Subagent 工具。
type ToolsProvider interface {
	Tools() []toolapp.Tool
}

// Deps are subagent's injected collaborators (DIP). Messages persists the sub-message; Resolver
// resolves the model; Tools is the parent registry; Bridge is the messages stream (nil → no live
// push, REST history still works).
//
// Deps 是 subagent 注入的协作者（DIP）。Messages 落 sub-message；Resolver 解析模型；Tools 是父注册表；
// Bridge 是 messages 流（nil → 无 live 推、REST 历史仍在）。
type Deps struct {
	Messages messagesdomain.Repository
	Resolver ModelResolver
	Tools    ToolsProvider
	Bridge   streamdomain.Bridge
}

// Service runs subagents. It satisfies skilldomain.SubagentRunner so skill fork can dispatch
// through it, and the Subagent tool calls the same Spawn.
//
// Service 跑 subagent。它满足 skilldomain.SubagentRunner 使 skill fork 经它派发，Subagent 工具调同一 Spawn。
type Service struct {
	deps Deps
	reg  *Registry
	log  *zap.Logger
}

// New constructs the Service. nil log → no-op logger.
//
// New 构造 Service。nil log → no-op logger。
func New(deps Deps, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{deps: deps, reg: NewRegistry(), log: log.Named("subagentapp")}
}

var _ skilldomain.SubagentRunner = (*Service)(nil)

// Registry exposes the built-in type registry (the Subagent tool reads Names() for its enum).
//
// Registry 暴露内置类型注册表（Subagent 工具读 Names() 作 enum）。
func (s *Service) Registry() *Registry { return s.reg }

// Spawn runs one subagent over prompt and returns its final answer. Synchronous: it builds the
// hybrid host, runs the ReAct loop, and the host persists the sub-message + streams it. A bad
// type / model-resolve error returns an error string the caller surfaces as the tool_result (no
// HTTP error — subagent failures are tool-level, not request-level). Recursion is refused here
// too (defense in depth; the Subagent tool also guards).
//
// Spawn 在 prompt 上跑一个 subagent 并返回最终答案。同步：构造混血 host、跑 ReAct 循环，host 落
// sub-message + 推流。坏类型 / 模型解析错返 error 串、由调用方作 tool_result 暴露（无 HTTP 错——
// subagent 失败是工具级、非请求级）。递归在此也拒（防御纵深；Subagent 工具亦守卫）。
func (s *Service) Spawn(ctx context.Context, agentType, prompt string) (string, error) {
	if _, inSub := reqctxpkg.GetSubagentID(ctx); inSub {
		return "", fmt.Errorf("subagent: a subagent cannot spawn another subagent")
	}
	typ, ok := s.reg.Get(agentType)
	if !ok {
		return "", fmt.Errorf("subagent: unknown type %q (have %v)", agentType, s.reg.Names())
	}

	bundle, err := s.deps.Resolver.Resolve(ctx)
	if err != nil {
		return "", fmt.Errorf("subagent: resolve model: %w", err)
	}

	var parentTools []toolapp.Tool
	if s.deps.Tools != nil {
		parentTools = s.deps.Tools.Tools()
	}
	tools := filterTools(typ, parentTools)

	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx) // the spawning tool_call — E3 anchor (empty for a fork skill not under a tool_call)
	runID := idgenpkg.New("subagt")
	subMsgID := idgenpkg.New("msg")

	// Open the sub-message (streaming) so its id anchors the live stream + persists the turn,
	// then emit message_start under the spawning tool_call (E3).
	//
	// 开 sub-message（streaming）使其 id 锚 live 流 + 落盘回合，再在派它的 tool_call 下发 message_start（E3）。
	subMsg := &messagesdomain.Message{
		ID:             subMsgID,
		ConversationID: convID,
		SubagentID:     runID,
		Role:           messagesdomain.RoleAssistant,
		Status:         messagesdomain.StatusStreaming,
		Provider:       bundle.Provider,
		ModelID:        bundle.Request.ModelID,
	}
	if toolCallID != "" {
		subMsg.Attrs = map[string]any{attrParentBlockID: toolCallID}
	}
	if err := s.deps.Messages.CreateMessage(ctx, subMsg, nil); err != nil {
		return "", fmt.Errorf("subagent: open sub-message: %w", err)
	}
	s.emitMessageStart(ctx, convID, subMsgID, toolCallID)

	// Sub-run context: mark it a subagent (recursion guard + todo scope), a fresh AgentState
	// (no SeenFiles/discovered pollution of the parent), and MessageID = subMsgID so loop's
	// blocks nest under the sub-message. Bridge / conversation / workspace / locale are inherited.
	//
	// 子运行 ctx：标记 subagent（递归守卫 + todo 作用域）、全新 AgentState（不污染父 SeenFiles/
	// discovered）、MessageID = subMsgID 使 loop 的 block 挂 sub-message 下。Bridge / conversation /
	// workspace / locale 继承。
	subCtx := reqctxpkg.SetSubagentID(ctx, runID)
	subCtx = reqctxpkg.WithAgentState(subCtx, agentstatepkg.New())
	subCtx = reqctxpkg.SetMessageID(subCtx, subMsgID)

	host := &subagentHost{
		svc:            s,
		conversationID: convID,
		subMsg:         subMsg,
		userPrompt:     prompt,
		systemPrompt:   composeSystemPrompt(typ, reqctxpkg.GetLocale(ctx)),
		tools:          tools,
	}
	req := bundle.Request
	req.System = host.systemPrompt

	result := loopapp.Run(subCtx, host, bundle.Client, req, typ.DefaultMaxTurns, s.log)
	return result.LastMessage, nil
}
