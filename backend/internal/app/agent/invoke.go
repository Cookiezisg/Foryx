package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// InvokeInput is the request shape for InvokeAgent (mirrors functionapp.RunInput).
//
// InvokeInput 是 InvokeAgent 的请求形状（对标 functionapp.RunInput）。
type InvokeInput struct {
	AgentID     string
	VersionID   string         // empty → active version
	Input       map[string]any // data fed to the agent (appended to its prompt)
	TriggeredBy string         // chat | workflow | manual
	MaxTurns    int            // ReAct turn cap; 0 → default

	// Workflow-only (ADR-010 sub-step replay): a flowrun :replay prepends prior completed steps
	// and records new ones. All nil/empty for a standalone chat/manual invoke.
	FlowrunID     string
	FlowrunNodeID string
	ReplaySteps   []RecordedStep
	Recorder      StepRecorder
}

// RecordedStep is one completed ReAct step (assistant blocks + tool results) for replay.
//
// RecordedStep 是一个完成的 ReAct 步（assistant blocks + tool results），供重放重建。
type RecordedStep struct {
	Assistant   []messagesdomain.Block
	ToolResults []messagesdomain.Block
}

// StepRecorder journals a completed step at its absolute turn index (workflow durable replay).
//
// StepRecorder 在绝对回合下标记账一个完成的步（workflow 持久重放）。
type StepRecorder func(ctx context.Context, step int, assistant, toolResults []messagesdomain.Block)

// InvokeResult is the terminal output of InvokeAgent.
//
// InvokeResult 是 InvokeAgent 的终态输出。
type InvokeResult struct {
	ExecutionID string `json:"executionId"`
	OK          bool   `json:"ok"`
	Output      any    `json:"output"`
	Status      string `json:"status"`
	StopReason  string `json:"stopReason,omitempty"`
	Steps       int    `json:"steps"`
	TokensIn    int    `json:"tokensIn"`
	TokensOut   int    `json:"tokensOut"`
	ErrorMsg    string `json:"errorMsg,omitempty"`
	ElapsedMs   int64  `json:"elapsedMs"`
}

// InvokeAgent runs an agent's ReAct loop once and records one Execution (mirrors function
// RunFunction: the single execution method every path — invoke_agent tool / HTTP :invoke /
// workflow agent node — funnels through, so every run lands in agent_executions).
//
// InvokeAgent 跑一次 agent ReAct loop 并记一条 Execution（对标 function.RunFunction：所有触发路径
// 都经此方法，每次执行都落表）。
func (s *Service) InvokeAgent(ctx context.Context, in InvokeInput) (*InvokeResult, error) {
	if s.invoke.Resolver == nil {
		return nil, fmt.Errorf("agentapp.InvokeAgent: invoke deps not configured (call SetInvokeDeps)")
	}

	a, err := s.repo.Get(ctx, in.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.InvokeAgent: %w", err)
	}
	versionID := in.VersionID
	if versionID == "" {
		if a.ActiveVersionID == "" {
			return nil, fmt.Errorf("agentapp.InvokeAgent: %w", agentdomain.ErrNoActiveVersion)
		}
		versionID = a.ActiveVersionID
	}
	v, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.InvokeAgent: version: %w", err)
	}

	startedAt := time.Now().UTC()
	result, modelID, runErr := s.runLoop(ctx, a, v, in)
	endedAt := time.Now().UTC()

	res := &InvokeResult{
		Status:    agentdomain.ExecutionStatusOK,
		ElapsedMs: endedAt.Sub(startedAt).Milliseconds(),
	}
	if runErr != nil {
		res.Status = agentdomain.ExecutionStatusFailed
		res.ErrorMsg = runErr.Error()
	} else {
		res.OK = result.Status != messagesdomain.StatusError
		if !res.OK {
			res.Status = agentdomain.ExecutionStatusFailed
			res.ErrorMsg = "agent loop error"
		}
		res.Output = result.LastMessage
		res.StopReason = result.StopReason
		res.Steps = result.Steps
		res.TokensIn = result.TokensIn
		res.TokensOut = result.TokensOut
	}

	res.ExecutionID = s.recordExecution(ctx, in, a, v, res, modelID, result.Blocks, startedAt, endedAt)
	return res, nil
}

