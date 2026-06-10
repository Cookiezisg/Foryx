// Package loop is the shared ReAct engine: stream LLM → dispatch tools → extend history →
// finalize, looped until the model stops calling tools or a ceiling trips. It is consumed
// by chat / agent / subagent / workflow-agent via the Host interface, and depends only on
// the neutral content model (domain/messages), the tool contract (app/tool), the LLM port
// (infra/llm), and the messages stream (domain/stream) — never on a specific consumer.
//
// Package loop 是共享 ReAct 引擎：流式调 LLM → 派发工具 → 扩展历史 → 终态，循环至模型停止调用
// 工具或触顶。经 Host 接口被 chat / agent / subagent / workflow-agent 消费，只依赖中立内容模型
// （domain/messages）、工具契约（app/tool）、LLM 端口（infra/llm）、messages 流（domain/stream）——
// 绝不依赖某个具体消费者。
package loop

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// maxConsecutiveAllFailTurns caps how many turns in a row may end with every tool call
// returning an error before the loop aborts (TOOL_ERROR_STORM). Three is the lowest value
// that still gives the LLM room to self-correct from a single mistake — burn-in saw the LLM
// build 4 orphan handlers before giving up; this cap stops similar drift early.
//
// maxConsecutiveAllFailTurns 限定连续多少轮全员失败后熔断（TOOL_ERROR_STORM）。3 是给 LLM 自纠
// 机会的最小值；burn-in 撞过 LLM 连建 4 个废 handler 才放弃，此熔断早停类似漂移。
const maxConsecutiveAllFailTurns = 3

// Host is the per-run hook surface: the loop asks it for the starting history and the
// current tool set, and hands it the terminal write. Block persistence is the host's job —
// the loop produces blocks in memory and streams them live; finalize is where they land.
//
// Host 是每次 run 的钩子面：loop 向它要起始历史与当前工具集，并把终态写交给它。block 持久化是
// host 的事——loop 内存产 block 并实时推流；终态落盘在此发生。
type Host interface {
	LoadHistory(ctx context.Context) ([]llminfra.LLMMessage, error)

	// Tools is recomputed every step: on-demand hosts widen the set as the LLM activates
	// lazy groups (activate_tools), so ctx carries that activation state.
	//
	// Tools 每步重算：按需 host 随 LLM activate_tools 扩张工具集，故 ctx 携带激活状态。
	Tools(ctx context.Context) []toolapp.Tool

	WriteFinalize(ctx context.Context, blocks []messagesdomain.Block, status, stopReason, errCode, errMsg string, in, out int)
}

// ReminderProvider is an OPTIONAL Host capability (type-asserted): when implemented, Run
// injects its system-reminders ahead of each step as transient user messages — the
// mechanism that keeps live state (the todo checklist, M1.11) in front of the model without
// polluting persisted history. A host without a todo service simply doesn't implement it.
//
// ReminderProvider 是 Host 可选能力（type-asserted）：实现它时，Run 每步前把它的 system-reminder
// 作为临时 user 消息注入——把 live 状态（todo 清单 M1.11）顶在模型眼前、又不污染持久历史的机制。
// 无 todo 服务的 host 不实现即可。
type ReminderProvider interface {
	SystemReminders(ctx context.Context) []string
}

// AutoActivator is an OPTIONAL Host capability (type-asserted): it activates the lazy group
// that contains a requested-but-inactive tool, so the LLM can call a forge tool without
// remembering to activate_tools first. Returns nil if the tool isn't in any lazy group.
//
// AutoActivator 是 Host 可选能力（type-asserted）：激活含「被点但未激活」工具的 lazy 组，使 LLM
// 调 forge 工具前无需先 activate_tools。工具不在任何 lazy 组时返回 nil。
type AutoActivator interface {
	TryActivateForTool(ctx context.Context, toolName string) []toolapp.Tool
}

// StepRecorder is an OPTIONAL Host capability (type-asserted): it journals each completed
// tool-step so a durable replay (workflow flowrun :replay) reconstructs history from the
// journal and skips already-completed steps (ADR-010). The chat host does NOT implement it.
// RecordStep is called only AFTER a step's tools ran and history extended — a crash before
// the call re-runs the whole step (at-least-once; tools must be idempotent).
//
// StepRecorder 是 Host 可选能力（type-asserted）：记账每个完成的 tool-step，供 flowrun :replay 从
// journal 重建历史、跳过已完成步（ADR-010）。chat host 不实现。RecordStep 仅在某步工具跑完 + 历史
// 扩展后调用——调用前崩溃则整步重跑（at-least-once；工具须幂等）。
type StepRecorder interface {
	RecordStep(ctx context.Context, step int, assistant, toolResults []messagesdomain.Block)
}

