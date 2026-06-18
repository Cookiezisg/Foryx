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

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/anselm/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
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
	// lazy groups (search_tools), so ctx carries that activation state.
	//
	// Tools 每步重算：按需 host 随 LLM search_tools 扩张工具集，故 ctx 携带激活状态。
	Tools(ctx context.Context) []toolapp.Tool

	WriteFinalize(ctx context.Context, blocks []messagesdomain.Block, status, stopReason, errCode, errMsg string, in, out int)
}

// ReminderProvider is an OPTIONAL Host capability (type-asserted): when implemented, Run
// injects its system-reminders ahead of each step as transient user messages — the
// mechanism that keeps live state (the todo checklist) in front of the model without
// polluting persisted history. A host without a todo service simply doesn't implement it.
//
// ReminderProvider 是 Host 可选能力（type-asserted）：实现它时，Run 每步前把它的 system-reminder
// 作为临时 user 消息注入——把 live 状态（todo 清单）顶在模型眼前、又不污染持久历史的机制。
// 无 todo 服务的 host 不实现即可。
type ReminderProvider interface {
	SystemReminders(ctx context.Context) []string
}

// AutoActivator is an OPTIONAL Host capability (type-asserted): it activates the lazy group
// that contains a requested-but-inactive tool, so the LLM can call a build tool without
// remembering to search_tools first. Returns nil if the tool isn't in any lazy group.
//
// AutoActivator 是 Host 可选能力（type-asserted）：激活含「被点但未激活」工具的 lazy 组，使 LLM
// 调 build 工具前无需先 search_tools。工具不在任何 lazy 组时返回 nil。
type AutoActivator interface {
	TryActivateForTool(ctx context.Context, toolName string) []toolapp.Tool
}

// StepRecorder is an OPTIONAL Host capability (type-asserted): it journals each completed
// tool-step so a durable replay (workflow flowrun :replay) reconstructs history from the
// journal and skips already-completed steps. The chat host does NOT implement it.
// RecordStep is called only AFTER a step's tools ran and history extended — a crash before
// the call re-runs the whole step (at-least-once; tools must be idempotent).
//
// StepRecorder 是 Host 可选能力（type-asserted）：记账每个完成的 tool-step，供 flowrun :replay 从
// journal 重建历史、跳过已完成步。chat host 不实现。RecordStep 仅在某步工具跑完 + 历史
// 扩展后调用——调用前崩溃则整步重跑（at-least-once；工具须幂等）。
type StepRecorder interface {
	RecordStep(ctx context.Context, step int, assistant, toolResults []messagesdomain.Block)
}

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
}

// Run executes the ReAct loop. baseReq.Messages is composed from host.LoadHistory; tools are
// recomputed per step from host.Tools(ctx) so search_tools widens the set for later steps;
// any ReminderProvider's reminders are injected fresh each step. It always ends with exactly
// one host.WriteFinalize.
//
// Run 执行 ReAct 循环。baseReq.Messages 取自 host.LoadHistory；tools 每步从 host.Tools(ctx) 重算，
// 使 search_tools 为后续步扩张工具集；ReminderProvider 的 reminder 每步重新注入。总以恰一次
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
	)

	for step := range maxSteps {
		req := baseReq
		req.Messages = injectReminders(ctx, host, history)

		// Recompute per step: a prior search_tools may have widened the set. byName MUST
		// match this step's offered tools so dispatch can't resolve a tool the LLM wasn't
		// shown this turn.
		//
		// 每步重算：上一步的 search_tools 可能已扩张工具集。byName 必须与本步 offer 的集合
		// 一致，避免调度到本回合未展示给 LLM 的工具。
		tools := host.Tools(ctx)
		req.Tools = toolapp.ToLLMDefs(tools)
		byName := toolsByName(tools)

		stepsRun = step + 1

		// buildOf lets streamLLM recognize a create/edit tool_call (a BuildTool) and mirror its arg
		// delta onto the entities stream (SSE-C). Built per step from this step's byName.
		//
		// buildOf 让 streamLLM 识别 create/edit tool_call（BuildTool）并把 arg delta 镜像到 entities 流
		// （SSE-C）。每步据本步 byName 建。
		buildOf := func(name string) (toolapp.BuildSpec, bool) {
			if ft, ok := byName[name].(toolapp.BuildTool); ok {
				return ft.Build(), true
			}
			return toolapp.BuildSpec{}, false
		}
		aBlocks, toolCalls, sr, streamErr, iT, oT := streamLLM(ctx, client, req, buildOf, log)
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
				// A provider can end the stream with stopReason=error yet an empty message (e.g. a
				// silent disconnect). Surface a non-empty, actionable reason so the turn doesn't
				// finalize as a contentless "error" with no cause and no recovery hint for the user.
				//
				// provider 可能以 stopReason=error 但空消息收尾（如静默断连）。补一句非空、可操作的因，
				// 免回合 finalize 成无因无恢复提示的空 "error"。
				if errMsg == "" {
					errMsg = "the model stream ended unexpectedly before finishing — your work so far is saved; retry to continue"
				}
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

		rBlocks := runTools(ctx, toolCalls, byName, log)
		allBlocks = append(allBlocks, rBlocks...)

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

		// Sub-step replay: journal a fully-completed step so a future :replay
		// reconstructs history from here instead of re-running this step's LLM + tools.
		//
		// 子步重放：记账一个已完成步，使将来 :replay 从此处重建历史，而非重跑本步的
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
