// Package loop is the shared ReAct engine (stream → tool dispatch → history → finalize).
//
// Package loop 是共享的 ReAct 引擎（流 → 工具调度 → 历史 → 终态）。
package loop

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// maxConsecutiveAllFailTurns caps how many turns in a row may end with every
// tool call returning an error before the loop aborts (TOOL_ERROR_STORM).
// Three is the lowest value that still gives the LLM room to self-correct from
// a single mistake — burn-in v2 saw the LLM build 4 orphan handlers before
// giving up; this cap stops similar drift early.
//
// maxConsecutiveAllFailTurns 限定连续多少轮全员失败后熔断（TOOL_ERROR_STORM）。
// 3 是给 LLM 自纠机会的最小值；burn-in v2 撞过 LLM 连建 4 个废 handler 才放弃,
// 此熔断早停类似漂移。
const maxConsecutiveAllFailTurns = 3

// Host is the per-run hook surface; block persistence happens via the eventlog Emitter, not Host.
//
// Host 是每次 run 的钩子面；block 持久化走 eventlog Emitter，不经 Host。
type Host interface {
	LoadHistory(ctx context.Context) ([]llminfra.LLMMessage, error)
	// Tools is recomputed every step: on-demand hosts widen the set as the
	// LLM activates lazy groups (activate_tools), so ctx carries that state.
	//
	// Tools 每步重算：按需 host 随 LLM activate_tools 扩张工具集，ctx 携带该状态。
	Tools(ctx context.Context) []toolapp.Tool
	WriteFinalize(ctx context.Context, blocks []chatdomain.Block, status, stopReason, errCode, errMsg string, in, out int)
}

type Result struct {
	Blocks      []chatdomain.Block
	Status      string
	StopReason  string
	TokensIn    int
	TokensOut   int
	Steps       int
	LastMessage string
}

// Run executes the ReAct loop, composing baseReq.Messages from host.LoadHistory; Tools are
// recomputed per step from host.Tools(ctx) so activate_tools widens the set for later steps.
//
// Run 执行 ReAct 循环；baseReq.Messages 取自 host.LoadHistory；Tools 每步从 host.Tools(ctx)
// 重算，使 activate_tools 能为后续步骤扩张工具集。
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
			chatdomain.StatusError, chatdomain.StopReasonError,
			"INTERNAL_ERROR", "load history: "+err.Error(), 0, 0)
		return Result{Status: chatdomain.StatusError, StopReason: chatdomain.StopReasonError}
	}

	var (
		allBlocks     []chatdomain.Block
		totalIn       int
		totalOut      int
		stopReason    = chatdomain.StopReasonEndTurn
		finalStatus   = chatdomain.StatusCompleted
		errCode       string
		errMsg        string
		finalWritten  bool
		stepsRun      int
		consecAllFail int
	)

	for step := range maxSteps {
		req := baseReq
		req.Messages = history

		// Recompute per step: a prior activate_tools may have widened the set.
		// byName MUST match this step's offered tools so dispatch can't resolve
		// a tool the LLM wasn't shown this turn.
		//
		// 每步重算：上一步的 activate_tools 可能已扩张工具集。byName 必须与本步
		// offer 的集合一致，避免调度到本回合未展示给 LLM 的工具。
		tools := host.Tools(ctx)
		req.Tools = toolapp.ToLLMDefs(tools)
		byName := toolsByName(tools)

		stepsRun = step + 1

		aBlocks, toolCalls, sr, em, iT, oT := streamLLM(ctx, client, req)
		allBlocks = append(allBlocks, aBlocks...)
		totalIn += iT
		totalOut += oT
		if sr != "" {
			stopReason = sr
		}

		if stopReason == chatdomain.StopReasonCancelled || stopReason == chatdomain.StopReasonError {
			status := chatdomain.StatusCancelled
			if stopReason == chatdomain.StopReasonError {
				status = chatdomain.StatusError
				errCode = "LLM_STREAM_ERROR"
				errMsg = em
			}
			finalStatus = status
			host.WriteFinalize(ctx, allBlocks, status, stopReason, errCode, errMsg, totalIn, totalOut)
			finalWritten = true
			break
		}

		if len(toolCalls) == 0 {
			host.WriteFinalize(ctx, allBlocks, chatdomain.StatusCompleted, stopReason, "", "", totalIn, totalOut)
			finalWritten = true
			break
		}

		rBlocks := runTools(ctx, toolCalls, byName, log)
		allBlocks = append(allBlocks, rBlocks...)

		// Consecutive-all-fail circuit breaker: count turns where every
		// tool_result has a non-empty Error. Three in a row breaks the loop
		// to prevent the LLM from drilling deeper into a stuck state (e.g.
		// repeatedly creating broken handlers).
		//
		// 连续全失败熔断:统计每轮 tool_result 全部带 Error 的次数,3 次连续即
		// 熔断,防 LLM 在卡壳状态下越钻越深(如反复造废 handler)。
		if len(rBlocks) > 0 {
			allFailed := true
			for _, b := range rBlocks {
				if b.Type == eventlogdomain.BlockTypeToolResult && b.Error == "" {
					allFailed = false
					break
				}
			}
			if allFailed {
				consecAllFail++
				if consecAllFail >= maxConsecutiveAllFailTurns {
					stopReason = chatdomain.StopReasonError
					errCode = "TOOL_ERROR_STORM"
					errMsg = fmt.Sprintf("%d consecutive turns where every tool call failed; aborting to prevent runaway", consecAllFail)
					finalStatus = chatdomain.StatusError
					host.WriteFinalize(ctx, allBlocks, chatdomain.StatusError, stopReason, errCode, errMsg, totalIn, totalOut)
					finalWritten = true
					break
				}
			} else {
				consecAllFail = 0
			}
		}

		history, err = extendHistory(log, history, aBlocks, rBlocks)
		if err != nil {
			log.Error("extend history failed", zap.Error(err))
			stopReason = chatdomain.StopReasonError
			errCode = "HISTORY_EXTEND_FAILED"
			errMsg = err.Error()
			finalStatus = chatdomain.StatusError
			host.WriteFinalize(ctx, allBlocks, chatdomain.StatusError, stopReason, errCode, errMsg, totalIn, totalOut)
			finalWritten = true
			break
		}

		log.Debug("react step complete", zap.Int("step", step))
	}

	if !finalWritten {
		// maxSteps exhausted while the model still wanted to act. Surface this
		// honestly — a non-success terminal + a distinct stop_reason — instead of
		// masquerading as a completed turn. The work isn't lost; the UI offers
		// "continue" off the MAX_STEPS_REACHED errCode to resume the same conversation.
		//
		// maxSteps 耗尽但模型还想动作。诚实暴露——非成功终态 + 独立 stop_reason，
		// 不再冒充 completed。工作没丢；UI 凭 MAX_STEPS_REACHED 提供「继续」续跑同会话。
		stopReason = chatdomain.StopReasonMaxSteps
		finalStatus = chatdomain.StatusError
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

func toolsByName(tools []toolapp.Tool) map[string]toolapp.Tool {
	m := make(map[string]toolapp.Tool, len(tools))
	for _, t := range tools {
		m[t.Name()] = t
	}
	return m
}
