// activate.go — Service.Activate (the L2 path: load body, substitute
// placeholders, set ActiveSkill on agentstate, dispatch to subagent if
// fork mode). Substitution helpers for $1/$ARGUMENTS/${CLAUDE_*} and
// the depth-guard that prevents nested-fork (per skill.md §9.5).
//
// activate.go ——Service.Activate（L2：加载 body、替换占位、给 agentstate
// 设 ActiveSkill、fork 模式派发 subagent）。$1/$ARGUMENTS/${CLAUDE_*}
// 替换 helper + 防嵌套 fork 的 depth guard（§9.5）。
package skill

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// bodyReadRetryDelay is the §9.5 brief retry window before giving up on a
// body re-read. Editors that use "write .tmp + rename" may show a
// momentarily-missing file; one 100ms retry covers it.
//
// bodyReadRetryDelay 是 §9.5 短暂重试窗口。"write .tmp + rename" 的编辑器
// 会有瞬时不存在；100ms 一次重试覆盖。
const bodyReadRetryDelay = 100 * time.Millisecond

// Activate loads the skill's body, substitutes placeholders, sets
// ActiveSkill on the conversation's AgentState (so subsequent tool
// dispatches can short-circuit permission via the allowed-tools list),
// and either returns the substituted body verbatim (default) OR spawns
// a subagent with the body as prompt (frontmatter.context=="fork").
//
// Per §9.5: when running inside an existing subagent (depth ≥ 1) the
// fork directive is ignored — the subagent context is already isolated;
// re-forking would just waste budget and violate the depth-1 invariant
// the recursion-defense relies on.
//
// Activate 加载 skill 的 body、替换占位、给对话的 AgentState 设
// ActiveSkill（让后续 tool dispatch 经 allowed-tools 短路 permission），
// 默认返回替换后的 body 字符串；frontmatter.context=="fork" 则用 body 作
// prompt spawn subagent 并返回其 last message。§9.5：已在 subagent 里
// （depth ≥ 1）忽略 fork 指令——已隔离再 fork 是浪费 + 破坏 depth-1
// 不变量。
func (s *Service) Activate(ctx context.Context, name string, arguments []string) (string, error) {
	// Lookup metadata (cheap; lock held briefly).
	// 查元数据（廉价；锁短持）。
	s.mu.RLock()
	skill, ok := s.skills[name]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("skillapp.Activate: %w: %q", skilldomain.ErrSkillNotFound, name)
	}

	body, err := readBodyWithRetry(skill.BodyPath)
	if err != nil {
		return "", fmt.Errorf("skillapp.Activate %s: %w", name, err)
	}
	if len(body) > skilldomain.MaxBodyBytes {
		// Body grew past cap between Scan time and now (user edit). Treat
		// as the same hard error Scan would have raised.
		// body 在 Scan 后超 cap（用户编辑）。同 Scan 的硬错。
		return "", fmt.Errorf("skillapp.Activate %s: %w", name, skilldomain.ErrBodyTooLarge)
	}

	convID, _ := reqctxpkg.GetConversationID(ctx)
	substituted := substitute(string(body), substituteVars{
		Arguments:    arguments,
		NamedArgs:    skill.Frontmatter.Arguments,
		SkillDir:     skill.DirPath,
		SessionID:    convID,
		Effort:       skill.Frontmatter.Effort,
	})

	// Set ActiveSkill on the per-conversation AgentState. Tool dispatch
	// queries this on every CheckPermissions to honor allowed-tools.
	// Defer-clear is intentionally *not* done here for non-fork — we want
	// the pre-approval to persist for the LLM's follow-up tool calls in
	// the same conversation (per skill.md §9.4 "non-fork: don't clear").
	// 给 per-conversation AgentState 设 ActiveSkill。tool dispatch 每次
	// CheckPermissions 查询。非 fork 故意不 defer-clear——让预授权在 LLM
	// 后续同对话调用持续（§9.4 "non-fork: don't clear"）。
	if state, hasState := reqctxpkg.GetAgentState(ctx); hasState {
		state.SetActiveSkill(skill)
	}

	// Fork mode: dispatch to SubagentService. Depth guard per §9.5.
	// fork 模式：派发 SubagentService。depth guard §9.5。
	if skill.Frontmatter.Context == "fork" {
		if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 {
			s.log.Info("skill activated within subagent; ignoring fork directive",
				zap.String("skill", name), zap.Int("depth", depth))
			return substituted, nil
		}
		if s.subagent == nil {
			return "", fmt.Errorf("skillapp.Activate %s: fork requested but SubagentService is nil", name)
		}
		agentType := skill.Frontmatter.Agent // validated non-empty at Scan time
		result, err := s.subagent.Spawn(ctx, agentType, substituted, subagentapp.SpawnOpts{})
		if err != nil {
			return "", fmt.Errorf("skillapp.Activate %s: subagent spawn: %w", name, err)
		}
		return result.Result, nil
	}

	return substituted, nil
}

