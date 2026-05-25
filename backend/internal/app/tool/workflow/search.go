package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type SearchWorkflow struct {
	svc *workflowapp.Service
	log *zap.Logger
}

func (t *SearchWorkflow) Name() string { return "search_workflow" }

func (t *SearchWorkflow) Description() string {
	return "Search the user's workflows by substring over name/description/tags (empty=list all). Returns id, name, description, tags, enabled, activeVersionId, needsAttention."
}

func (t *SearchWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Substring to search for; empty = list all"},
			"limit": {"type": "integer", "description": "Max results (default 10)"}
		}
	}`)
}

func (t *SearchWorkflow) IsReadOnly() bool        { return true }
func (t *SearchWorkflow) NeedsReadFirst() bool    { return false }
func (t *SearchWorkflow) RequiresWorkspace() bool { return false }

func (t *SearchWorkflow) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchWorkflow) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_workflow: bad args: %w", err)
	}
	if args.Limit <= 0 {
		args.Limit = 10
	}
	rows, err := t.svc.Search(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("search_workflow: %w", err)
	}
	if len(rows) > args.Limit {
		rows = rows[:args.Limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, w := range rows {
		out = append(out, map[string]any{
			"id":              w.ID,
			"name":            w.Name,
			"description":     w.Description,
			"tags":            w.Tags,
			"enabled":         w.Enabled,
			"activeVersionId": w.ActiveVersionID,
			"needsAttention":  w.NeedsAttention,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
