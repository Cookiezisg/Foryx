// generator.go — LLM-driven Summary builder. Implements the Generator
// interface that Service.Refresh consults when source items change.
//
// Single-attempt design (post-2026-05-08 屎山拯救计划 #7): one LLM call,
// parse + validate that the Summary is non-empty, return on success.
// Any failure (transport / parse / empty summary / output overflow)
// returns ErrGenerationFailed so Service.Refresh falls back to
// mechanicalFallback. The previous "3-attempt retry + coverage
// validation + missing-id hint augmentation" was over-engineered for
// the actual failure modes (modern LLMs succeed first-try at this
// task ~99%; transport-level failures don't recover via retry; the 1s
// polling loop already provides natural retry on the next user activity).
//
// Output cap: ~10 KB defensive char limit (~2000 tokens). Past that →
// treat as malformed and fall back to mechanical.
//
// generator.go ——LLM-driven Summary 构建。实现 Service.Refresh 在 source
// items 变时查的 Generator 接口。
//
// 单次设计（2026-05-08 屎山拯救计划 #7 后）：调一次 LLM，解析 + 校验 Summary
// 非空，成功就返。任何失败（传输 / 解析 / Summary 空 / 输出溢出）返
// ErrGenerationFailed 让 Service.Refresh 退 mechanicalFallback。原 "3 次重试
// + coverage 校验 + 漏 ID hint" 对实际失败模式过设计——现代 LLM 这种任务首试
// ~99%；传输层失败重试无效；catalog 1s 轮询本身已是用户下次活动时的自然重试。
//
// 输出上限 ~10 KB（~2000 token）防御 cap。超之视畸形退 mechanical。
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// generatorOutputCharCap is the ~10 KB defensive cap. The LLM's
// max_tokens=2000 yields ~8 KB at typical English; 10 KB gives headroom.
// Past this we treat as malformed.
//
// generatorOutputCharCap：~10 KB 防御上限。LLM max_tokens=2000 典型英文 ~8 KB；
// 10 KB 留余。超之视畸形。
const generatorOutputCharCap = 10 * 1024

// LLMGenerator is the production Generator. Constructed in main.go;
// plugged into Service via SetGenerator post-construction.
//
// LLMGenerator 是生产 Generator。main.go 构造；经 SetGenerator 后置注入。
type LLMGenerator struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	log     *zap.Logger
}

// NewLLMGenerator constructs an LLMGenerator. picker / keys / factory
// are the same triplet used by mcp.Search + skill.Search + forge.search
// — wired in main.go from existing services.
//
// NewLLMGenerator 构造 LLMGenerator。picker / keys / factory 三元组同
// mcp.Search + skill.Search + forge.search——main.go 接已有 service。
func NewLLMGenerator(picker modeldomain.ModelPicker, keys apikeydomain.KeyProvider, factory *llminfra.Factory, log *zap.Logger) *LLMGenerator {
	if log == nil {
		log = zap.NewNop()
	}
	return &LLMGenerator{
		picker:  picker,
		keys:    keys,
		factory: factory,
		log:     log.Named("catalog.generator"),
	}
}

// Generate makes one LLM call to produce the Summary. Any failure
// (resolve / transport / parse / empty Summary / output overflow)
// returns ErrGenerationFailed so Service.Refresh switches to
// mechanicalFallback. Coverage from the LLM is passed through verbatim
// without validation — historic 3-attempt retry + missing-id hint
// augmentation removed per 屎山拯救计划 #7 (modern LLMs first-try this
// task ~99% successfully; the 1s polling loop is the natural retry).
//
// Generate 调一次 LLM 生成 Summary。任何失败（resolve / 传输 / 解析 / Summary
// 空 / 输出溢出）返 ErrGenerationFailed 让 Service.Refresh 切 mechanicalFallback。
// LLM 返的 Coverage 原样透传不校验——历史的 3 次重试 + 漏 ID hint 按屎山拯救
// 计划 #7 删（现代 LLM 这种任务首试 ~99%；catalog 1s 轮询是自然重试）。
func (g *LLMGenerator) Generate(ctx context.Context, items []catalogdomain.Item, gMap map[string]catalogdomain.Granularity) (*catalogdomain.Catalog, error) {
	if len(items) == 0 {
		return mechanicalFallback(items, gMap), nil
	}

	bundle, err := llmclientpkg.Resolve(ctx, g.picker, g.keys, g.factory)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve LLM: %v", catalogdomain.ErrGenerationFailed, err)
	}

	prompt := buildPrompt(items, gMap)

	raw, err := llminfra.Generate(ctx, bundle.Client, llminfra.Request{
		ModelID: bundle.ModelID,
		Key:     bundle.Key,
		BaseURL: bundle.BaseURL,
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		g.log.Warn("catalog generation LLM call failed", zap.Error(err))
		return nil, fmt.Errorf("%w: %v", catalogdomain.ErrGenerationFailed, err)
	}

	if len(raw) > generatorOutputCharCap {
		g.log.Warn("catalog generation output exceeds char cap; falling back to mechanical",
			zap.Int("chars", len(raw)))
		return nil, fmt.Errorf("%w: output exceeded %d chars", catalogdomain.ErrGenerationFailed, generatorOutputCharCap)
	}

	jsonStr, ok := llmparsepkg.ExtractJSON(raw)
	if !ok {
		g.log.Warn("catalog generation: no JSON in LLM response; falling back to mechanical",
			zap.String("response_snippet", trimResp(raw, 200)))
		return nil, fmt.Errorf("%w: no JSON in response", catalogdomain.ErrGenerationFailed)
	}

	// 2026-05 #13 fix: LLM only generates Summary text; Coverage is computed
	// from the raw items list by source name (function/handler/workflow/mcp/skill).
	// Stable for the frontend, doesn't rely on LLM-decided semantic categories.
	// 2026-05 #13: LLM 只生成 summary 文本;coverage 由 backend 按 source 名
	// 直接从 items 拼装,稳定可消费。
	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		g.log.Warn("catalog generation: JSON parse failed; falling back to mechanical", zap.Error(err))
		return nil, fmt.Errorf("%w: parse JSON: %v", catalogdomain.ErrGenerationFailed, err)
	}

	if strings.TrimSpace(parsed.Summary) == "" {
		g.log.Warn("catalog generation: empty Summary; falling back to mechanical")
		return nil, fmt.Errorf("%w: empty Summary", catalogdomain.ErrGenerationFailed)
	}

	return &catalogdomain.Catalog{
		Summary:     parsed.Summary,
		Coverage:    computeCoverage(items),
		GeneratedBy: "llm",
	}, nil
}