// ParkHandler is an OPTIONAL Host capability (type-asserted): implementing it OPTS THE RUN INTO
// human-in-the-loop parking (R0064). When present, the loop parks — finalizes the partial turn as
// `parked` and returns the pending requests in Result.Parks — on a dangerous tool call or an
// InteractiveTool (ask_user) call. AllowsTool lets the host skip the danger park for a tool the
// user session-whitelisted (always-allow); it never suppresses an ask. A host WITHOUT this
// capability never parks: dangerous tools run (pure trust) and ask_user is unreachable — correct
// for a non-interactive host (subagent / workflow-agent, which has no interactive approver).
//
// ParkHandler 是 Host 可选能力（type-asserted）：实现它即让本次运行 opt-in 人在环 park（R0064）。在场时，
// loop 在危险工具调用或 InteractiveTool（ask_user）调用处 park——把半截回合落成 `parked` 并在 Result.Parks
// 返回待决请求。AllowsTool 让 host 对用户会话白名单的工具跳过 danger park（always-allow）；绝不抑制 ask。
// 无此能力的 host 永不 park：危险工具照跑（纯信任）、ask_user 不可达——非交互 host（subagent / workflow-agent，
// 无交互审批人）正确。
type ParkHandler interface {
	AllowsTool(name string) bool
}

// ParkRequest is one pending human interaction surfaced when the loop parks: the tool_call that
// triggered it, why (Kind), and the call's name + raw args (the danger gate's gated call, or the
// ask_user elicitation request). The DURABLE record is the parked message's pending tool_result
// block keyed by ToolCallID; this struct is the in-memory hand-off to the host's caller for
// surfacing (notifications) — resolution reads the durable row, not this.
//
// ParkRequest 是 loop park 时露出的一条待决人机交互：触发的 tool_call、原因（Kind）、调用名 + 裸 args
// （danger 门控的被门调用，或 ask_user 的 elicitation 请求）。耐久记录是 parked message 下按 ToolCallID 键的
// pending tool_result 块；本结构是交给 host 调用方做露出（通知）的内存交接——决议读耐久行、非此。
type ParkRequest struct {
	ToolCallID string
	Kind       string // ParkKindAsk | ParkKindDanger
	ToolName   string
	Args       string // the call's raw JSON args
}

// Park kinds.
//
// Park 种类。
const (
	ParkKindAsk    = "ask"
	ParkKindDanger = "danger"
)

// Resolution verbs — how a human resolves a parked interaction (R0064). Shared by every parking
// host (chat / agent) so the wire contract is one vocabulary. danger: approve | deny; ask:
// accept | decline; either: cancel (abandon the run).
//
// 决议动词——人如何决议一个 parked 交互（R0064）。每个 parking host（chat / agent）共用，使线缆契约是一套词表。
// danger：approve | deny；ask：accept | decline；两者：cancel（放弃运行）。
const (
	ResolveApprove = "approve" // danger: run the gated tool
	ResolveDeny    = "deny"    // danger: skip it, feed the denial back to the model
	ResolveAccept  = "accept"  // ask: submit the answer
	ResolveDecline = "decline" // ask: refuse to answer, feed back
	ResolveCancel  = "cancel"  // either: abandon the whole parked run
)

// Result is the terminal summary of one Run.
//
// Result 是一次 Run 的终态汇总。
type Result struct {
	Blocks      []messagesdomain.Block
	Status      string
	StopReason  string
	TokensIn    int
	TokensOut   int
	Steps       int
	LastMessage string
	// Parks (non-nil only when Status == parked) lists the pending human interactions the host's
	// caller must surface; their durable form is the parked message's pending tool_result blocks.
	//
	// Parks（仅 Status==parked 时非空）列出 host 调用方须露出的待决人机交互；其耐久形态是 parked message 的
	// pending tool_result 块。
	Parks []ParkRequest
}

