// search.go — search_forges system tool: ranks the user's forge library by
// relevance to a natural-language query using the LLM.
//
// search.go — search_forges 系统工具：用 LLM 按自然语言查询对用户 forge 库做相关性排序。
package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// SearchForge implements the search_forges system tool.
//
// SearchForge 实现 search_forges 系统工具。
type SearchForge struct {
	svc     *forgeapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	log     *zap.Logger
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (t *SearchForge) Name() string { return "search_forges" }

func (t *SearchForge) Description() string {
	return "Search the user's forge library for relevant forges given a query. " +
		"Returns up to limit forges ranked by relevance. " +
		"Use get_forge to inspect the full code of a candidate before running it."
}

func (t *SearchForge) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Natural language description of what you're looking for"},
			"limit": {"type": "integer", "description": "Maximum results to return (default 3, max 5)"}
		},
		"required": ["query"]
	}`)
}

// ── Static metadata ───────────────────────────────────────────────────────────

func (t *SearchForge) IsReadOnly() bool        { return true }
func (t *SearchForge) NeedsReadFirst() bool    { return false }
func (t *SearchForge) RequiresWorkspace() bool { return false }

// ── Args-dependent hooks ──────────────────────────────────────────────────────


func (t *SearchForge) ValidateInput(json.RawMessage) error { return nil }

func (t *SearchForge) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────────────

func (t *SearchForge) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_forges: bad args: %w", err)
	}
	if args.Limit <= 0 || args.Limit > 5 {
		args.Limit = 3
	}

	forges, err := t.svc.ListAll(ctx)
	if err != nil {
		return "", fmt.Errorf("search_forges: list: %w", err)
	}
	if len(forges) == 0 {
		b, _ := json.Marshal([]any{})
		return string(b), nil
	}

	// Build LLM prompt: query + forge catalog → ranked id+score JSON.
	// 构建 LLM prompt：query + forge 清单 → 排序好的 id+score JSON。
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nForges:\n", args.Query)
	for _, f := range forges {
		fmt.Fprintf(&sb, "- id: %s, name: %s, description: %s\n", f.ID, f.Name, f.Description)
	}
	fmt.Fprintf(&sb, "\nReturn the %d most relevant forge IDs as JSON: "+
		`[{"id":"f_xxx","score":0.95},...]`+
		"\nRespond with valid JSON only.", args.Limit)

	bc, err := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", fmt.Errorf("search_forges: %w", err)
	}
	resp, err := llminfra.Generate(ctx, bc.Client, llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: sb.String()}},
	})
	if err != nil {
		return "", fmt.Errorf("search_forges: llm: %w", err)
	}

	var ranked []struct {
		ID    string  `json:"id"`
		Score float32 `json:"score"`
	}
	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		// Wrap with llm.ErrProviderError sentinel so callers can errors.Is
		// — same pattern as other LLM-call-failure paths (mcp calltool, etc).
		// 用 llm.ErrProviderError sentinel 包，让调用方 errors.Is 区分。
		return "", fmt.Errorf("search_forges: LLM response contained no JSON: %w: %q", llminfra.ErrProviderError, resp)
	}
	if err = json.Unmarshal([]byte(jsonStr), &ranked); err != nil {
		return "", fmt.Errorf("search_forges: parse ranking: %w", err)
	}

	ids := make([]string, len(ranked))
	scoreMap := make(map[string]float32, len(ranked))
	for i, r := range ranked {
		ids[i] = r.ID
		scoreMap[r.ID] = r.Score
	}

	fetched, err := t.svc.GetForgesByIDs(ctx, ids)
	if err != nil {
		return "", fmt.Errorf("search_forges: fetch: %w", err)
	}

	// Score is the LLM's relevance score (0-1), NOT a vector cosine similarity.
	// Naming it "score" instead of "similarity" avoids misleading the LLM into
	// thinking it's a calibrated similarity metric.
	//
	// Score 是 LLM 自评的相关性分数（0-1），不是向量 cosine similarity。
	// 改名 "score" 避免误导 LLM 以为是经过校准的相似度指标。
	type result struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		Parameters   any     `json:"parameters"`
		ReturnSchema any     `json:"returnSchema"`
		Score        float32 `json:"score"`
	}
	out := make([]result, 0, len(fetched))
	for _, f := range fetched {
		// Unmarshal errors here mean DB data is corrupted for this forge;
		// keep the forge in the result with nil schemas rather than aborting
		// search. Log Warn so the corruption surfaces in operator logs
		// instead of silently producing forges-without-schemas in search
		// results — same defect-class lesson as B2 silent fallback.
		// DB 损坏时保留 forge 但 schema=nil 不中止搜索。Warn log 让损坏
		// 在 operator 日志可见——同 B2 silent fallback 经验。
		var params, ret any
		if err := json.Unmarshal([]byte(f.Parameters), &params); err != nil && t.log != nil {
			t.log.Warn("forgetool.SearchForge: corrupt Parameters JSON; using nil schema in search result",
				zap.String("forge_id", f.ID), zap.Error(err))
		}
		if err := json.Unmarshal([]byte(f.ReturnSchema), &ret); err != nil && t.log != nil {
			t.log.Warn("forgetool.SearchForge: corrupt ReturnSchema JSON; using nil schema in search result",
				zap.String("forge_id", f.ID), zap.Error(err))
		}
		out = append(out, result{
			ID: f.ID, Name: f.Name, Description: f.Description,
			Parameters: params, ReturnSchema: ret, Score: scoreMap[f.ID],
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
