// gate.go — the Gate runtime: Evaluate() applies deny → ask → allow →
// defaultMode rules; also wires in the session ask-once cache so a
// "user said yes once" doesn't re-prompt every turn in the same chat.
// PreToolUse / PostToolUse hook calls live in hooks_bridge.go (next).
//
// gate.go ——Gate 运行时：Evaluate() 按 deny → ask → allow → defaultMode
// 评估规则；session ask-once 缓存让"用户答过一次 yes"不每轮再提示。
// PreToolUse / PostToolUse hook 在 hooks_bridge.go。
package permissionsgate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"

	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

// RulesProvider returns the current rules snapshot. Implemented by
// infra/settings.Service so the gate stays infra-agnostic.
//
// RulesProvider 返当前规则快照。infra/settings.Service 实现让 gate
// 与 infra 解耦。
type RulesProvider interface {
	GetRules() *permdomain.Settings
}

// Gate orchestrates rule evaluation + session ask cache. Stateless
// w.r.t. configuration (reads via RulesProvider); session cache lives
// in askCache map keyed by sessionID.
//
// Gate 编排规则评估 + session ask 缓存。配置无状态（经 RulesProvider 读）；
// session 缓存在 askCache map 按 sessionID 键存。
type Gate struct {
	rules RulesProvider

	mu       sync.RWMutex
	askCache map[string]map[string]struct{} // sessionID → set of (toolName + argsHash)
}

// New constructs a Gate backed by the given RulesProvider.
//
// New 用给定 RulesProvider 构造 Gate。
func New(rules RulesProvider) *Gate {
	return &Gate{
		rules:    rules,
		askCache: map[string]map[string]struct{}{},
	}
}

// Evaluate decides what to do with a tool call. SessionID scopes the
// ask-once cache (caller passes conversationID or a stable session
// identifier). Returns the Decision; caller (chat/tools.go::runTools)
// honors ActionAllow / asks user via AskUserQuestion on ActionAsk /
// emits a deny tool_result on ActionDeny.
//
// Evaluate 决定 tool call 怎么处理。SessionID 框 ask-once 缓存（caller
// 传 conversationID 或稳定 session 标识）。返 Decision；caller
// (chat/tools.go::runTools) 按 Action 行事：Allow 透过 / Ask 经
// AskUserQuestion 问 / Deny 发拒绝 tool_result。
func (g *Gate) Evaluate(sessionID, toolName string, args json.RawMessage, destructive bool) permdomain.Decision {
	settings := g.rules.GetRules()
	if settings == nil {
		// Shouldn't happen — settings.New() always seeds empty defaults —
		// but if it does, fail safe by asking.
		// 不该发生——settings.New() 总播种空默认——若发生，安全 fallback 是问。
		return permdomain.Decision{Action: permdomain.ActionAsk, Reason: "no settings loaded"}
	}

	// Bypass mode skips rule evaluation entirely (protectedPaths still
	// guards write; that's enforced by pathguard separately).
	// Bypass 模式完全跳规则评估（protectedPaths 仍守写，pathguard 独立强制）。
	if settings.EffectiveDefaultMode() == permdomain.DefaultModeBypass {
		return permdomain.Decision{Action: permdomain.ActionAllow, Reason: "defaultMode=bypass"}
	}

	// 1. Deny rules — first match wins, never overridable.
	// 1. Deny 规则——第一匹配赢，永不可推翻。
	for _, pat := range settings.Permissions.Deny {
		if MatchesRule(pat, toolName, args) {
			return permdomain.Decision{Action: permdomain.ActionDeny, Reason: "matched deny rule: " + pat}
		}
	}

	// 2. Session ask-once cache: if the user already said yes to this
	//    exact (tool, args) in this session, skip the ask.
	// 2. Session ask-once 缓存：本 session 内对同一 (tool, args) 用户
	//    已答 yes，跳过 ask。
	cacheKey := makeCacheKey(toolName, args)
	if g.checkAskCache(sessionID, cacheKey) {
		return permdomain.Decision{Action: permdomain.ActionAllow, Reason: "session-cached approval"}
	}

	// 3. Ask rules — first match.
	// 3. Ask 规则——第一匹配。
	for _, pat := range settings.Permissions.Ask {
		if MatchesRule(pat, toolName, args) {
			return permdomain.Decision{Action: permdomain.ActionAsk, Reason: "matched ask rule: " + pat}
		}
	}

	// 4. Allow rules — first match.
	// 4. Allow 规则——第一匹配。
	for _, pat := range settings.Permissions.Allow {
		if MatchesRule(pat, toolName, args) {
			return permdomain.Decision{Action: permdomain.ActionAllow, Reason: "matched allow rule: " + pat}
		}
	}

	// 5. destructive=true: LLM self-reported risky → force ask even when
	//    no rule matches (the LLM admitting destructive is itself signal).
	//    Bypass mode short-circuited above; safe modes here promote ask.
	// 5. destructive=true：LLM 自报危险 → 即使无规则也强制 ask（LLM 自承
	//    destructive 本身是信号）。bypass 上面短路；安全模式提升为 ask。
	if destructive {
		return permdomain.Decision{Action: permdomain.ActionAsk, Reason: "tool self-declared destructive=true"}
	}

	// 6. Fall through to defaultMode.
	// 6. 走 defaultMode。
	switch settings.EffectiveDefaultMode() {
	case permdomain.DefaultModeAllow:
		return permdomain.Decision{Action: permdomain.ActionAllow, Reason: "defaultMode=allow"}
	case permdomain.DefaultModeDeny:
		return permdomain.Decision{Action: permdomain.ActionDeny, Reason: "defaultMode=deny"}
	default: // ask
		// ReadOnly tools never ask under defaultMode=ask — that would
		// drown the user in prompts for harmless reads.
		// 默认 ask 时只读 tool 不问——否则用户被无害读淹没。
		if lvl, ok := toolLevels[toolName]; ok && lvl == permdomain.LevelReadOnly {
			return permdomain.Decision{Action: permdomain.ActionAllow, Reason: "read-only tool"}
		}
		return permdomain.Decision{Action: permdomain.ActionAsk, Reason: "defaultMode=ask"}
	}
}

