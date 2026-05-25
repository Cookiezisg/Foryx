package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

type SearchHandler struct {
	svc     *handlerapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	log     *zap.Logger
}

func (t *SearchHandler) Name() string { return "search_handler" }

func (t *SearchHandler) Description() string {
	return "Search the user's handler library by natural-language query, ranked by relevance. " +
		"Inspect a hit with get_handler (methods + configState) before call_handler."
}

func (t *SearchHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Natural language description"},
			"limit": {"type": "integer", "description": "Max results (default 3, max 5)"}
		},
		"required": ["query"]
	}`)
}

func (t *SearchHandler) IsReadOnly() bool        { return true }
func (t *SearchHandler) NeedsReadFirst() bool    { return false }
func (t *SearchHandler) RequiresWorkspace() bool { return false }

func (t *SearchHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_handler: bad args: %w", err)
	}
	if args.Limit <= 0 || args.Limit > 5 {
		args.Limit = 3
	}

	hs, err := t.svc.ListAll(ctx)
	if err != nil {
		return "", fmt.Errorf("search_handler: list: %w", err)
	}
	if len(hs) == 0 {
		b, _ := json.Marshal([]any{})
		return string(b), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nHandlers:\n", args.Query)
	for _, h := range hs {
		fmt.Fprintf(&sb, "- id: %s, name: %s, description: %s\n", h.ID, h.Name, h.Description)
	}
	fmt.Fprintf(&sb, "\nReturn the %d most relevant handler IDs as JSON: "+
		`[{"id":"hd_xxx","score":0.95},...]`+
		"\nRespond with valid JSON only.", args.Limit)

	bc, err := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", fmt.Errorf("search_handler: %w", err)
	}
	resp, err := llminfra.Generate(ctx, bc.Client, llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: sb.String()}},
	})
	if err != nil {
		return "", fmt.Errorf("search_handler: llm: %w", err)
	}

	var ranked []struct {
		ID    string  `json:"id"`
		Score float32 `json:"score"`
	}
	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		return "", fmt.Errorf("search_handler: LLM response contained no JSON: %w: %q", llminfra.ErrProviderError, resp)
	}
	if err := json.Unmarshal([]byte(jsonStr), &ranked); err != nil {
		return "", fmt.Errorf("search_handler: parse ranking: %w", err)
	}

	byID := make(map[string]int, len(hs))
	for i, h := range hs {
		byID[h.ID] = i
	}
	type result struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Score       float32  `json:"score"`
	}
	out := make([]result, 0, len(ranked))
	for _, r := range ranked {
		idx, ok := byID[r.ID]
		if !ok {
			t.log.Warn("handlertool.SearchHandler: LLM returned unknown id", zap.String("id", r.ID))
			continue
		}
		h := hs[idx]
		out = append(out, result{
			ID: h.ID, Name: h.Name, Description: h.Description, Tags: h.Tags, Score: r.Score,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