// runLoop builds the agent host + LLM bundle and runs app/loop.Run (the ReAct loop). The loop's
// emitter streams blocks to whatever stream scope ctx carries (eventlog when invoked in chat) —
// agent writes no stream code.
//
// runLoop 构造 agent host + LLM bundle 并跑 app/loop.Run（ReAct 循环）。loop 的 emitter 把 block
// 推到 ctx 携带的 stream scope（chat 内调用时即 eventlog）——agent 不写流式代码。
func (s *Service) runLoop(ctx context.Context, a *agentdomain.Agent, v *agentdomain.Version, in InvokeInput) (loopapp.Result, string, error) {
	// Knowledge prefix (the agent's attached docs) prepended to the user message. A mounted
	// capability with its dep unwired is a wiring bug — fail loudly, never run degraded.
	// Knowledge 前缀（agent 挂的文档）前置到 user 消息。挂了能力而依赖未装配是装配 bug——大声失败、绝不降级跑。
	prefix := ""
	if len(v.Knowledge) > 0 {
		if s.invoke.Knowledge == nil {
			return loopapp.Result{}, "", fmt.Errorf("agent mounts knowledge but no KnowledgeProvider is wired")
		}
		p, kErr := s.invoke.Knowledge.BuildKnowledgePrefix(ctx, v.Knowledge)
		if kErr != nil {
			return loopapp.Result{}, "", fmt.Errorf("resolve knowledge: %w", kErr)
		}
		prefix = p
	}
	userMsg := prefix + v.Prompt
	if len(in.Input) > 0 {
		b, _ := json.Marshal(in.Input)
		userMsg += "\n\nInput data:\n```json\n" + string(b) + "\n```"
	}

	// Synthesize the version's mounts (fn_/hd_…method/mcp:server/tool) into bound tools — the
	// agent's entire tool universe. Fail-fast: a deleted/renamed-away target fails the invoke
	// (a worker missing a declared capability must not run silently degraded).
	// 把版本挂载（fn_/hd_…method/mcp:server/tool）合成绑定工具——agent 的全部工具宇宙。fail-fast：
	// 目标被删/不在线即 invoke 失败（worker 缺声明能力绝不静默降级跑）。
	var tools []toolapp.Tool
	if len(v.Tools) > 0 {
		if s.invoke.Mounts == nil {
			return loopapp.Result{}, "", fmt.Errorf("agent mounts tools but no MountResolver is wired")
		}
		var mErr error
		tools, mErr = s.invoke.Mounts.Resolve(ctx, v.Tools)
		if mErr != nil {
			return loopapp.Result{}, "", fmt.Errorf("resolve mounts: %w", mErr)
		}
	}

	// The mounted skill renders into the system prompt as the run's execution guide.
	// 挂载的 skill 渲染进 system prompt，作为本次运行的执行指南。
	skillGuide := ""
	if v.Skill != "" {
		if s.invoke.Skill == nil {
			return loopapp.Result{}, "", fmt.Errorf("agent mounts skill %q but no SkillGuide is wired", v.Skill)
		}
		g, gErr := s.invoke.Skill.Guide(ctx, v.Skill)
		if gErr != nil {
			return loopapp.Result{}, "", fmt.Errorf("resolve skill: %w", gErr)
		}
		skillGuide = g
	}

	bundle, err := s.invoke.Resolver.ResolveAgent(ctx, v.ModelOverride)
	if err != nil {
		return loopapp.Result{}, "", fmt.Errorf("resolve LLM: %w", err)
	}

	host := &agentHost{
		userPrompt: userMsg,
		tools:      tools,
		replay:     in.ReplaySteps,
		recorder:   in.Recorder,
		log:        s.log,
	}

	req := bundle.Request
	req.System = buildSystemPrompt(a, v, skillGuide)

	maxTurns := in.MaxTurns
	if maxTurns <= 0 {
		maxTurns = limitspkg.Current().Agent.InvokeMaxTurns
	}
	remaining := maxTurns - len(in.ReplaySteps)
	if remaining < 1 {
		remaining = 1
	}

	// Chat surfacing (E3): invoked as a tool in a chat turn, nest the agent's streamed blocks under
	// the invoke_agent tool_call so the front end shows the run inline as the tool's intermediate.
	// These blocks are stream-only — the durable record is the Execution transcript, NOT
	// message_blocks. Outside chat (no tool_call) this is a no-op and nothing streams.
	//
	// chat 呈现（E3）：在 chat turn 内作为 tool 调起时，把 agent 的流式 block 嵌在 invoke_agent tool_call
	// 下，使前端把运行内联呈现为该 tool 的中间过程。这些 block 仅流——耐久记录是 Execution transcript、**非**
	// message_blocks。不在 chat（无 tool_call）则 no-op、不流。
	if tcID, ok := reqctxpkg.GetToolCallID(ctx); ok && tcID != "" {
		ctx = reqctxpkg.SetMessageID(ctx, tcID)
	}

	// SSE-C: mirror this run's ReAct trace (every block) onto the entities stream scoped to the
	// agent, so the agent panel shows the run live regardless of caller (chat / REST / workflow).
	// nil bridge → no-op.
	//
	// SSE-C：把本次运行的 ReAct 轨迹（每个 block）镜像到 agent scope 的 entities 流，使 agent 面板实时显示运行
	// （与谁触发无关——chat / REST / workflow）。nil bridge → no-op。
	ctx = entitystreamapp.WithBridge(ctx, s.invoke.EntitiesBridge)
	ctx = entitystreamapp.WithRunScope(ctx, streamdomain.Scope{Kind: streamdomain.KindAgent, ID: a.ID})

	result := loopapp.Run(ctx, host, bundle.Client, req, remaining, s.log)
	return result, req.ModelID, nil
}

