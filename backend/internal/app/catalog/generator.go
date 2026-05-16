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

const generatorOutputCharCap = 10 * 1024

// LLMGenerator is the production Generator wired in main.go.
//
// LLMGenerator 是生产环境的 Generator，由 main.go 装配。
type LLMGenerator struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	log     *zap.Logger
}

// NewLLMGenerator constructs an LLMGenerator wired with picker/keys/factory.
//
// NewLLMGenerator 用 picker/keys/factory 构造 LLMGenerator。
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

// Generate makes one LLM call; any failure returns ErrGenerationFailed.
//
// Generate 调一次 LLM；任何失败返 ErrGenerationFailed 触发 mechanical fallback。
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

func computeCoverage(items []catalogdomain.Item) map[string][]string {
	out := make(map[string][]string)
	for _, it := range items {
		if it.Source == "" {
			continue
		}
		out[it.Source] = append(out[it.Source], it.ID)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

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
