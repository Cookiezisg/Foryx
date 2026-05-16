package subagent

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// defaultRunTimeout caps a single Spawn — the sole preemption mechanism for sub-runs.
//
// defaultRunTimeout 限定单次 Spawn，是 sub-run 唯一的抢占机制。
const defaultRunTimeout = 5 * time.Minute

const (
	StatusCompleted = "completed"
	StatusMaxTurns  = "max_turns"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

// SpawnOpts overrides per-call; empty fields fall back to the type's defaults.
//
// SpawnOpts per-call 覆盖；空字段回落到类型默认。
type SpawnOpts struct {
	MaxTurns int
}

// SpawnResult is what Service.Spawn hands back; RunID is the sub-Message ID.
//
// SpawnResult 是 Service.Spawn 的回执；RunID 即 sub-Message ID。
type SpawnResult struct {
	RunID     string
	Type      string
	Status    string
	ErrorMsg  string
	Result    string
	TokensIn  int
	TokensOut int
	StepsUsed int
}

// Spawn boots one sub-run end-to-end; on any error returns a Status=StatusFailed SpawnResult plus the error.
//
// Spawn 一站式启动 sub-run；任何错误都返回 Status=StatusFailed 的 SpawnResult 并带 error。
func (s *Service) Spawn(parentCtx context.Context, typeName, prompt string, opts SpawnOpts) (*SpawnResult, error) {
	typ, ok := s.registry.Get(typeName)
	if !ok {
		return nil, fmt.Errorf("subagentapp.Spawn: %w: %q", subagentdomain.ErrTypeNotFound, typeName)
	}

	parentMsgID, _ := reqctxpkg.GetMessageID(parentCtx)
	parentToolCallID, _ := reqctxpkg.GetToolCallID(parentCtx)
	parentConvID, _ := reqctxpkg.GetConversationID(parentCtx)
	uid, _ := reqctxpkg.GetUserID(parentCtx)

	bundle, err := llmclientpkg.Resolve(parentCtx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		return nil, fmt.Errorf("subagentapp.Spawn resolve LLM: %w", err)
	}

	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = typ.DefaultMaxTurns
	}

	em := eventlogpkg.From(parentCtx)
	subMsgID := idgenpkg.New("msg")
	msgBlockID := ""
	if parentToolCallID != "" && parentMsgID != "" {
		msgBlockID = idgenpkg.New("blk")
		em.EmitBlockStart(parentCtx, msgBlockID, parentToolCallID, parentMsgID,
			eventlogdomain.BlockTypeMessage,
			map[string]any{
				"messageId": subMsgID,
				"type":      typ.Name,
			})
		em.EmitMessageStart(parentCtx, subMsgID, chatdomain.RoleAssistant, msgBlockID,
			map[string]any{
				"kind":     "subagent_run",
				"type":     typ.Name,
				"maxTurns": maxTurns,
			})
	}

	subCtx := reqctxpkg.WithSubagentDepth(parentCtx, reqctxpkg.GetSubagentDepth(parentCtx)+1)
	subCtx = reqctxpkg.WithMessageID(subCtx, subMsgID)
	subCtx = reqctxpkg.WithParentBlockID(subCtx, "")
	subCtx = eventlogpkg.With(subCtx, em)
	subCtx, cancel := context.WithTimeout(subCtx, defaultRunTimeout)
	defer cancel()

	host := &subagentHost{
		svc:           s,
		subMsgID:      subMsgID,
		parentConvID:  parentConvID,
		parentBlockID: msgBlockID,
		uid:           uid,
		typeName:      typ.Name,
		maxTurns:      maxTurns,
		tools:         s.filterTools(typ),
		userPrompt:    prompt,
		systemPrompt:  composeSystemPrompt(typ.SystemPrompt, reqctxpkg.GetLocale(parentCtx)),
	}

	baseReq := llminfra.Request{
		ModelID: bundle.ModelID,
		Key:     bundle.Key,
		BaseURL: bundle.BaseURL,
		System:  host.systemPrompt,
	}

	var (
		result loopapp.Result
		runErr error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("subagent panic: %v", r)
				s.log.Error("subagent run panicked",
					zap.String("sub_msg_id", subMsgID), zap.Any("panic", r))
			}
		}()
		result = loopapp.Run(subCtx, host, bundle.Client, baseReq, maxTurns, s.log)
	}()

	spawn := &SpawnResult{
		RunID:     subMsgID,
		Type:      typ.Name,
		Result:    result.LastMessage,
		TokensIn:  result.TokensIn,
		TokensOut: result.TokensOut,
		StepsUsed: result.Steps,
	}
	switch {
	case runErr != nil:
		spawn.Status = StatusFailed
		spawn.ErrorMsg = runErr.Error()
	case result.StopReason == chatdomain.StopReasonCancelled:
		spawn.Status = StatusCancelled
	case result.StopReason == chatdomain.StopReasonMaxTokens:
		spawn.Status = StatusMaxTurns
	case result.Status == chatdomain.StatusError:
		spawn.Status = StatusFailed
		spawn.ErrorMsg = result.StopReason
	default:
		spawn.Status = StatusCompleted
	}

	// Detached reconcile so parent cancel doesn't drop the status rewrite.
	// detached 写入避免 parent cancel 丢失 status 重对齐。
	if spawn.Status != StatusCompleted {
		reconcileCtx := reqctxpkg.SetUserID(context.Background(), uid)
		reconcileCtx = reqctxpkg.WithConversationID(reconcileCtx, parentConvID)
		if existing, err := s.chatRepo.GetMessage(reconcileCtx, subMsgID); err == nil && existing != nil {
			existing.Status = spawn.Status
			if spawn.ErrorMsg != "" {
				existing.ErrorMessage = spawn.ErrorMsg
			}
			if err := s.chatRepo.SaveMessage(reconcileCtx, existing); err != nil {
				s.log.Warn("subagent status reconcile write failed",
					zap.String("sub_msg_id", subMsgID),
					zap.String("status", spawn.Status),
					zap.Error(err))
			}
		}
	}

	// Detached StopBlock so parent cancel can't leave a dangling block_start (§S21).
	// detached StopBlock 防止 parent cancel 留下 dangling block_start (§S21)。
	if msgBlockID != "" {
		closeStatus := eventlogdomain.StatusCompleted
		switch spawn.Status {
		case StatusFailed:
			closeStatus = eventlogdomain.StatusError
		case StatusCancelled:
			closeStatus = eventlogdomain.StatusCancelled
		}
		stopCtx := reqctxpkg.SetUserID(context.Background(), uid)
		stopCtx = reqctxpkg.WithConversationID(stopCtx, parentConvID)
		em.StopBlock(stopCtx, msgBlockID, closeStatus, nil)
	}

	s.log.Info("subagent run terminated",
		zap.String("sub_msg_id", subMsgID),
		zap.String("type", typ.Name),
		zap.String("status", spawn.Status),
		zap.Int("tokens_in", spawn.TokensIn),
		zap.Int("tokens_out", spawn.TokensOut),
		zap.Int("steps", spawn.StepsUsed))

	return spawn, runErr
}
