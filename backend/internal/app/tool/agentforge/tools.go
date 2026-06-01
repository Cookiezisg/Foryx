// Package agentforge provides LLM-callable tools for the Agent entity (quadrinity 4th member).
// These are the 6 core tools from the design (create_agent / edit_agent / get_agent /
// search_agents / delete_agent / accept_pending_agent).
//
// Package agentforge 提供 Agent 实体的 LLM 工具（quadrinity 第四元，核心 6 个）。
package agentforge

import (
	"context"
	"encoding/json"
	"fmt"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
)

// AgentTools returns all core agent forging tools.
func AgentTools(svc *agentapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchAgents{svc: svc},
		&GetAgent{svc: svc},
		&CreateAgent{svc: svc},
		&EditAgent{svc: svc},
		&AcceptPendingAgent{svc: svc},
		&DeleteAgent{svc: svc},
	}
}

// ── SearchAgents ─────────────────────────────────────────────────────────────

type SearchAgents struct{ svc *agentapp.Service }

func (t *SearchAgents) Name() string { return "search_agents" }
func (t *SearchAgents) Description() string {
	return "Find agents in the user's library by name/description substring (empty=list all). Returns id, name, description, activeVersionId. Inspect with get_agent before editing."
}
func (t *SearchAgents) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}`)
}
func (t *SearchAgents) IsReadOnly() bool        { return true }
func (t *SearchAgents) NeedsReadFirst() bool    { return false }
func (t *SearchAgents) RequiresWorkspace() bool { return false }
func (t *SearchAgents) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchAgents) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (t *SearchAgents) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &args)
	limit := args.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	agents, _, err := t.svc.List(ctx, limit, "")
	if err != nil {
		return "", fmt.Errorf("search_agents: %w", err)
	}
	type row struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		ActiveVersionID string `json:"activeVersionId,omitempty"`
	}
	q := args.Query
	out := make([]row, 0, len(agents))
	for _, a := range agents {
		if q != "" {
			if !containsSub(a.Name, q) && !containsSub(a.Description, q) {
				continue
			}
		}
		out = append(out, row{ID: a.ID, Name: a.Name, Description: a.Description, ActiveVersionID: a.ActiveVersionID})
	}
	b, _ := json.Marshal(map[string]any{"count": len(out), "agents": out})
	return string(b), nil
}

// ── GetAgent ─────────────────────────────────────────────────────────────────

type GetAgent struct{ svc *agentapp.Service }

func (t *GetAgent) Name() string { return "get_agent" }
func (t *GetAgent) Description() string {
	return "Get full agent details: prompt, skill, knowledge, tools, outputSchema, active version and pending version if any. Use before editing."
}
func (t *GetAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)
}
func (t *GetAgent) IsReadOnly() bool        { return true }
func (t *GetAgent) NeedsReadFirst() bool    { return false }
func (t *GetAgent) RequiresWorkspace() bool { return false }
func (t *GetAgent) ValidateInput(args json.RawMessage) error {
	var a struct{ ID string `json:"id"` }
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}
func (t *GetAgent) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (t *GetAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct{ ID string `json:"id"` }
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_agent: %w", err)
	}
	a, err := t.svc.Get(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_agent: %w", err)
	}
	b, _ := json.Marshal(a)
	return string(b), nil
}

// ── CreateAgent ───────────────────────────────────────────────────────────────

type CreateAgent struct{ svc *agentapp.Service }

func (t *CreateAgent) Name() string { return "create_agent" }
func (t *CreateAgent) Description() string {
	return `Create a new agent (configured LLM worker). v1 auto-accepts.

FIELD SHAPES:
  outputSchema: {"kind":"free_text"} | {"kind":"enum","enums":["a","b","c"]} | {"kind":"json_schema","schema":{...JSON Schema...}}
  tools: [{"ref":"fn_xxx"},{"ref":"hd_xxx.method"},{"ref":"mcp:server/tool"}]
         NEVER include "ag_" refs — agents cannot call other agents.
  knowledge: ["doc_xxx","doc_yyy"]  (document IDs; attached as knowledge base)
  skill: "skill-name"  (optional; max 1 skill)

