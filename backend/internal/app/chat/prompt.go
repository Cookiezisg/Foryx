package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	conversationdomain "github.com/sunweilin/anselm/backend/internal/domain/conversation"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// System prompt static sections: high-density, no product fluff, no safety theater (local
// single-user) — and deliberately not boxing the agent in. Cache order is stable: invariant
// static blocks first (identity → how_to_work → tools), dynamic context in the middle, and the
// two rule blocks last because end-of-prompt instructions get the highest adherence.
//
// System prompt 静态段：高密度、无产品 fluff、无 safety theater（本地单用户）——且刻意不框死
// agent。缓存顺序稳定：不变静态块在前（identity → how_to_work → tools），动态上下文居中，两个
// 规则块殿后，因末尾指令遵从度最高。
const (
	identitySection = `You are Anselm, a local-first agentic assistant running on the user's own machine. ` +
		`You operate over their whole computer (absolute paths, no project root) and a workspace of built capabilities — ` +
		`functions, handlers, agents, and workflows the user builds and you can call, create, and refine.`

	howToWorkSection = `Reuse before you build: search the existing library before building anything new. ` +
		`Verify before you claim — run it, read the result, then report. ` +
		`Prefer the smallest change that works; say what you actually did, not what you intended. ` +
		`When a step fails, surface the real error rather than papering over it.`

	toolsSection = `Resident tools are always available. Other tools are listed below as "name(required args): purpose" — ` +
		`call search_tools with a short description of what you need to pull a tool's full definition before using it. ` +
		`An arg name ending in Id wants that entity's id (an fn_/hd_/wf_/ag_/tr_… id), not its name — use the matching search_* tool to resolve a name → id first. ` +
		`Each tool call self-reports a one-line summary and a danger level; you choose the right tool for the job.`

	architectureRulesSection = `Capabilities are entities: anything reusable belongs to a function (stateless logic), a handler ` +
		`(stateful service), an agent (a configured LLM worker), or a workflow (a durable orchestration graph). ` +
		`Reach for an agent node when a step needs judgment; a function when it's deterministic.`

	criticalRulesSection = `Do not fabricate results or tool output. ` +
		`If you cannot complete the request with the tools you have, say so plainly instead of pretending. ` +
		`Keep responses concise.`
)

// buildSystemPrompt assembles the turn's system prompt from static sections + live context
// (capabilities / memory / documents / the user's own system prompt / environment). Each
// non-empty section is wrapped in <section name="..."> so the model can tell them apart. A nil
// optional provider simply contributes nothing.
//
// buildSystemPrompt 从静态段 + live 上下文（capabilities / memory / documents / 用户自己的 system
// prompt / environment）组装回合 system prompt。每个非空段用 <section name="..."> 包裹，使模型能
// 区分。可选 provider 为 nil 时该段不贡献内容。
func (s *Service) buildSystemPrompt(ctx context.Context, conv *conversationdomain.Conversation) string {
	type section struct{ name, content string }
	sections := []section{
		{"identity", identitySection},
		{"how_to_work", howToWorkSection},
		{"tools", s.toolsOverview()},
	}
	if s.deps.Catalog != nil {
		sections = append(sections, section{"capabilities", s.deps.Catalog.GetForSystemPrompt(ctx)})
	}
	if s.deps.Memory != nil {
		sections = append(sections, section{"memory", s.deps.Memory.ForSystemPrompt(ctx)})
	}
	if s.deps.Documents != nil {
		if docs, err := s.deps.Documents.RenderAttached(ctx, conv.AttachedDocuments); err == nil {
			sections = append(sections, section{"documents", docs})
		}
	}
	sections = append(sections,
		section{"user_system_prompt", conv.SystemPrompt},
		section{"environment", environmentSection(ctx)},
		section{"architecture_rules", architectureRulesSection},
		section{"critical_rules", criticalRulesSection},
	)

	var b strings.Builder
	for _, sec := range sections {
		if strings.TrimSpace(sec.content) == "" {
			continue
		}
		fmt.Fprintf(&b, "<section name=%q>\n%s\n</section>\n\n", sec.name, sec.content)
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolsOverview renders the static tools guidance + the lazy-tool catalog (name: one-line
// description) so the LLM knows the full inventory and never blind-searches. Resident tools' full
// defs are already in the request; only the lazy overview needs surfacing here.
//
// toolsOverview 渲染静态工具指引 + lazy 工具目录（name: 一句话 description），使 LLM 知道全集、永不
// 盲搜。Resident 工具完整定义已在 request；此处只需浮出 lazy 概览。
func (s *Service) toolsOverview() string {
	overview := s.deps.Toolset.Overview()
	if len(overview) == 0 {
		return toolsSection
	}
	var b strings.Builder
	b.WriteString(toolsSection)
	b.WriteString("\n\nSearchable tools:")
	for _, t := range overview {
		if len(t.Params) > 0 {
			fmt.Fprintf(&b, "\n  - %s(%s): %s", t.Name, strings.Join(t.Params, ", "), t.Description)
		} else {
			fmt.Fprintf(&b, "\n  - %s: %s", t.Name, t.Description)
		}
	}
	return b.String()
}

// environmentSection states today's date and the user's reply language, so the model anchors time
// references and answers in the workspace's language.
//
// environmentSection 给出今天日期与用户回复语言，使模型锚定时间引用并以工作区语言作答。
func environmentSection(ctx context.Context) string {
	lang := "English"
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		lang = "Chinese"
	}
	return fmt.Sprintf("Today's date: %s.\nReply in %s.", time.Now().UTC().Format("2006-01-02"), lang)
}
