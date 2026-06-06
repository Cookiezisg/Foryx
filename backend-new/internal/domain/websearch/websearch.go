// Package websearch is the domain layer for web-search configuration — the
// provider vocabulary and the port through which a caller learns which search
// api-key the current workspace has chosen.
//
// Like domain/model it is a thin type+port layer with NO store: the chosen key
// id is stored on the workspace row (alongside the default-model selections),
// and workspace.Service implements SearchKeyPicker. The WebSearch tool
// (tool/web) depends on this package for the provider constants and the picker;
// the per-provider HTTP calls live in the tool, not here.
//
// Named "websearch" (not "search") because tool/search is the unrelated
// filesystem search (Glob/Grep/LS). This is network search.
//
// Package websearch 是网络搜索配置的 domain 层——provider 词表 + 调用方据以得知当前
// workspace 选定了哪把搜索 api-key 的端口。
//
// 同 domain/model，是仅含类型 + 端口的薄层、**无 store**:选定的 key id 存在 workspace 行上
// （与默认模型选择并列），由 workspace.Service 实现 SearchKeyPicker。WebSearch 工具（tool/web）
// 依赖本包拿 provider 常量与 picker;各 provider 的 HTTP 调用在工具里、不在此处。
//
// 取名 "websearch"（非 "search"）因为 tool/search 是无关的文件系统搜索（Glob/Grep/LS）。这里是网络搜索。
package websearch

import "context"

// Search providers Forgify can route a WebSearch query to. The provider of a
// chosen key is implied by the api-key itself (apikey.Credentials.Provider) —
// these constants let the WebSearch tool switch on it without hardcoding strings.
//
// Forgify 可把 WebSearch 查询路由到的搜索 provider。选定 key 的 provider 由 api-key 自身隐含
// （apikey.Credentials.Provider）——这些常量让 WebSearch 工具据以分派、不必硬编码字符串。
const (
	ProviderBrave  = "brave"
	ProviderSerper = "serper"
	ProviderTavily = "tavily"
	ProviderBocha  = "bocha"
)

// IsProvider reports whether p is a recognised search provider. The WebSearch
// tool uses it to reject a key whose provider is not a search backend (e.g. the
// user pointed default-search at an LLM key) with a clear tool-result.
//
// IsProvider 报告 p 是否为已知搜索 provider。WebSearch 工具用它拒绝 provider 非搜索后端的
// key（如用户把 default-search 指向了 LLM key），返清晰 tool-result。
func IsProvider(p string) bool {
	switch p {
	case ProviderBrave, ProviderSerper, ProviderTavily, ProviderBocha:
		return true
	default:
		return false
	}
}

// Providers returns every search provider in canonical order — for UI listing
// and docs; selection is a single explicit choice, not a priority walk.
//
// Providers 按规范顺序返回所有搜索 provider——供 UI 列举与文档;选择是单一显式选定，非优先级遍历。
func Providers() []string {
	return []string{ProviderBrave, ProviderSerper, ProviderTavily, ProviderBocha}
}

// SearchKeyPicker is the DIP port the WebSearch tool depends on: it reports the
// api-key id the current workspace (taken from ctx) has chosen for search, with
// ok=false when none is configured. Implemented by workspace.Service, mirroring
// how model.ModelPicker is implemented there. A single explicit choice — no
// priority list — so the agent never burns credits probing providers in turn.
//
// SearchKeyPicker 是 WebSearch 工具依赖的 DIP 端口:报告当前 workspace（取自 ctx）为搜索选定的
// api-key id，未配置时 ok=false。由 workspace.Service 实现，镜像 model.ModelPicker 在那里的实现。
// 单一显式选择、无优先级列表——agent 永不挨个试 provider 乱烧钱。
type SearchKeyPicker interface {
	DefaultSearchKeyID(ctx context.Context) (string, bool)
}
