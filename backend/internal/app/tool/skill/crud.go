package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	skillapp "github.com/sunweilin/anselm/backend/internal/app/skill"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	skilldomain "github.com/sunweilin/anselm/backend/internal/domain/skill"
)

// saveSkillArgs is the shared create/edit payload (both write a full SKILL.md).
//
// saveSkillArgs 是 create/edit 共享的载荷（两者都全量写 SKILL.md）。
type saveSkillArgs struct {
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Body                   string   `json:"body"`
	AllowedTools           []string `json:"allowedTools"`
	Context                string   `json:"context"`
	Agent                  string   `json:"agent"`
	Arguments              []string `json:"arguments"`
	DisableModelInvocation bool     `json:"disableModelInvocation"`
}

// toInput maps tool args to the app SaveInput; source marks the AI as author.
//
// toInput 把工具 args 映射成 app SaveInput；source 标记 AI 为作者。
func (a saveSkillArgs) toInput() skillapp.SaveInput {
	return skillapp.SaveInput{
		Name:                   a.Name,
		Description:            a.Description,
		Body:                   a.Body,
		AllowedTools:           a.AllowedTools,
		Context:                a.Context,
		Agent:                  a.Agent,
		Arguments:              a.Arguments,
		DisableModelInvocation: a.DisableModelInvocation,
		Source:                 skilldomain.SourceAI,
	}
}

const saveSkillSchema = `{
	"type": "object",
	"required": ["name", "description", "body"],
	"properties": {
		"name": {"type": "string", "description": "Lowercase slug, e.g. code-review."},
		"description": {"type": "string", "description": "What the skill does AND when to use it (this is how it gets discovered)."},
		"body": {"type": "string", "description": "Markdown instructions; may use $ARGUMENTS / $1 / ${CLAUDE_SESSION_ID} placeholders."},
		"allowedTools": {"type": "array", "items": {"type": "string"}, "description": "Tools pre-approved (skip per-call confirmation) while this skill is active."},
		"context": {"type": "string", "enum": ["inline", "fork"], "description": "inline injects into the current dialogue (default); fork runs in an isolated subagent."},
		"agent": {"type": "string", "description": "Subagent type — required when context=fork."},
		"arguments": {"type": "array", "items": {"type": "string"}, "description": "Named argument labels for $name substitution."},
		"disableModelInvocation": {"type": "boolean", "description": "If true, the skill is hidden from the model's catalog (user-only trigger)."}
	}
}`

func validateSave(tool string, args json.RawMessage) error {
	var a struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("%s: bad args: %w", tool, err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("%s: name is required", tool)
	}
	if strings.TrimSpace(a.Description) == "" {
		return fmt.Errorf("%s: description is required", tool)
	}
	if strings.TrimSpace(a.Body) == "" {
		return fmt.Errorf("%s: body is required", tool)
	}
	return nil
}

// CreateSkill authors a brand-new skill (name conflict → error).
//
// CreateSkill 创作一个全新 skill（同名冲突 → 报错）。
type CreateSkill struct{ svc *skillapp.Service }

func (t *CreateSkill) Name() string { return "create_skill" }

func (t *CreateSkill) Description() string {
	return "Author a NEW skill — a reusable instruction pack you can later activate. Use this to codify a workflow you just performed into a repeatable capability. Fails if the name already exists (use edit_skill to change one)."
}

func (t *CreateSkill) Parameters() json.RawMessage { return json.RawMessage(saveSkillSchema) }

func (t *CreateSkill) ValidateInput(args json.RawMessage) error {
	return validateSave("create_skill", args)
}

func (t *CreateSkill) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args saveSkillArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_skill: bad args: %w", err)
	}
	sk, err := t.svc.Create(ctx, args.toInput())
	if err != nil {
		return "", fmt.Errorf("create_skill: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"created": sk.Name}), nil
}

var _ toolapp.Tool = (*CreateSkill)(nil)

// EditSkill overwrites an existing skill (read it first with get_skill to preserve content).
//
// EditSkill 覆盖一个已存在的 skill（先 get_skill 读取以保留内容）。
type EditSkill struct{ svc *skillapp.Service }

func (t *EditSkill) Name() string { return "edit_skill" }

func (t *EditSkill) Description() string {
	return "Overwrite an existing skill's SKILL.md (full replacement). Call get_skill first to retrieve the current content, modify it, then pass the complete new version here. Fails if the skill doesn't exist (use create_skill)."
}

func (t *EditSkill) Parameters() json.RawMessage { return json.RawMessage(saveSkillSchema) }

func (t *EditSkill) ValidateInput(args json.RawMessage) error {
	return validateSave("edit_skill", args)
}

func (t *EditSkill) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args saveSkillArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_skill: bad args: %w", err)
	}
	sk, err := t.svc.Replace(ctx, args.toInput())
	if err != nil {
		return "", fmt.Errorf("edit_skill: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"updated": sk.Name}), nil
}

var _ toolapp.Tool = (*EditSkill)(nil)

// DeleteSkill removes a skill directory.
//
// DeleteSkill 删除一个 skill 目录。
type DeleteSkill struct {
	svc  *skillapp.Service
	deps toolapp.DependentCounter
}

func (t *DeleteSkill) Name() string { return "delete_skill" }

func (t *DeleteSkill) Description() string {
	return "Delete a skill permanently (removes its directory). Cannot be undone. The result reports how many agents equipped it (and may now fail) — to check dependents BEFORE deleting, use get_relations."
}

func (t *DeleteSkill) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name"],
		"properties": {"name": {"type": "string"}}
	}`)
}

func (t *DeleteSkill) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_skill: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrNameRequired
	}
	return nil
}

func (t *DeleteSkill) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_skill: bad args: %w", err)
	}
	// Skill is name-as-id, so its relation id is the skill name (agents equip skills by name).
	// skill 是名即 id，故其 relation id 就是 skill 名（agent 按名 equip skill）。
	deps := toolapp.DependentCount(ctx, t.deps, relationdomain.EntityKindSkill, args.Name)
	if err := t.svc.Delete(ctx, args.Name); err != nil {
		return "", fmt.Errorf("delete_skill: %w", err)
	}
	return toolapp.ToJSON(toolapp.AnnotateDependents(map[string]any{"deleted": args.Name}, deps)), nil
}

var _ toolapp.Tool = (*DeleteSkill)(nil)
