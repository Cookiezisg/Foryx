package approval

import (
	"context"
	"encoding/json"
	"fmt"

	approvalapp "github.com/sunweilin/forgify/backend/internal/app/approval"
)

// --- search_approval -------------------------------------------------------

type SearchApproval struct{ svc *approvalapp.Service }

func (t *SearchApproval) Name() string { return "search_approval" }

func (t *SearchApproval) Description() string {
	return "Find approval forms by case-insensitive substring over name / description. Returns id + name + description; empty query lists all. Use get_approval for the full template + rules."
}

func (t *SearchApproval) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Substring to match; omit or empty to list all."}
		}
	}`)
}

func (t *SearchApproval) ValidateInput(json.RawMessage) error { return nil }

func (t *SearchApproval) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_approval: bad args: %w", err)
	}
	forms, err := t.svc.Search(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("search_approval: %w", err)
	}
	type slim struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := make([]slim, 0, len(forms))
	for _, f := range forms {
		out = append(out, slim{ID: f.ID, Name: f.Name, Description: f.Description})
	}
	return toJSON(map[string]any{"count": len(out), "approvals": out}), nil
}

// --- get_approval ----------------------------------------------------------

type GetApproval struct{ svc *approvalapp.Service }

func (t *GetApproval) Name() string { return "get_approval" }

func (t *GetApproval) Description() string {
	return "Get one approval form with its active version (template + allowReason + timeout + timeoutBehavior)."
}

func (t *GetApproval) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["approvalId"],
		"properties": {"approvalId": {"type": "string"}}
	}`)
}

func (t *GetApproval) ValidateInput(args json.RawMessage) error {
	var a struct {
		ApprovalID string `json:"approvalId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("get_approval: bad args: %w", err)
	}
	if a.ApprovalID == "" {
		return fmt.Errorf("get_approval: approvalId is required")
	}
	return nil
}

func (t *GetApproval) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ApprovalID string `json:"approvalId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_approval: bad args: %w", err)
	}
	f, err := t.svc.Get(ctx, args.ApprovalID)
	if err != nil {
		return "", fmt.Errorf("get_approval: %w", err)
	}
	return toJSON(f), nil
}
