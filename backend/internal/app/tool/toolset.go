package tool

// Toolset partitions system tools into always-present resident tools and lazily-loaded groups.
// In T7 chat.Service.host.Tools() returns the full flattened set — the split enables T8 to
// gate lazy groups behind activate_tools without breaking existing behavior.
//
// Toolset 把系统工具分成常驻 resident 和按需加载的 lazy 组。
// T7 里 host.Tools() 返全集——拆分使 T8 可以用 activate_tools 按需加载 lazy 组。
type Toolset struct {
	// Resident tools are always present in every LLM turn.
	Resident []Tool
	// Lazy maps category name → tools only loaded after activate_tools calls ActivateGroup.
	Lazy map[string][]Tool
}

// All returns Resident + all Lazy groups flattened; order: resident first, then lazy by insertion
// order of the map (undefined in Go, but stable-enough for tests that check the set not the order).
//
// All 返回 Resident + 所有 Lazy 组展开；resident 优先，lazy 顺序不定（Go map 无序）。
func (ts Toolset) All() []Tool {
	total := len(ts.Resident)
	for _, v := range ts.Lazy {
		total += len(v)
	}
	out := make([]Tool, 0, total)
	out = append(out, ts.Resident...)
	for _, group := range ts.Lazy {
		out = append(out, group...)
	}
	return out
}