// recordExecution writes one terminal Execution row (best-effort, on a detached ctx that keeps
// workspace so a cancelled run still persists the record). Mirrors functionapp.recordExecution.
//
// recordExecution 写一行终态 Execution（best-effort，用保留 workspace 的 detached ctx，使被取消的
// 运行仍落账）。对标 functionapp.recordExecution。
func (s *Service) recordExecution(ctx context.Context, in InvokeInput, a *agentdomain.Agent, v *agentdomain.Version, res *InvokeResult, modelID string, blocks []messagesdomain.Block, startedAt, endedAt time.Time) string {
	triggeredBy := in.TriggeredBy
	if !agentdomain.IsValidTrigger(triggeredBy) {
		triggeredBy = agentdomain.TriggeredByManual
	}
	input := in.Input
	if input == nil {
		input = map[string]any{}
	}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	// Flowrun identity: the explicit InvokeInput fields win (ADR-010 sub-step replay passes them);
	// otherwise the scheduler's ctx injection covers the plain workflow-dispatch path.
	// Flowrun 身份：显式 InvokeInput 字段优先（ADR-010 子步重放会传）；否则调度器的 ctx 注入覆盖普通
	// workflow 派发路径。
	flowrunID, flowrunNodeID := in.FlowrunID, in.FlowrunNodeID
	if flowrunID == "" {
		flowrunID, _ = reqctxpkg.GetFlowrunID(ctx)
	}
	if flowrunNodeID == "" {
		flowrunNodeID, _ = reqctxpkg.GetFlowrunNodeID(ctx)
	}

	// The full block transcript is this run's self-contained durable record (NOT persisted to the
	// shared message_blocks table). Always at least "[]" so the column never holds null.
	//
	// 完整 block transcript 是本次运行自包含的耐久记录（**不**落共享的 message_blocks 表）。至少 "[]"，使列永不为 null。
	transcript, err := json.Marshal(blocks)
	if err != nil || len(transcript) == 0 {
		transcript = []byte("[]")
	}

	exec := &agentdomain.Execution{
		ID:             idgenpkg.New("agx"),
		AgentID:        a.ID,
		VersionID:      v.ID,
		ModelID:        modelID,
		Status:         res.Status,
		TriggeredBy:    triggeredBy,
		Input:          input,
		Output:         res.Output,
		Transcript:     transcript,
		ErrorMessage:   res.ErrorMsg,
		ElapsedMs:      res.ElapsedMs,
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		FlowrunID:      flowrunID,
		FlowrunNodeID:  flowrunNodeID,
	}

	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	detached := reqctxpkg.Detached(wsID)
	if err := s.repo.SaveExecution(detached, exec); err != nil {
		s.log.Warn("agentapp.recordExecution: save failed (best-effort)",
			zap.String("agentId", a.ID), zap.String("versionId", v.ID), zap.Error(err))
		return ""
	}
	return exec.ID
}

