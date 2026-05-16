package skill

import (
	"context"
	"encoding/json"
	"fmt"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

type GetSkillExecution struct {
	repo skilldomain.ExecutionRepository
}

func (t *GetSkillExecution) Name() string { return "get_skill_execution" }

func (t *GetSkillExecution) Description() string {
	return "Fetch one Skill activation by id (ske_xxx). Returns full output text, " +
		"substitutions used, fork depth, timing."
}

func (t *GetSkillExecution) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"id": {"type": "string"}},
		"required": ["id"]
	}`)
}

func (t *GetSkillExecution) IsReadOnly() bool        { return true }
func (t *GetSkillExecution) NeedsReadFirst() bool    { return false }
func (t *GetSkillExecution) RequiresWorkspace() bool { return false }
func (t *GetSkillExecution) ValidateInput(json.RawMessage) error { return nil }
func (t *GetSkillExecution) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetSkillExecution) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct{ ID string `json:"id"` }
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_skill_execution: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_skill_execution: id required")
	}
	row, err := t.repo.GetExecutionByID(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_skill_execution: %w", err)
	}
	_ = skilldomain.ExecutionStatusOK
	b, _ := json.Marshal(row)
	return string(b), nil
}
