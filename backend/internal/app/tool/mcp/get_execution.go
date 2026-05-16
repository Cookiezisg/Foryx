package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

type GetMCPCall struct {
	repo mcpdomain.CallRepository
}

func (t *GetMCPCall) Name() string { return "get_mcp_call" }

func (t *GetMCPCall) Description() string {
	return "Fetch one MCP tool call by id (mcl_xxx). Returns full input + output JSON " +
		"(no truncation), error details, server + tool name, timing."
}

func (t *GetMCPCall) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"id": {"type": "string"}},
		"required": ["id"]
	}`)
}

func (t *GetMCPCall) IsReadOnly() bool        { return true }
func (t *GetMCPCall) NeedsReadFirst() bool    { return false }
func (t *GetMCPCall) RequiresWorkspace() bool { return false }
func (t *GetMCPCall) ValidateInput(json.RawMessage) error { return nil }
func (t *GetMCPCall) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetMCPCall) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct{ ID string `json:"id"` }
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_mcp_call: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_mcp_call: id required")
	}
	row, err := t.repo.GetCallByID(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_mcp_call: %w", err)
	}
	_ = mcpdomain.CallStatusOK // keep import explicit
	b, _ := json.Marshal(row)
	return string(b), nil
}
