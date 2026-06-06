package tool

// Toolset partitions system tools into always-present resident tools and lazily-loaded
// groups. Resident tools are in every LLM turn; a Lazy group's tools stay collapsed — the
// system prompt only names the category — until the LLM calls activate_tools to pull the
// group in. This caps prompt tokens when some tools carry large descriptions. The
// resident/lazy split, the activate_tools tool, and the per-conversation activation state
// are assembled by chat (M5.2); this struct is just the partition.
//
// Toolset 把系统工具分成常驻 resident 与按需加载的 lazy 组。Resident 每个 LLM 回合都在；Lazy 组
// 的工具保持收起——系统 prompt 只报类名——直到 LLM 调 activate_tools 把该组拉入。这在部分工具携带
// 巨大 description 时给 prompt token 设上限。resident/lazy 划分、activate_tools 工具、每对话激活
// 状态由 chat（M5.2）组装；本结构只是那份划分。
type Toolset struct {
	// Resident tools are present in every LLM turn.
	//
	// Resident 工具每个 LLM 回合都在。
	Resident []Tool

	// Lazy maps a category name to the tools loaded only after activate_tools pulls it in.
	//
	// Lazy 把类名映射到只有 activate_tools 拉入后才加载的工具。
	Lazy map[string][]Tool
}

// All returns Resident followed by every Lazy group flattened — the full inventory, for a
// tools-overview handler. Lazy group order follows Go map iteration (unspecified); a
// caller needing a stable order sorts by Tool.Name.
//
// All 返回 Resident 后接所有 Lazy 组展平——全量清单，给工具总览 handler。Lazy 组顺序随 Go map
// 迭代（未定）；要稳定顺序的调用方按 Tool.Name 排序。
func (ts Toolset) All() []Tool {
	total := len(ts.Resident)
	for _, g := range ts.Lazy {
		total += len(g)
	}
	out := make([]Tool, 0, total)
	out = append(out, ts.Resident...)
	for _, g := range ts.Lazy {
		out = append(out, g...)
	}
	return out
}
