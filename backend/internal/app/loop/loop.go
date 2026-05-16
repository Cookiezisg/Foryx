// Package loop is the shared ReAct engine (stream → tool dispatch → history → finalize).
//
// Package loop 是共享的 ReAct 引擎（流 → 工具调度 → 历史 → 终态）。
package loop

import (
	"context"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// Host is the per-run hook surface; block persistence happens via the eventlog Emitter, not Host.
//
// Host 是每次 run 的钩子面；block 持久化走 eventlog Emitter，不经 Host。
type Host interface {
	LoadHistory(ctx context.Context) ([]llminfra.LLMMessage, error)
	Tools() []toolapp.Tool
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

// Run executes the ReAct loop, composing baseReq.Messages from host.LoadHistory and Tools from host.Tools.
//
// Run 执行 ReAct 循环；baseReq.Messages 取自 host.LoadHistory，Tools 取自 host.Tools。
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

	tools := host.Tools()
	baseReq.Tools = toolapp.ToLLMDefs(tools)
	byName := toolsByName(tools)

	var (
		allBlocks    []chatdomain.Block
		totalIn      int
		totalOut     int
		stopReason   = chatdomain.StopReasonEndTurn
		finalStatus  = chatdomain.StatusCompleted
		errCode      string
		errMsg       string
		finalWritten bool
		stepsRun     int
	)

	for step := range maxSteps {
		req := baseReq
		req.Messages = history

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
		stopReason = chatdomain.StopReasonMaxTokens
		host.WriteFinalize(ctx, allBlocks, chatdomain.StatusCompleted, stopReason, "", "", totalIn, totalOut)
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
