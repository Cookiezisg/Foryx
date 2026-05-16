package function

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// SearchFunction implements search_function.
//
// SearchFunction 实现 search_function。
type SearchFunction struct {
	svc     *functionapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	log     *zap.Logger
}

func (t *SearchFunction) Name() string { return "search_function" }

func (t *SearchFunction) Description() string {
	return "Search the user's function library for relevant functions given a query. " +
		"Returns up to limit functions ranked by relevance. " +
		"Use get_function to inspect the full code of a candidate before running it."
}

func (t *SearchFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Natural language description of what you're looking for"},
			"limit": {"type": "integer", "description": "Maximum results to return (default 3, max 5)"}
		},
		"required": ["query"]
	}`)
}

func (t *SearchFunction) IsReadOnly() bool        { return true }
func (t *SearchFunction) NeedsReadFirst() bool    { return false }
func (t *SearchFunction) RequiresWorkspace() bool { return false }

func (t *SearchFunction) ValidateInput(json.RawMessage) error { return nil }

func (t *SearchFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_function: bad args: %w", err)
	}
	if args.Limit <= 0 || args.Limit > 5 {
		args.Limit = 3
	}

	fns, err := t.svc.ListAll(ctx)
	if err != nil {
		return "", fmt.Errorf("search_function: list: %w", err)
	}
	if len(fns) == 0 {
		b, _ := json.Marshal([]any{})
		return string(b), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nFunctions:\n", args.Query)
	for _, f := range fns {
		fmt.Fprintf(&sb, "- id: %s, name: %s, description: %s\n", f.ID, f.Name, f.Description)
	}
	fmt.Fprintf(&sb, "\nReturn the %d most relevant function IDs as JSON: "+
		`[{"id":"fn_xxx","score":0.95},...]`+
		"\nRespond with valid JSON only.", args.Limit)

	bc, err := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", fmt.Errorf("search_function: %w", err)
	}
	resp, err := llminfra.Generate(ctx, bc.Client, llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: sb.String()}},
	})
	if err != nil {
		return "", fmt.Errorf("search_function: llm: %w", err)
	}

	var ranked []struct {
		ID    string  `json:"id"`
		Score float32 `json:"score"`
	}
	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		return "", fmt.Errorf("search_function: LLM response contained no JSON: %w: %q", llminfra.ErrProviderError, resp)
	}
	if err = json.Unmarshal([]byte(jsonStr), &ranked); err != nil {
		return "", fmt.Errorf("search_function: parse ranking: %w", err)
	}

	byID := make(map[string]int, len(fns))
	for i, f := range fns {
		byID[f.ID] = i
	}

	type result struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Tags        []string `json:"tags"`
		Score       float32 `json:"score"`
	}
	out := make([]result, 0, len(ranked))
	for _, r := range ranked {
		idx, ok := byID[r.ID]
		if !ok {
			t.log.Warn("functiontool.SearchFunction: LLM returned unknown function id", zap.String("id", r.ID))
			continue
		}
		f := fns[idx]
		out = append(out, result{
			ID: f.ID, Name: f.Name, Description: f.Description,
			Tags: f.Tags, Score: r.Score,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
