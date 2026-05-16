package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type RevertFunction struct {
	svc *functionapp.Service
}

func (t *RevertFunction) Name() string { return "revert_function" }

func (t *RevertFunction) Description() string {
	return "Revert a function's active version back to a previously-accepted version " +
		"number. Use list_function or get_function to see version history first."
}

func (t *RevertFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Function ID"},
			"targetVersion": {"type": "integer", "description": "Version number to revert to (must be already accepted)"}
		},
		"required": ["id", "targetVersion"]
	}`)
}

func (t *RevertFunction) IsReadOnly() bool        { return false }
func (t *RevertFunction) NeedsReadFirst() bool    { return false }
func (t *RevertFunction) RequiresWorkspace() bool { return false }

func (t *RevertFunction) ValidateInput(json.RawMessage) error { return nil }
func (t *RevertFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *RevertFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID            string `json:"id"`
		TargetVersion int    `json:"targetVersion"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_function: bad args: %w", err)
	}
	if args.ID == "" || args.TargetVersion <= 0 {
		return "", fmt.Errorf("revert_function: id + targetVersion required (targetVersion > 0)")
	}
	v, err := t.svc.Revert(ctx, args.ID, args.TargetVersion)
	if err != nil {
		return "", fmt.Errorf("revert_function: %w", err)
	}
	out := map[string]any{
		"versionId":     v.ID,
		"targetVersion": v.Version,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
