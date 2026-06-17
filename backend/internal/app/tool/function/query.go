package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/anselm/backend/internal/app/function"
	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	searchdomain "github.com/sunweilin/anselm/backend/internal/domain/search"
)

// --- search_function -------------------------------------------------------

type SearchFunction struct {
	svc     *functionapp.Service
	content *searchapp.Service // nil → legacy substring only. nil → 仅原子串路径。
}

func (t *SearchFunction) Name() string { return "search_function" }

func (t *SearchFunction) Description() string {
	return "Find functions by case-insensitive substring over name / description / tags. Returns id + name + description; empty query lists all. Use get_function for full code + parameters."
}

func (t *SearchFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Substring to match; omit or empty to list all."}
		}
	}`)
}

func (t *SearchFunction) ValidateInput(json.RawMessage) error { return nil }

func (t *SearchFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_function: bad args: %w", err)
	}
	if body, ok := toolapp.ContentSearch(ctx, t.content, searchdomain.TypeFunction, args.Query, "functions"); ok {
		return body, nil
	}
	fns, err := t.svc.Search(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("search_function: %w", err)
	}
	out := make([]searchdomain.EntitySlim, 0, len(fns))
	for _, f := range fns {
		out = append(out, searchdomain.EntitySlim{ID: f.ID, Name: f.Name, Description: f.Description})
	}
	return toolapp.ToJSON(map[string]any{"count": len(out), "functions": out}), nil
}

// --- get_function ----------------------------------------------------------

type GetFunction struct{ svc *functionapp.Service }

func (t *GetFunction) Name() string { return "get_function" }

func (t *GetFunction) Description() string {
	return "Get one function with its active version (code, parameters, return schema, dependencies, env status)."
}

func (t *GetFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["functionId"],
		"properties": {"functionId": {"type": "string"}}
	}`)
}

func (t *GetFunction) ValidateInput(args json.RawMessage) error {
	var a struct {
		FunctionID string `json:"functionId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("get_function: bad args: %w", err)
	}
	if a.FunctionID == "" {
		return ErrFunctionIDRequired
	}
	return nil
}

func (t *GetFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID string `json:"functionId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_function: bad args: %w", err)
	}
	f, err := t.svc.Get(ctx, args.FunctionID)
	if err != nil {
		return "", fmt.Errorf("get_function: %w", err)
	}
	return toolapp.ToJSON(f), nil
}
