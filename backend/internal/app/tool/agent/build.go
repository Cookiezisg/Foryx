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

// configArgs is the shared create/edit config payload. create takes a full snapshot; edit MERGES —
// only the fields actually present in the request overlay the agent's current config (see mergeConfig).
//
// configArgs 是 create/edit 共享的配置载荷。create 取全量快照；edit 合并——只有请求中实际出现的字段覆盖
// agent 当前配置（见 mergeConfig）。
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
		"skill": {"type": "string", "description": "Optional skill name to mount — it MUST already exist (create_skill first; a non-existent name is rejected at build time). Its instructions (the skill Guide) are injected into the prompt. Only the guidance is mounted — a skill's allowed-tools pre-authorization does NOT carry to an agent (its runs may be unattended, so dangerous tools still require confirmation)."},
		"knowledge": {"type": "array", "items": {"type": "string"}, "description": "Document IDs attached as background knowledge — each MUST already exist (a non-existent doc id is rejected at build time)."},
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
	return "Edit an agent: change ONLY the fields you pass — every omitted field keeps its current value (a partial edit no longer wipes the agent's mounted tools/knowledge). To clear a field, pass it explicitly empty ([] / \"\"). Each edit produces a new version that takes effect immediately; use revert_agent to switch back to an older one."
}

func (t *EditAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["agentId"],
		"properties": {
			"agentId": {"type": "string"},` + configProps + `
		}
	}`)
}

func (t *EditAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_agent: bad args: %w", err)
	}
	// A partial edit overlays only provided fields, so prompt is no longer required — agentId alone is.
	//
	// 部分编辑只覆盖所提供字段，故 prompt 不再必填——仅 agentId 必填。
	if strings.TrimSpace(a.AgentID) == "" {
		return ErrAgentIDRequired
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
	// MERGE, not full-replace: start from the agent's current active config and overlay only the fields
	// the agent actually provided. edit_agent used to REPLACE the whole config, so a prompt-only edit
	// silently wiped the agent's mounted tools/knowledge — a MEASURED ~40% config-drop rate that the
	// "whole-config replace, read first" description failed to prevent (F-edit-agent-merge).
	// 合并、非全替换：从 agent 当前 active 配置起、只覆盖 agent 真正提供的字段。edit_agent 原先全替换，故只改
	// prompt 的编辑会静默抹掉挂载的 tools/knowledge——实测约 40% 丢配率，"整配替换、先读" 的描述拦不住。
	cur, err := t.svc.Get(ctx, a.AgentID)
	if err != nil {
		return "", fmt.Errorf("edit_agent: %w", err)
	}
	merged := mergeConfig(configFromActive(cur), []byte(argsJSON))
	v, err := t.svc.Edit(ctx, agentapp.EditInput{ID: a.AgentID, Config: merged})
	if err != nil {
		return "", fmt.Errorf("edit_agent: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"agentId": a.AgentID, "versionId": v.ID, "version": v.Version}), nil
}

// configFromActive maps an agent's current active version back to a Config — the merge base for a
// partial edit_agent. A nil active version yields a zero config.
//
// configFromActive 把 agent 当前 active 版本映回 Config——partial edit_agent 的合并基底。无 active 版本→零配置。
func configFromActive(a *agentdomain.Agent) agentapp.Config {
	v := a.ActiveVersion
	if v == nil {
		return agentapp.Config{}
	}
	return agentapp.Config{
		Prompt: v.Prompt, Skill: v.Skill, Knowledge: v.Knowledge, Tools: v.Tools,
		Inputs: v.Inputs, Outputs: v.Outputs, ModelOverride: v.ModelOverride,
	}
}

// mergeConfig overlays onto current ONLY the config fields actually present in argsJSON (an absent
// field keeps its current value; a provided field — even empty/null — is set). Pure, so the
// preserve-omitted / clear-on-explicit-empty semantics are unit-testable without a service.
//
// mergeConfig 把 argsJSON 中**实际出现**的配置字段覆盖到 current（缺省字段保留当前值；提供的字段——哪怕
// 空/null——被设置）。纯函数，使「省略则保留 / 显式空则清」语义无需服务即可单测。
func mergeConfig(current agentapp.Config, argsJSON []byte) agentapp.Config {
	var a configArgs
	_ = json.Unmarshal(argsJSON, &a)
	var present map[string]json.RawMessage
	_ = json.Unmarshal(argsJSON, &present)
	merged := current
	if _, ok := present["prompt"]; ok {
		merged.Prompt = a.Prompt
	}
	if _, ok := present["skill"]; ok {
		merged.Skill = a.Skill
	}
	if _, ok := present["knowledge"]; ok {
		merged.Knowledge = a.Knowledge
	}
	if _, ok := present["tools"]; ok {
		merged.Tools = a.Tools
	}
	if _, ok := present["inputs"]; ok {
		merged.Inputs = a.Inputs
	}
	if _, ok := present["outputs"]; ok {
		merged.Outputs = a.Outputs
	}
	if _, ok := present["modelOverride"]; ok {
		merged.ModelOverride = a.ModelOverride
	}
	if _, ok := present["changeReason"]; ok {
		merged.ChangeReason = a.ChangeReason
	}
	return merged
}