// buildSystemPrompt composes the agent identity + worker discipline + the mounted skill's
// execution guide + the outputSchema instruction.
//
// buildSystemPrompt 组装 agent 身份 + worker 纪律 + 挂载 skill 的执行指南 + outputSchema 指令。
func buildSystemPrompt(a *agentdomain.Agent, v *agentdomain.Version, skillGuide string) string {
	identity := "You are a workflow automation worker."
	if a.Name != "" {
		identity = "You are " + a.Name + ", a workflow automation worker."
		if a.Description != "" {
			identity += " Your role: " + a.Description
		}
	}
	prompt := identity +
		" Use available tools as needed; respond concisely when finished." +
		" Only use the tools explicitly provided to you. Do not attempt capabilities you have no tool for."
	if skillGuide != "" {
		prompt += "\n\n## Execution guide (skill: " + v.Skill + ")\n\n" + skillGuide
	}
	return prompt + outputsInstruction(v.Outputs)
}

// agentHost is the per-invoke loop.Host: history is the prompt (+ replay), Tools is the
// pre-filtered whitelist, WriteFinalize is a no-op (agent runs persist via Execution, not
// message history), and RecordStep journals new steps when a recorder is wired (workflow).
//
// agentHost 是每次 invoke 的 loop.Host：history 即 prompt（+ 重放），Tools 是预过滤白名单，
// WriteFinalize 为 no-op（agent 运行经 Execution 落账、非消息历史），RecordStep 在装了 recorder 时
// 记新步（workflow）。
type agentHost struct {
	userPrompt string
	tools      []toolapp.Tool
	replay     []RecordedStep
	recorder   StepRecorder
	log        *zap.Logger
}

func (h *agentHost) LoadHistory(_ context.Context) ([]llminfra.LLMMessage, error) {
	history := []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: h.userPrompt}}
	for _, step := range h.replay {
		blocks := append(append([]messagesdomain.Block{}, step.Assistant...), step.ToolResults...)
		history = append(history, loopapp.BlocksToAssistantLLM(blocks)...)
	}
	return history, nil
}

func (h *agentHost) Tools(_ context.Context) []toolapp.Tool { return h.tools }

func (h *agentHost) WriteFinalize(_ context.Context, _ []messagesdomain.Block, _, _, _, _ string, _, _ int) {
}

// RecordStep implements loop.StepRecorder (type-asserted by Run); forwards to the wired
// recorder at the absolute turn index (replay offset + step).
//
// RecordStep 实现 loop.StepRecorder（被 Run type-assert）；按绝对回合下标（重放偏移 + step）转给
// 装入的 recorder。
func (h *agentHost) RecordStep(ctx context.Context, step int, assistant, toolResults []messagesdomain.Block) {
	if h.recorder != nil {
		h.recorder(ctx, len(h.replay)+step, assistant, toolResults)
	}
}

// outputsInstruction renders the agent's declared output fields as a hard instruction
// appended to the system prompt, so the LLM's final answer is a single JSON object with those
// fields. No declared outputs → no instruction (the agent answers free-form).
//
// outputsInstruction 把 agent 声明的输出字段渲成追加 system prompt 的硬约束，使 LLM 最终答案是带
// 这些字段的单个 JSON 对象。无声明 → 无约束（agent 自由作答）。
func outputsInstruction(fields []schemapkg.Field) string {
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nYour FINAL answer must be a single JSON object with exactly these fields (output only the JSON, no prose):")
	for _, f := range fields {
		fmt.Fprintf(&b, "\n  - %s (%s)", f.Name, f.Type)
		if f.Description != "" {
			b.WriteString(": " + f.Description)
		}
	}
	return b.String()
}
