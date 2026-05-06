// search.go — Service.Search ranks skills by user query. Mirrors the
// mcp.Search / forge.Search "mode A" pattern: if the catalog is already
// small enough (≤ topK) skip the LLM and return everything; otherwise
// build a numbered prompt and parse the LLM's index list.
//
// search.go ——Service.Search 按 query 排序 skill。复用 mcp.Search /
// forge.Search "模式 A"：catalog 小（≤ topK）跳 LLM 全返；否则 prompt 编
// 号 + 解析 LLM 返的 index 列表。
package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// Search returns at most topK skills matching query. Empty catalog returns
// an empty slice; ≤ topK skills returns alpha-sorted; > topK invokes LLM
// ranking and falls back to alpha order if parsing fails.
//
// Search 返最多 topK 个匹配 query 的 skill。空 catalog 返空 slice；
// ≤ topK 字母序全返；> topK 调 LLM 排序，解析失败回字母序。
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

	bundle, err := llmclientpkg.Resolve(ctx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
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
		return nil, fmt.Errorf("skillapp.Search: llm: %w", err)
	}

	indices, err := parseRankedIndices(resp, len(all))
	if err != nil {
		// Ranking parse fail is non-fatal — return alpha-order top K so
		// the LLM caller still gets something usable. Log so author can
		// debug their catalog if it keeps happening.
		// 排序解析失败非致命——返字母序前 K 让 LLM 调用方仍有可用结果。
		// 持续发生时 author 可经 log 调 catalog。
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

// buildRankingPrompt assembles the LLM prompt for skill ranking. Skills
// are presented as numbered list with name + description (description
// truncated at 200 chars to keep prompt size predictable on big catalogs).
//
// buildRankingPrompt 装 skill 排序 prompt。skill 编号列表 + name +
// description（200 字符截断让大 catalog 的 prompt 大小可预测）。
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

// parseRankedIndices extracts the LLM-emitted index array. Tolerates
// markdown-fenced output via llmparsepkg.ExtractJSON.
//
// parseRankedIndices 提取 LLM 发的 index 数组。经 llmparsepkg.ExtractJSON
// 容忍 markdown 围栏。
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
