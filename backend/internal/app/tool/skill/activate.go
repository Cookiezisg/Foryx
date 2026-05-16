package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// ErrEmptyName: name missing or whitespace.
//
// ErrEmptyName：name 缺失或全空白。
var ErrEmptyName = errors.New("name is required and must be non-empty")

const activateSkillDescription = `Load a skill's full instructions. The result is the substituted body text (or, when the skill declares context: fork, the final output of an isolated subagent that ran the body). Activation also pre-approves the skill's allowed-tools for the rest of this conversation.`

var activateSkillSchema = json.RawMessage(`{
	"type": "object",
	"required": ["name"],
	"properties": {
		"name": {
			"type": "string",
			"description": "Skill name (from search_skills result, or known by convention like 'pr-review')."
		},
		"arguments": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Positional arguments substituted into $1, $2, ..., $ARGUMENTS, and named placeholders matching the skill's frontmatter.arguments declaration."
		}
	}
}`)

// ActivateSkill implements the activate_skill system tool.
//
// ActivateSkill 是 activate_skill 系统工具的实现。
type ActivateSkill struct {
	svc *skillapp.Service
}

func (t *ActivateSkill) Name() string                { return "activate_skill" }
func (t *ActivateSkill) Description() string         { return activateSkillDescription }
func (t *ActivateSkill) Parameters() json.RawMessage { return activateSkillSchema }

// IsReadOnly = false because activate writes to AgentState (ActiveSkill side-channel).
//
// IsReadOnly = false 因为 activate 改 AgentState（ActiveSkill 旁路）。
func (t *ActivateSkill) IsReadOnly() bool        { return false }
func (t *ActivateSkill) NeedsReadFirst() bool    { return false }
func (t *ActivateSkill) RequiresWorkspace() bool { return false }


func (t *ActivateSkill) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("activate_skill.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrEmptyName
	}
	return nil
}

func (t *ActivateSkill) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// Execute calls Service.Activate; returns body or maps known errors to friendly LLM-facing strings.
//
// Execute 调 Service.Activate；返 body 或将已知错误映射为友好字符串。
func (t *ActivateSkill) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name      string   `json:"name"`
		Arguments []string `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("activate_skill.Execute: parse args: %w", err)
	}

	out, err := t.svc.Activate(ctx, args.Name, args.Arguments)
	if err == nil {
		return out, nil
	}

	switch {
	case errors.Is(err, skilldomain.ErrSkillNotFound):
		return fmt.Sprintf("Skill %q not found. Call search_skills first to see what's available.", args.Name), nil
	case errors.Is(err, skilldomain.ErrBodyTooLarge):
		return fmt.Sprintf("Skill %q body exceeds the %d-byte limit. Ask the user to split long instructions into separate resource files.", args.Name, skilldomain.MaxBodyBytes), nil
	default:
		return "", fmt.Errorf("activate_skill: %w", err)
	}
}


var _ toolapp.Tool = (*ActivateSkill)(nil)