WHEN TO CREATE AN AGENT (not a function):
  - Classification / routing / intent detection / extraction → agent with outputSchema=enum
  - Multi-step reasoning over data → agent with tools
  - Knowledge-base Q&A → agent with knowledge docs

IMPOSSIBLE CAPABILITY RULE: Only write capabilities the agent can actually fulfill with its tools.
If it needs external data, attach a forge function/handler as a tool or use knowledge docs.

Keep description to one short line — it appears in the capability menu.`
}
func (t *CreateAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":         {"type": "string"},
			"description":  {"type": "string", "description": "One short line for the capability menu"},
			"prompt":       {"type": "string"},
			"skill":        {"type": "string", "description": "Optional single skill name to pre-activate"},
			"knowledge":    {"type": "array", "items": {"type":"string"}, "description": "Document IDs"},
			"tools":        {"type": "array", "items": {"type":"object"}, "description": "[{ref:'fn_xxx'|'hd_xxx.method'|'mcp:server/tool'}]; no ag_ refs"},
			"outputSchema": {"type": "object", "description": "{kind:'free_text'|'enum'|'json_schema', enums?:[...], schema?:{...}}"},
			"modelOverride":{"type": "string", "description": "Optional model ID override"},
			"changeReason": {"type": "string"}
		},
		"required": ["name", "prompt"]
	}`)
}
func (t *CreateAgent) IsReadOnly() bool        { return false }
func (t *CreateAgent) NeedsReadFirst() bool    { return false }
func (t *CreateAgent) RequiresWorkspace() bool { return false }
func (t *CreateAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return err
	}
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if a.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}
func (t *CreateAgent) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (t *CreateAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name          string                   `json:"name"`
		Description   string                   `json:"description"`
		Tags          []string                 `json:"tags"`
		Prompt        string                   `json:"prompt"`
		Skill         string                   `json:"skill"`
		Knowledge     []string                 `json:"knowledge"`
		Tools         []agentdomain.ToolRef    `json:"tools"`
		OutputSchema  *agentdomain.OutputSchema `json:"outputSchema"`
		ModelOverride string                   `json:"modelOverride"`
		ChangeReason  string                   `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_agent: %w", err)
	}
	a, v, err := t.svc.Create(ctx, agentapp.CreateInput{
		Name: args.Name, Description: args.Description, Tags: args.Tags,
		Prompt: args.Prompt, Skill: args.Skill, Knowledge: args.Knowledge,
		Tools: args.Tools, OutputSchema: args.OutputSchema,
		ModelOverride: args.ModelOverride, ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("create_agent: %w", err)
	}
	out := map[string]any{
		"id": a.ID, "name": a.Name,
		"versionId": v.ID, "activeVersionId": a.ActiveVersionID,
		"next_step": "Agent created. Reference it as " + a.ID + " in a workflow agent node (config.agentRef) or a tool node (config.callable=" + a.ID + ").",
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── EditAgent ─────────────────────────────────────────────────────────────────

type EditAgent struct{ svc *agentapp.Service }

func (t *EditAgent) Name() string { return "edit_agent" }
func (t *EditAgent) Description() string {
	return `Edit an agent — creates a pending version. Repeated edits rewrite the same pending (iterate-same-pending). User must accept or reject via accept_pending_agent.

tools field is REPLACE (not merge) — include ALL tools you want, not just changed ones.`
}
func (t *EditAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id":           {"type": "string"},
			"prompt":       {"type": "string"},
			"skill":        {"type": "string"},
			"knowledge":    {"type": "array", "items": {"type":"string"}},
			"tools":        {"type": "array", "items": {"type":"object"}, "description": "REPLACE semantics — include all tools"},
			"outputSchema": {"type": "object"},
			"modelOverride":{"type": "string"},
			"changeReason": {"type": "string"}
		},
		"required": ["id"]
	}`)
}
func (t *EditAgent) IsReadOnly() bool        { return false }
func (t *EditAgent) NeedsReadFirst() bool    { return true }
func (t *EditAgent) RequiresWorkspace() bool { return false }
func (t *EditAgent) ValidateInput(args json.RawMessage) error {
	var a struct{ ID string `json:"id"` }
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}
func (t *EditAgent) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (t *EditAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID            string                    `json:"id"`
		Prompt        *string                   `json:"prompt"`
		Skill         *string                   `json:"skill"`
		Knowledge     []string                  `json:"knowledge"`
		Tools         []agentdomain.ToolRef     `json:"tools"`
		OutputSchema  *agentdomain.OutputSchema  `json:"outputSchema"`
		ModelOverride *string                   `json:"modelOverride"`
		ChangeReason  string                    `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_agent: %w", err)
	}
	v, err := t.svc.Edit(ctx, agentapp.EditInput{
		ID: args.ID, Prompt: args.Prompt, Skill: args.Skill,
		Knowledge: args.Knowledge, Tools: args.Tools,
		OutputSchema: args.OutputSchema, ModelOverride: args.ModelOverride,
		ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("edit_agent: %w", err)
	}
	out := map[string]any{"pendingId": v.ID, "agentId": args.ID}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── AcceptPendingAgent ────────────────────────────────────────────────────────

type AcceptPendingAgent struct{ svc *agentapp.Service }

func (t *AcceptPendingAgent) Name() string { return "accept_pending_agent" }
func (t *AcceptPendingAgent) Description() string {
	return "Accept the pending agent version, making it the active version. The previous active version is retired."
}
func (t *AcceptPendingAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)
}
func (t *AcceptPendingAgent) IsReadOnly() bool        { return false }
func (t *AcceptPendingAgent) NeedsReadFirst() bool    { return false }
func (t *AcceptPendingAgent) RequiresWorkspace() bool { return false }
func (t *AcceptPendingAgent) ValidateInput(args json.RawMessage) error {
	var a struct{ ID string `json:"id"` }
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}
func (t *AcceptPendingAgent) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (t *AcceptPendingAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct{ ID string `json:"id"` }
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("accept_pending_agent: %w", err)
	}
	v, err := t.svc.Accept(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("accept_pending_agent: %w", err)
	}
	out := map[string]any{"agentId": args.ID, "versionId": v.ID, "accepted": true}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── DeleteAgent ───────────────────────────────────────────────────────────────

type DeleteAgent struct{ svc *agentapp.Service }

func (t *DeleteAgent) Name() string { return "delete_agent" }
func (t *DeleteAgent) Description() string {
	return "Soft-delete an agent. Workflows referencing it become needs_attention."
}
func (t *DeleteAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)
}
func (t *DeleteAgent) IsReadOnly() bool        { return false }
func (t *DeleteAgent) NeedsReadFirst() bool    { return false }
func (t *DeleteAgent) RequiresWorkspace() bool { return false }
func (t *DeleteAgent) ValidateInput(args json.RawMessage) error {
	var a struct{ ID string `json:"id"` }
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}
func (t *DeleteAgent) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (t *DeleteAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct{ ID string `json:"id"` }
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_agent: %w", err)
	}
	if err := t.svc.Delete(ctx, args.ID); err != nil {
		return "", fmt.Errorf("delete_agent: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"deleted": true, "id": args.ID})
	return string(b), nil
}

// containsSub is a simple case-insensitive substring check.
func containsSub(s, sub string) bool {
	if sub == "" {
		return true
	}
	ls, lsub := toLower(s), toLower(sub)
	for i := 0; i <= len(ls)-len(lsub); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}