// computeCoverage groups items by their Source field into the coverage
// map. Source values come straight from each CatalogSource.Name() (e.g.
// "function" / "handler" / "workflow" / "mcp" / "skill") — frontend
// and tests can index by these stable keys, no LLM semantic ambiguity.
//
// computeCoverage 按 Source 字段把 items 分组成 coverage map;Source 直接
// 来自 CatalogSource.Name(),稳定可消费。
func computeCoverage(items []catalogdomain.Item) map[string][]string {
	out := make(map[string][]string)
	for _, it := range items {
		if it.Source == "" {
			continue
		}
		out[it.Source] = append(out[it.Source], it.ID)
	}
	// Sort each source's IDs for stable output (so fingerprint doesn't churn).
	// 每个 source 的 ID 排序保 fingerprint 稳定。
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// ── prompt + parsing helpers ─────────────────────────────────────────

const generatorPromptTemplate = `You are generating a "Capability Catalog" summary that will be inserted into another LLM's system prompt.
The summary tells the other LLM what high-level capability categories are available, when to use each, and how to discover details.

CONSTRAINTS — ALL MANDATORY:
1. Coverage: every item below MUST be referenced (directly named or grouped). Don't lose items.
2. Brevity: total summary <= 600 tokens. Prefer "5 file-processing tools" over listing 5 names.
3. Granularity rules:
   - source granularity=PerItem (function, handler, workflow, skill): grouping/merging allowed
   - source granularity=PerServer (mcp): one mention per server, do NOT merge
4. Detect overlap and write routing observations inline: If two items in different
   sources serve similar purposes (e.g., a forged function that calls GitHub API + a github MCP server),
   add a "Notes on choosing" section telling the LLM which to prefer and why.
   Inferences should come from the item descriptions provided below.
5. End with: "If a task could fit multiple categories, you MAY call multiple search tools in parallel."

OUTPUT JSON only (no surrounding prose, no markdown fences). Just one field:
{
  "summary": "<markdown text>"
}

(The system will compute a coverage map mechanically by source name —
function/handler/workflow/mcp/skill — so you don't need to output one.)

ITEMS:
%s`

// buildPrompt assembles the LLM request. Items rendered as a per-source
// list with id + name + description so the LLM has both a stable handle
// (id) for the coverage field and human-readable text for the summary.
//
// buildPrompt 装 LLM 请求。items 按 source 列出 + id + name + description
// ——LLM 既有稳定 handle（id）填 coverage，又有人类可读文本写 summary。
func buildPrompt(items []catalogdomain.Item, gMap map[string]catalogdomain.Granularity) string {
	var itemsBlock strings.Builder
	bySource := groupBySource(items)
	sourceNames := make([]string, 0, len(bySource))
	for name := range bySource {
		sourceNames = append(sourceNames, name)
	}
	sort.Strings(sourceNames)
	for _, srcName := range sourceNames {
		gran := gMap[srcName]
		fmt.Fprintf(&itemsBlock, "\n[%s, granularity=%s]\n", srcName, gran.String())
		srcItems := bySource[srcName]
		sort.Slice(srcItems, func(i, j int) bool { return srcItems[i].Name < srcItems[j].Name })
		for _, it := range srcItems {
			fmt.Fprintf(&itemsBlock, "  - id=%q name=%q description=%q\n", it.ID, it.Name, it.Description)
		}
	}
	return fmt.Sprintf(generatorPromptTemplate, itemsBlock.String())
}

func trimResp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
