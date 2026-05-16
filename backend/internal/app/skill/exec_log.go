package skill

import (
	"context"
	stderrors "errors"
	"time"

	"go.uber.org/zap"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// recordExecution writes one terminal skill_executions row via detached ctx.
//
// recordExecution 用 detached ctx 写入 skill_executions 终态行。
func (s *Service) recordExecution(ctx context.Context, name string, arguments []string, result string, callErr error, startedAt, endedAt time.Time) {
	s.mu.RLock()
	repo := s.execRepo
	skillRow := s.skills[name]
	s.mu.RUnlock()
	if repo == nil {
		return
	}

	uid, _ := reqctxpkg.RequireUserID(ctx)
	if uid == "" {
		uid = reqctxpkg.DefaultLocalUserID
	}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	depth := reqctxpkg.GetSubagentDepth(ctx)

	status := skilldomain.ExecutionStatusOK
	errCode, errMsg := "", ""
	if callErr != nil {
		switch {
		case stderrors.Is(callErr, context.Canceled):
			status = skilldomain.ExecutionStatusCancelled
			errCode = "CTX_CANCELLED"
		case stderrors.Is(callErr, context.DeadlineExceeded):
			status = skilldomain.ExecutionStatusTimeout
			errCode = "DEADLINE_EXCEEDED"
		default:
			status = skilldomain.ExecutionStatusFailed
			errCode = "SKILL_ACTIVATE_FAILED"
		}
		errMsg = callErr.Error()
	}

	triggeredBy := skilldomain.TriggeredByChat
	if toolCallID == "" && convID == "" {
		triggeredBy = skilldomain.TriggeredByHTTP
	}

	skillVersion := ""
	if skillRow != nil {
		skillVersion = extractSkillVersion(skillRow)
	}

	subs := map[string]any{}
	for i, a := range arguments {
		subs["$"+itoa(i+1)] = a
	}

	row := &skilldomain.Execution{
		ID:             idgenpkg.New("ske"),
		UserID:         uid,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          map[string]any{"arguments": arguments},
		Output:         result,
		ErrorCode:      errCode,
		ErrorMessage:   errMsg,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		SkillName:      name,
		SkillVersion:   skillVersion,
		ForkDepth:      depth,
		Substitutions:  subs,
	}

	detached := reqctxpkg.SetUserID(context.Background(), uid)
	if err := repo.SaveExecution(detached, row); err != nil {
		s.log.Warn("recordExecution: save failed",
			zap.String("skill", name), zap.Error(err))
	}
}

func extractSkillVersion(*skilldomain.Skill) string {
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