// RememberApproval caches a yes-answer so subsequent same-(tool,args)
// calls in the same session skip the prompt. Called by chat/tools.go
// after the user confirms via AskUserQuestion.
//
// RememberApproval 缓存 yes 答案，同 session 内同 (tool,args) 后续跳 ask。
// chat/tools.go 在用户经 AskUserQuestion 确认后调。
func (g *Gate) RememberApproval(sessionID, toolName string, args json.RawMessage) {
	key := makeCacheKey(toolName, args)
	g.mu.Lock()
	defer g.mu.Unlock()
	set, ok := g.askCache[sessionID]
	if !ok {
		set = make(map[string]struct{})
		g.askCache[sessionID] = set
	}
	set[key] = struct{}{}
}

// ForgetSession drops all cached approvals for sessionID. Called when
// a conversation ends so memory doesn't grow unbounded across sessions.
//
// ForgetSession 丢 sessionID 的全部缓存。对话结束时调，防内存无界增长。
func (g *Gate) ForgetSession(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.askCache, sessionID)
}

func (g *Gate) checkAskCache(sessionID, key string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	set, ok := g.askCache[sessionID]
	if !ok {
		return false
	}
	_, hit := set[key]
	return hit
}

// makeCacheKey hashes (toolName, normalisedArgs) so different arg
// orderings still hit the same entry. Hash collisions are not a
// security risk — worst case is a free pass on a different call that
// happens to hash the same; sha256 makes this astronomically unlikely.
//
// makeCacheKey 哈希 (toolName, 归一化 args)，args 顺序不同也命中同条目。
// 哈希冲突非安全风险——最差是另一个碰巧同 hash 的调用获通过；sha256
// 让这事概率天文级别低。
func makeCacheKey(toolName string, args json.RawMessage) string {
	normalised := canonicalizeArgs(args)
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0})
	h.Write([]byte(normalised))
	return toolName + ":" + hex.EncodeToString(h.Sum(nil)[:16])
}

// canonicalizeArgs re-encodes args via map[string]any so key order is
// alphabetical (Go map iteration is randomised but json.Marshal of map
// sorts keys). Returns "" on parse failure (cache key fallback to bare
// tool name).
//
// canonicalizeArgs 经 map[string]any 重编 args，让 key 字母序（Go map
// 迭代随机但 json.Marshal of map 按 key 排序）。解析失败返 ""（cache
// key 退到裸 tool 名）。
func canonicalizeArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return strings.TrimSpace(string(args))
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(raw)
}
