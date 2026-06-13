package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	modelclientapp "github.com/sunweilin/forgify/backend/internal/app/modelclient"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// llmSifter backs the search_blocks precision chain (§7.4) with the utility
// model — resolution goes through modelclient (the one shared chain; a hand-rolled
// copy here once miswired base URL into the wire model id, AC-26). One short
// completion, strict numbers-only output; any failure makes the chain fall back
// to index ranking.
//
// llmSifter 用 utility 模型支撑 search_blocks 精度链（§7.4）——解析走 modelclient
// （唯一共享链；这里曾手抄一份并把 base URL 误接进线缆 model id，AC-26）。一次短
// 补全、严格只回编号；任何失败让链回退索引排序。
type llmSifter struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

func (f *llmSifter) Sift(ctx context.Context, query string, items []string, topN int) ([]int, error) {
	client, req, _, err := modelclientapp.Resolve(ctx, modeldomain.ScenarioUtility, nil, f.picker, f.keys, f.factory)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "You select workflow building blocks. The user needs: %q\n\nCandidate blocks:\n", query)
	for i, item := range items {
		fmt.Fprintf(&sb, "%d) %s\n", i+1, item)
	}
	fmt.Fprintf(&sb, "\nReturn ONLY a JSON array of the numbers of the best-matching blocks, best first, at most %d, e.g. [3,1]. No other text. Return [] if nothing fits.", topN)

	req.Messages = []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: sb.String()}}
	out, err := llminfra.Generate(ctx, client, req)
	if err != nil {
		return nil, err
	}
	return parseSiftPicks(out)
}

// parseSiftPicks extracts the first JSON int array from the reply (models love
// wrapping answers) and converts 1-based numbers to 0-based indexes.
//
// parseSiftPicks 从回复里取首个 JSON 整数数组（模型爱包话），并把 1 基编号转 0 基下标。
func parseSiftPicks(out string) ([]int, error) {
	start := strings.Index(out, "[")
	end := strings.LastIndex(out, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("sift: no JSON array in %q", out)
	}
	var nums []int
	if err := json.Unmarshal([]byte(out[start:end+1]), &nums); err != nil {
		return nil, fmt.Errorf("sift: parse %q: %w", out[start:end+1], err)
	}
	picks := make([]int, 0, len(nums))
	for _, n := range nums {
		picks = append(picks, n-1)
	}
	return picks, nil
}
