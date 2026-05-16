package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// Search returns up to topK skills ranked for query; falls back to alpha order on parse fail.
//
// Search 返回最多 topK 个排序后的 skill；解析失败回退字母序。
func (s *Service) Search(ctx context.Context, query string, topK int) ([]*skilldomain.Skill, error) {
	if topK <= 0 {
		topK = 3
	}
	all := s.List(ctx)
	if len(all) == 0 {
		return []*skilldomain.Skill{}, nil
	}
	if len(all) <= topK {
		return all, nil
	}

	prompt := buildRankingPrompt(query, all, topK)

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress,
		map[string]any{"stage": "rerank", "tool": "search_skills", "candidates": len(all)})

	bundle, err := llmclientpkg.Resolve(ctx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return nil, fmt.Errorf("skillapp.Search: resolve LLM: %w", err)
	}
	resp, err := llminfra.Generate(ctx, bundle.Client, llminfra.Request{
		ModelID: bundle.ModelID,
		Key:     bundle.Key,
		BaseURL: bundle.BaseURL,
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return nil, fmt.Errorf("skillapp.Search: llm: %w", err)
	}
	em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	indices, err := parseRankedIndices(resp, len(all))
	if err != nil {
		s.log.Warn("skill search rank parse failed; falling back to alpha order",
			zap.String("query", query),
			zap.String("response_snippet", trimResp(resp, 200)),
			zap.Error(err))
		return all[:min(topK, len(all))], nil
	}

	out := make([]*skilldomain.Skill, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(all) {
			continue
		}
		out = append(out, all[idx])
		if len(out) >= topK {
			break
		}
	}
	if len(out) == 0 {
		return all[:min(topK, len(all))], nil
	}
	return out, nil
}

func buildRankingPrompt(query string, all []*skilldomain.Skill, topK int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nAvailable skills:\n", query)
	for i, sk := range all {
		desc := sk.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		fmt.Fprintf(&sb, "%d. %s — %s\n", i, sk.Name, desc)
	}
	fmt.Fprintf(&sb, "\nReturn the indices of the %d most relevant skills as a JSON array, "+
		"most relevant first: [3, 7, 1, ...]\n"+
		"Respond with valid JSON only, no surrounding prose.", topK)
	return sb.String()
}

func parseRankedIndices(resp string, total int) ([]int, error) {
	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		return nil, fmt.Errorf("no JSON in response: %q", trimResp(resp, 200))
	}
	var raw []int
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	out := make([]int, 0, len(raw))
	for _, idx := range raw {
		if idx >= 0 && idx < total {
			out = append(out, idx)
		}
	}
	return out, nil
}

func trimResp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
