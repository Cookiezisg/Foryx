package catalog

import (
	"fmt"
	"sort"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// mechanicalFallback enumerates items per-source into a Markdown summary.
//
// mechanicalFallback 按 source 分组生成 Markdown summary，覆盖全部 item。
func mechanicalFallback(items []catalogdomain.Item, gMap map[string]catalogdomain.Granularity) *catalogdomain.Catalog {
	bySource := groupBySource(items)
	sourceNames := make([]string, 0, len(bySource))
	for name := range bySource {
		sourceNames = append(sourceNames, name)
	}
	sort.Strings(sourceNames)

	var b strings.Builder
	coverage := map[string][]string{}

	// Empty library: skip the section entirely (no awkward header-then-blank
	// when the user has not forged anything yet).
	// 空库:跳整段(用户还没锻造时避免 header 下空白怪态)。
	if len(items) > 0 {
		b.WriteString("## Available capabilities\n")

		for _, name := range sourceNames {
			srcItems := bySource[name]
			sort.Slice(srcItems, func(i, j int) bool { return srcItems[i].Name < srcItems[j].Name })

			gran := gMap[name]
			fmt.Fprintf(&b, "\n### %s (%d, %s)\n", name, len(srcItems), gran.String())
			ids := make([]string, 0, len(srcItems))
			for _, it := range srcItems {
				desc := it.Description
				if desc == "" {
					desc = "(no description)"
				}
				fmt.Fprintf(&b, "- **%s**: %s\n", it.Name, desc)
				ids = append(ids, it.ID)
			}
			coverage[name] = ids
		}

		b.WriteString("\nIf a task could fit multiple categories, you MAY call multiple search tools in parallel.\n")
	}

	return &catalogdomain.Catalog{
		Summary:     b.String(),
		Coverage:    coverage,
		GeneratedBy: "mechanical-fallback",
	}
}

func groupBySource(items []catalogdomain.Item) map[string][]catalogdomain.Item {
	out := map[string][]catalogdomain.Item{}
	for _, it := range items {
		out[it.Source] = append(out[it.Source], it)
	}
	return out
}
