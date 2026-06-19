package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentapp "github.com/sunweilin/anselm/backend/internal/app/agent"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
	modeldomain "github.com/sunweilin/anselm/backend/internal/domain/model"
	schemapkg "github.com/sunweilin/anselm/backend/internal/pkg/schema"
)

// configArgs is the shared create/edit config payload (a full snapshot — edit REPLACES).
//
// configArgs 是 create/edit 共享的配置载荷（全量快照——edit 替换）。
type configArgs struct {
	Prompt        string                `json:"prompt"`
	Skill         string                `json:"skill"`
	Knowledge     []string              `json:"knowledge"`
	Tools         []agentdomain.ToolRef `json:"tools"`
	Inputs        []schemapkg.Field     `json:"inputs"`
	Outputs       []schemapkg.Field     `json:"outputs"`
	ModelOverride *modeldomain.ModelRef `json:"modelOverride"`
	ChangeReason  string                `json:"changeReason"`
}

func (c configArgs) toConfig() agentapp.Config {
	return agentapp.Config{
		Prompt: c.Prompt, Skill: c.Skill, Knowledge: c.Knowledge, Tools: c.Tools,
		Inputs: c.Inputs, Outputs: c.Outputs, ModelOverride: c.ModelOverride, ChangeReason: c.ChangeReason,
	}
}

// configProps is the shared JSON-schema fragment for the mounted config fields.
const configProps = `
		"prompt": {"type": "string", "description": "System prompt defining the agent's role and behaviour."},
		"skill": {"type": "string", "description": "Optional skill name to mount: its instructions (the skill Guide) are injected into the prompt. Only the guidance is mounted — a skill's allowed-tools pre-authorization does NOT carry to an agent (its runs may be unattended, so dangerous tools still require confirmation)."},
		"knowledge": {"type": "array", "items": {"type": "string"}, "description": "Document IDs attached as background knowledge."},
		"tools": {"type": "array", "description": "Callables the agent may use: each {ref, name}. ref is fn_… / hd_…method / mcp:server/tool — NEVER ag_ (an agent cannot call another agent; to chain agents, build a workflow with an agent node for each instead).", "items": {"type": "object", "required": ["ref"], "properties": {"ref": {"type": "string"}, "name": {"type": "string"}}}},
		"inputs": {"type": "array", "description": "Declared task inputs the workflow feeds: each {name, type, description}. type ∈ string|number|boolean|object|array.", "items": {"type": "object"}},
		"outputs": {"type": "array", "description": "Declared result fields downstream reads: each {name, type, description}. Empty = free-form text answer; otherwise the final answer is a JSON object with these fields.", "items": {"type": "object"}},
		"modelOverride": {"type": "object", "description": "Optional {apiKeyId, modelId} to override the default agent model.", "properties": {"apiKeyId": {"type": "string"}, "modelId": {"type": "string"}}},
		"changeReason": {"type": "string", "description": "One-line reason for this change."}`

// --- create_agent ----------------------------------------------------------

type CreateAgent struct{ svc *agentapp.Service }

func (t *CreateAgent) Name() string { return "create_agent" }

func (t *CreateAgent) Description() string {
	return "Build a new agent — a configured LLM worker that runs a ReAct loop. It writes no code; it mounts capabilities by reference: a prompt, an optional skill, knowledge documents, and tools (fn_/hd_/mcp refs). v1 takes effect immediately (no separate accept). Build an agent (not a function) when the task needs LLM reasoning/judgement across multiple tool calls; build a function for deterministic code."
}

func (t *CreateAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name", "prompt"],
		"properties": {
			"name": {"type": "string", "description": "Unique agent name."},
			"description": {"type": "string", "description": "One-line role summary."},
			"tags": {"type": "array", "items": {"type": "string"}},` + configProps + `
		}
	}`)
}

func (t *CreateAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_agent: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Prompt) == "" {
		return ErrNamePromptRequired
	}
	return nil
}

func (t *CreateAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		configArgs
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("create_agent: bad args: %w", err)
	}
	ag, v, err := t.svc.Create(ctx, agentapp.CreateInput{
		Name: a.Name, Description: a.Description, Tags: a.Tags, Config: a.configArgs.toConfig(),
	})
	if err != nil {
		return "", fmt.Errorf("create_agent: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": ag.ID, "versionId": v.ID, "version": v.Version}), nil
}

// --- edit_agent ------------------------------------------------------------

type EditAgent struct{ svc *agentapp.Service }

func (t *EditAgent) Name() string { return "edit_agent" }

func (t *EditAgent) Description() string {
	return "Edit an agent: REPLACE its full configuration, producing a new version that takes effect immediately. Read the agent first (get_agent) — this is a whole-config replace, not a merge. Use revert_agent to switch back to an older version."
}

func (t *EditAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["agentId", "prompt"],
		"properties": {
			"agentId": {"type": "string"},` + configProps + `
		}
	}`)
}

func (t *EditAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		AgentID string `json:"agentId"`
		Prompt  string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_agent: bad args: %w", err)
	}
	if strings.TrimSpace(a.AgentID) == "" || strings.TrimSpace(a.Prompt) == "" {
		return ErrIDPromptRequired
	}
	return nil
}

func (t *EditAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		AgentID string `json:"agentId"`
		configArgs
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("edit_agent: bad args: %w", err)
	}
	v, err := t.svc.Edit(ctx, agentapp.EditInput{ID: a.AgentID, Config: a.configArgs.toConfig()})
	if err != nil {
		return "", fmt.Errorf("edit_agent: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"agentId": a.AgentID, "versionId": v.ID, "version": v.Version}), nil
}