// readBodyWithRetry reads the SKILL.md body, retrying once after 100ms
// on ErrNotExist (covers editor "write .tmp + rename" race per §9.5).
// All other errors propagate immediately.
//
// readBodyWithRetry 读 SKILL.md body，ErrNotExist 100ms 后重试一次（覆盖
// 编辑器 "write .tmp + rename" 竞态 §9.5）。其他错立即上抛。
func readBodyWithRetry(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err == nil {
		return body, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	time.Sleep(bodyReadRetryDelay)
	return os.ReadFile(path)
}

// ── substitution ─────────────────────────────────────────────────────

// substituteVars bundles every placeholder source into one struct so the
// substitute fn signature stays stable as we add more placeholders later.
//
// substituteVars 把每个占位源打包，让 substitute 签名在加新占位时稳定。
type substituteVars struct {
	Arguments []string // positional ($1, $2, ..., $ARGUMENTS join)
	NamedArgs []string // names from frontmatter.arguments (matched to Arguments by index)
	SkillDir  string   // ${CLAUDE_SKILL_DIR}
	SessionID string   // ${CLAUDE_SESSION_ID}
	Effort    string   // ${CLAUDE_EFFORT} (V1 plumbed but not consumed)
}

// substitute applies all V1 placeholder forms. Replacement order matters:
// longer prefixes first (${CLAUDE_*} before $1 to avoid $1 in the middle
// of ${CLAUDE_SESSION_ID} matching). Strings.Replacer handles overlapping
// rules deterministically (longest-prefix wins per the standard library
// docs).
//
// substitute 应用所有 V1 占位形式。替换顺序：长 prefix 优先（${CLAUDE_*}
// 在 $1 之前，避免 ${CLAUDE_SESSION_ID} 中间的 $1 误匹配）。
// strings.Replacer 文档保证 longest-prefix wins。
func substitute(body string, v substituteVars) string {
	pairs := []string{
		"${CLAUDE_SKILL_DIR}", v.SkillDir,
		"${CLAUDE_SESSION_ID}", v.SessionID,
		"${CLAUDE_EFFORT}", v.Effort,
		"$ARGUMENTS", strings.Join(v.Arguments, " "),
	}
	// $1..$N positional placeholders. Walk highest-N first so $10 doesn't
	// get pre-empted by $1's match (Replacer is longest-key-wins, but we
	// want defensive ordering for clarity).
	// $1..$N 位置占位。从高到低走防 $10 被 $1 抢匹配（Replacer 长 key 胜，
	// 但显式排序更清晰）。
	for i := len(v.Arguments); i >= 1; i-- {
		pairs = append(pairs, "$"+strconv.Itoa(i), v.Arguments[i-1])
	}
	// Named placeholders ($pr_number etc.) per frontmatter.arguments
	// declaration. Indexed by position in the NamedArgs slice → matched
	// to Arguments[i].
	// 命名占位（$pr_number 等）按 frontmatter.arguments 声明。NamedArgs[i]
	// → Arguments[i]。
	for i, name := range v.NamedArgs {
		if i >= len(v.Arguments) {
			break
		}
		if name == "" {
			continue
		}
		pairs = append(pairs, "$"+name, v.Arguments[i])
	}
	return strings.NewReplacer(pairs...).Replace(body)
}
