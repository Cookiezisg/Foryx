package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
)

// --- revert_agent ----------------------------------------------------------

type RevertAgent struct{ svc *agentapp.Service }

func (t *RevertAgent) Name() string { return "revert_agent" }

func (t *RevertAgent) Description() string {
	return "Revert an agent's active version to an existing older version number (does not renumber). Use when a recent edit made it worse — the version history is the undo."
}

func (t *RevertAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["agentId","version"],"properties":{"agentId":{"type":"string"},"version":{"type":"integer","description":"Target version number to make active."}}}`)
}

func (t *RevertAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		AgentID string `json:"agentId"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("revert_agent: bad args: %w", err)
	}
	if strings.TrimSpace(a.AgentID) == "" || a.Version < 1 {
		return ErrRevertArgsRequired
	}
	return nil
}

func (t *RevertAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		AgentID string `json:"agentId"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("revert_agent: bad args: %w", err)
	}
	v, err := t.svc.Revert(ctx, a.AgentID, a.Version)
	if err != nil {
		return "", fmt.Errorf("revert_agent: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"agentId": a.AgentID, "versionId": v.ID, "version": v.Version}), nil
}

// --- delete_agent ----------------------------------------------------------

type DeleteAgent struct{ svc *agentapp.Service }

func (t *DeleteAgent) Name() string { return "delete_agent" }

func (t *DeleteAgent) Description() string {
	return "Delete an agent (soft-delete). Its relation edges to mounted skill/doc/fn/hd/mcp are removed; its execution history is retained."
}

func (t *DeleteAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["agentId"],"properties":{"agentId":{"type":"string"}}}`)
}

func (t *DeleteAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_agent: bad args: %w", err)
	}
	if strings.TrimSpace(a.AgentID) == "" {
		return ErrAgentIDRequired
	}
	return nil
}

func (t *DeleteAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("delete_agent: bad args: %w", err)
	}
	if err := t.svc.Delete(ctx, a.AgentID); err != nil {
		return "", fmt.Errorf("delete_agent: %w", err)
	}
	return fmt.Sprintf("Deleted agent %q.", a.AgentID), nil
}

// --- invoke_agent ----------------------------------------------------------

type InvokeAgent struct{ svc *agentapp.Service }

func (t *InvokeAgent) Name() string { return "invoke_agent" }

func (t *InvokeAgent) Description() string {
	return "Run an agent: it executes its ReAct loop over the given input and returns the final output (shaped by its outputSchema). Find one with search_agent first. The run is recorded — inspect it later with search_agent_executions / get_agent_execution (the latter carries the full transcript)."
}

func (t *InvokeAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["agentId"],"properties":{"agentId":{"type":"string"},"input":{"type":"object","description":"Data fed to the agent (appended to its prompt)."}}}`)
}

func (t *InvokeAgent) ValidateInput(args json.RawMessage) error {
	var a struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invoke_agent: bad args: %w", err)
	}
	if strings.TrimSpace(a.AgentID) == "" {
		return ErrAgentIDRequired
	}
	return nil
}

func (t *InvokeAgent) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		AgentID string         `json:"agentId"`
		Input   map[string]any `json:"input"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("invoke_agent: bad args: %w", err)
	}
	res, err := t.svc.InvokeAgent(ctx, agentapp.InvokeInput{
		AgentID:     a.AgentID,
		Input:       a.Input,
		TriggeredBy: agentdomain.TriggeredByChat,
	})
	if err != nil {
		return "", fmt.Errorf("invoke_agent: %w", err)
	}
	return toolapp.ToJSON(res), nil
}
