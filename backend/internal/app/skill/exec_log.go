// exec_log.go — D22 skill_executions row writer. Called from Activate's
// defer wrapper. Best-effort (failure logs but doesn't propagate);
// detached ctx + user stamp so caller-cancel doesn't lose the row (§S9).
//
// exec_log.go —— D22 skill_executions 写入。Activate defer wrapper 调;
// best-effort + detached ctx + user stamp(§S9)防 caller-cancel 丢日志。

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

// recordExecution writes one terminal skill_executions row. Pulls
// linkage IDs (conv/msg/toolCall) from ctx;classifies err type into
// status (ok/cancelled/timeout/failed).
//
// recordExecution 写一行 skill_executions 终态;按 ctx 抽 linkage ID +
// 按 err 类型分 status。
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
		// V1: skilldomain.Skill exposes a content hash via the scanner;
		// if SkillVersion field isn't surfaced we fall back to empty.
		// V1:skilldomain.Skill 暂未暴露 content hash 字段,留空。
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

// extractSkillVersion best-effort reads a content hash from the Skill
// struct. V1: returns empty until skilldomain.Skill exposes a Hash field.
//
// extractSkillVersion best-effort 读 content hash;V1 留空。
func extractSkillVersion(*skilldomain.Skill) string {
	return ""
}

// itoa is the tiny strconv.Itoa shim — kept local so we don't pull in
// strconv just for this one call.
//
// itoa 是 strconv.Itoa 的局部 shim;省 import。
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