// Run executes the ReAct loop. baseReq.Messages is composed from host.LoadHistory; tools are
// recomputed per step from host.Tools(ctx) so activate_tools widens the set for later steps;
// any ReminderProvider's reminders are injected fresh each step. It always ends with exactly
// one host.WriteFinalize.
//
// Run 执行 ReAct 循环。baseReq.Messages 取自 host.LoadHistory；tools 每步从 host.Tools(ctx) 重算，
// 使 activate_tools 为后续步扩张工具集；ReminderProvider 的 reminder 每步重新注入。总以恰一次
// host.WriteFinalize 收尾。
func Run(
	ctx context.Context,
	host Host,
	client llminfra.Client,
	baseReq llminfra.Request,
	maxSteps int,
	log *zap.Logger,
) Result {
	if log == nil {
		log = zap.NewNop()
	}

	history, err := host.LoadHistory(ctx)
	if err != nil {
		host.WriteFinalize(ctx, nil,
			messagesdomain.StatusError, messagesdomain.StopReasonError,
			"INTERNAL_ERROR", "load history: "+err.Error(), 0, 0)
		return Result{Status: messagesdomain.StatusError, StopReason: messagesdomain.StopReasonError}
	}

	var (
		allBlocks     []messagesdomain.Block
		totalIn       int
		totalOut      int
		stopReason    = messagesdomain.StopReasonEndTurn
		finalStatus   = messagesdomain.StatusCompleted
		errCode       string
		errMsg        string
		finalWritten  bool
		stepsRun      int
		consecAllFail int
		parkReqs      []ParkRequest
	)

	// Human-in-the-loop parking is enabled only for a host that opts in (ParkHandler). allowsTool
	// is the always-allow session whitelist (skip the danger park); absent → nothing whitelisted.
	//
	// 人在环 park 仅对 opt-in 的 host（ParkHandler）启用。allowsTool 是 always-allow 会话白名单（跳过 danger
	// park）；缺省 → 无白名单。
	ph, parkEnabled := host.(ParkHandler)
	allowsTool := func(string) bool { return false }
	if parkEnabled {
		allowsTool = ph.AllowsTool
	}

	for step := range maxSteps {
		req := baseReq
		req.Messages = injectReminders(ctx, host, history)

		// Recompute per step: a prior activate_tools may have widened the set. byName MUST
		// match this step's offered tools so dispatch can't resolve a tool the LLM wasn't
		// shown this turn.
		//
		// 每步重算：上一步的 activate_tools 可能已扩张工具集。byName 必须与本步 offer 的集合
		// 一致，避免调度到本回合未展示给 LLM 的工具。
		tools := host.Tools(ctx)
		req.Tools = toolapp.ToLLMDefs(tools)
		byName := toolsByName(tools)

		stepsRun = step + 1

		// forgeOf lets streamLLM recognize a create/edit tool_call (a ForgeTool) and mirror its arg
		// delta onto the entities stream (SSE-C). Built per step from this step's byName.
		//
		// forgeOf 让 streamLLM 识别 create/edit tool_call（ForgeTool）并把 arg delta 镜像到 entities 流
		// （SSE-C）。每步据本步 byName 建。
		forgeOf := func(name string) (toolapp.ForgeSpec, bool) {
			if ft, ok := byName[name].(toolapp.ForgeTool); ok {
				return ft.Forge(), true
			}
			return toolapp.ForgeSpec{}, false
		}
		aBlocks, toolCalls, sr, streamErr, iT, oT := streamLLM(ctx, client, req, forgeOf, log)
		allBlocks = append(allBlocks, aBlocks...)
		totalIn += iT
		totalOut += oT
		if sr != "" {
			stopReason = sr
		}

		if stopReason == messagesdomain.StopReasonCancelled || stopReason == messagesdomain.StopReasonError {
			status := messagesdomain.StatusCancelled
			if stopReason == messagesdomain.StopReasonError {
				status = messagesdomain.StatusError
				errCode = "LLM_STREAM_ERROR"
				errMsg = streamErr
			}
			finalStatus = status
			host.WriteFinalize(ctx, allBlocks, status, stopReason, errCode, errMsg, totalIn, totalOut)
			finalWritten = true
			break
		}

		if len(toolCalls) == 0 {
			host.WriteFinalize(ctx, allBlocks, messagesdomain.StatusCompleted, stopReason, "", "", totalIn, totalOut)
			finalWritten = true
			break
		}

		// Auto-activate: if a requested tool isn't in the current set but an AutoActivator
		// host can find it in a lazy group, activate that group and rebuild byName.
		//
		// auto-activate：被点工具不在当前集合，但 AutoActivator host 能在某 lazy 组找到它，则激活
		// 该组并重建 byName。
		if aa, ok := host.(AutoActivator); ok {
			for _, tc := range toolCalls {
				if _, found := byName[tc.Name]; !found {
					if newTools := aa.TryActivateForTool(ctx, tc.Name); newTools != nil {
						byName = toolsByName(newTools)
					}
				}
			}
		}

		rBlocks, parks := runTools(ctx, toolCalls, byName, parkEnabled, allowsTool, log)
		allBlocks = append(allBlocks, rBlocks...)

		// Park (R0064): a dangerous call or an ask_user call wrote a pending tool_result instead of
		// running. Finalize the partial turn as `parked` (durable; the inbox is parked messages) and
		// hand the pending requests back for the host's caller to surface. A continuation turn resumes
		// once they resolve.
		//
		// Park（R0064）：危险调用或 ask_user 调用写了 pending tool_result 而非执行。把半截回合落成 `parked`
		// （耐久；收件箱即 parked message），并把待决请求交回供 host 调用方露出。决议后续跑回合恢复。
		if len(parks) > 0 {
			parkReqs = parks
			finalStatus = messagesdomain.StatusParked
			stopReason = messagesdomain.StopReasonParked
			host.WriteFinalize(ctx, allBlocks, finalStatus, stopReason, "", "", totalIn, totalOut)
			finalWritten = true
			break
		}

		// Consecutive-all-fail circuit breaker: count turns where every tool_result carries
		// an error. Three in a row breaks the loop to stop the LLM drilling deeper into a
		// stuck state (e.g. repeatedly creating broken handlers).
		//
		// 连续全失败熔断：统计每轮 tool_result 全带 error 的次数。3 次连续即熔断，防 LLM 在卡壳
		// 状态越钻越深（如反复造废 handler）。
		if len(rBlocks) > 0 {
			allFailed := true
			for _, b := range rBlocks {
				if b.Type == messagesdomain.BlockTypeToolResult && b.Error == "" {
					allFailed = false
					break
				}
			}
			if allFailed {
				consecAllFail++
				if consecAllFail >= maxConsecutiveAllFailTurns {
					stopReason = messagesdomain.StopReasonError
					errCode = "TOOL_ERROR_STORM"
					errMsg = fmt.Sprintf("%d consecutive turns where every tool call failed; aborting to prevent runaway", consecAllFail)
					finalStatus = messagesdomain.StatusError
					host.WriteFinalize(ctx, allBlocks, messagesdomain.StatusError, stopReason, errCode, errMsg, totalIn, totalOut)
					finalWritten = true
					break
				}
			} else {
				consecAllFail = 0
			}
		}

		history = extendHistory(history, aBlocks, rBlocks)

		// Sub-step replay (ADR-010): journal a fully-completed step so a future :replay
		// reconstructs history from here instead of re-running this step's LLM + tools.
		//
		// 子步重放（ADR-010）：记账一个已完成步，使将来 :replay 从此处重建历史，而非重跑本步的
		// LLM + 工具。
		if rec, ok := host.(StepRecorder); ok {
			rec.RecordStep(ctx, step, aBlocks, rBlocks)
		}

		log.Debug("react step complete", zap.Int("step", step))
	}

	if !finalWritten {
		// maxSteps exhausted while the model still wanted to act. Surface this honestly — a
		// non-success terminal + a distinct stop_reason — instead of masquerading as a
		// completed turn. The work isn't lost; the UI offers "continue" off MAX_STEPS_REACHED.
		//
		// maxSteps 耗尽但模型还想动作。诚实暴露——非成功终态 + 独立 stop_reason，不冒充 completed。
		// 工作没丢；UI 凭 MAX_STEPS_REACHED 提供「继续」。
		stopReason = messagesdomain.StopReasonMaxSteps
		finalStatus = messagesdomain.StatusError
		errCode = "MAX_STEPS_REACHED"
		errMsg = fmt.Sprintf("reached the step limit (%d) before finishing; continue to resume", maxSteps)
		host.WriteFinalize(ctx, allBlocks, finalStatus, stopReason, errCode, errMsg, totalIn, totalOut)
	}

	return Result{
		Blocks:      allBlocks,
		Status:      finalStatus,
		StopReason:  stopReason,
		TokensIn:    totalIn,
		TokensOut:   totalOut,
		Steps:       stepsRun,
		LastMessage: ExtractTextContent(allBlocks),
		Parks:       parkReqs,
	}
}

