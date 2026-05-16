package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

type SearchSkillExecutions struct {
	repo skilldomain.ExecutionRepository
}

func (t *SearchSkillExecutions) Name() string { return "search_skill_executions" }

func (t *SearchSkillExecutions) Description() string {
	return "Search Skill activation history (skill_executions table). Filter by skillName / " +
		"status / conversationId / flowrunId / forkDepth. Returns previews + aggregates."
}

func (t *SearchSkillExecutions) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"skillName":      {"type": "string"},
			"status":         {"type": "string", "enum": ["ok","failed","cancelled","timeout"]},
			"conversationId": {"type": "string"},
			"flowrunId":      {"type": "string"},
			"forkDepth":      {"type": "integer"},
			"limit":          {"type": "integer"},
			"cursor":         {"type": "string"}
		}
	}`)
}

func (t *SearchSkillExecutions) IsReadOnly() bool        { return true }
func (t *SearchSkillExecutions) NeedsReadFirst() bool    { return false }
func (t *SearchSkillExecutions) RequiresWorkspace() bool { return false }
func (t *SearchSkillExecutions) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchSkillExecutions) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchSkillExecutions) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		SkillName, Status, ConversationID, FlowrunID, Cursor string
		ForkDepth                                            *int
		Limit                                                int
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_skill_executions: bad args: %w", err)
	}
	filter := skilldomain.ExecutionFilter{
		SkillName: args.SkillName, Status: args.Status,
		ConversationID: args.ConversationID, FlowrunID: args.FlowrunID,
		ForkDepth: args.ForkDepth, Limit: args.Limit, Cursor: args.Cursor,
	}
	rows, next, err := t.repo.ListExecutions(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("search_skill_executions: %w", err)
	}
	agg, _ := t.repo.ComputeAggregates(ctx, filter)

	type preview struct {
		ID, Status, SkillName, StartedAt, ErrorMessage string
		ElapsedMs                                      int64
		ForkDepth                                      int
		OutputPreview                                  string
	}
	previews := make([]preview, 0, len(rows))
	for _, r := range rows {
		previews = append(previews, preview{
			ID: r.ID, Status: r.Status, SkillName: r.SkillName,
			StartedAt: r.StartedAt.Format(time.RFC3339), ElapsedMs: r.ElapsedMs,
			ForkDepth:     r.ForkDepth,
			ErrorMessage:  r.ErrorMessage,
			OutputPreview: truncateJSON(r.Output, 200),
		})
	}
	resp := map[string]any{
		"count":      len(previews),
		"executions": previews,
		"nextCursor": next,
		"hasMore":    next != "",
		"aggregates": agg,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

func truncateJSON(v any, max int) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