// injectReminders appends any ReminderProvider reminders as transient <system-reminder> user
// messages onto a COPY of history, so each step sees current live state and the persisted
// history stays clean. No provider / no reminders → history is returned unchanged.
//
// injectReminders 把 ReminderProvider 的 reminder 作为临时 <system-reminder> user 消息追加到
// history 的**副本**上，使每步看到当前 live 状态而持久历史保持干净。无 provider / 无 reminder →
// 原样返回 history。
func injectReminders(ctx context.Context, host Host, history []llminfra.LLMMessage) []llminfra.LLMMessage {
	rp, ok := host.(ReminderProvider)
	if !ok {
		return history
	}
	reminders := rp.SystemReminders(ctx)
	if len(reminders) == 0 {
		return history
	}
	out := make([]llminfra.LLMMessage, len(history), len(history)+len(reminders))
	copy(out, history)
	for _, r := range reminders {
		out = append(out, llminfra.LLMMessage{
			Role:    llminfra.RoleUser,
			Content: "<system-reminder>\n" + r + "\n</system-reminder>",
		})
	}
	return out
}

func toolsByName(tools []toolapp.Tool) map[string]toolapp.Tool {
	m := make(map[string]toolapp.Tool, len(tools))
	for _, t := range tools {
		m[t.Name()] = t
	}
	return m
}
