// Package hooks runs user-configured shell hooks at 3 lifecycle points
// (V1.2 §3 final-sweep): PreToolUse / PostToolUse / Stop. Hook config
// lives in ~/.forgify/settings.json (hooks block); the Runner spawns
// the shell command per fire with JSON on stdin and parses JSON on
// stdout, honoring exit-2 as a blocking signal.
//
// Package hooks 跑用户配置的 shell hook，3 个生命周期点（V1.2 §3）：
// PreToolUse / PostToolUse / Stop。hook 配置在 ~/.forgify/settings.json
// （hooks 块）；Runner 每次触发 spawn shell 命令，stdin 喂 JSON，stdout
// 解析 JSON，exit=2 视为 blocking 信号。
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

// SettingsProvider returns the latest Settings snapshot — used to read
// the hooks block. Same shape as gate's RulesProvider; infra/settings
// implements it so this pkg stays infra-agnostic.
//
// SettingsProvider 返最新 Settings 快照——读 hooks 块。同 gate 的
// RulesProvider 形状；infra/settings 实现让本包 infra 无关。
type SettingsProvider interface {
	GetRules() *permdomain.Settings
}

// Runner is the public entry point. Spawn one per Service; reuse across
// all conversations. No persistent state — each fire is independent.
//
// Runner 是对外入口。每个 Service spawn 一个；跨所有 conversation 复用。
// 无持久状态——每次 fire 独立。
type Runner struct {
	settings SettingsProvider
	log      *zap.Logger
}

// New constructs a Runner. log may be nil → zap.Nop.
//
// New 构造 Runner。log 可 nil → zap.Nop。
func New(settings SettingsProvider, log *zap.Logger) *Runner {
	if log == nil {
		log = zap.NewNop()
	}
	return &Runner{settings: settings, log: log.Named("hooks")}
}

// FirePreToolUse runs all matching PreToolUse hooks for the call.
// Returns the first Decision != "" emitted by any hook, or empty
// Decision when none short-circuited. Caller (chat/tools.go) honors
// the decision (deny → cancel tool dispatch; ask → AskUserQuestion).
//
// FirePreToolUse 跑所有匹配 PreToolUse hook。返第一个非空 Decision；都
// 不短路返空 Decision。caller (chat/tools.go) 按 decision 行事。
func (r *Runner) FirePreToolUse(ctx context.Context, in permdomain.HookInput) permdomain.Decision {
	in.HookEventName = "PreToolUse"
	hooks := r.matchingHooks(eventPre, in.ToolName, in.ToolInput)
	for _, h := range hooks {
		out := r.runOne(ctx, h, in)
		if out.Decision != "" {
			return permdomain.Decision{Action: out.Decision, Reason: out.Reason}
		}
	}
	return permdomain.Decision{}
}

// FirePostToolUse runs all matching PostToolUse hooks. Returns aggregated
// text to inject into the next LLM turn (concatenated InjectIntoNextTurn
// from all hooks). PostToolUse cannot block — decision fields are ignored.
//
// FirePostToolUse 跑所有匹配 PostToolUse hook。返要注入下轮 LLM context
// 的聚合文本（所有 hook 的 InjectIntoNextTurn 拼）。PostToolUse 不能阻断
// ——decision 字段忽略。
func (r *Runner) FirePostToolUse(ctx context.Context, in permdomain.HookInput) string {
	in.HookEventName = "PostToolUse"
	hooks := r.matchingHooks(eventPost, in.ToolName, in.ToolInput)
	var sb strings.Builder
	for _, h := range hooks {
		out := r.runOne(ctx, h, in)
		if out.InjectIntoNextTurn != "" {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(out.InjectIntoNextTurn)
		}
	}
	return sb.String()
}

// FireStop runs all Stop hooks. Returns true if any hook decided to
// continue (Decision="continue" via Reason field convention), causing
// chat runner to fire a follow-up agent turn. False = let stop proceed.
// Reason is the aggregated continue prompt.
//
// FireStop 跑所有 Stop hook。任一 hook 决定继续（Decision="continue"，
// Reason 字段载继续 prompt）返 true，让 chat runner 再跑一轮。false =
// 放行 stop。Reason 是聚合继续 prompt。
func (r *Runner) FireStop(ctx context.Context, in permdomain.HookInput) (cont bool, prompt string) {
	in.HookEventName = "Stop"
	hooks := r.matchingHooks(eventStop, "", nil)
	var sb strings.Builder
	for _, h := range hooks {
		out := r.runOne(ctx, h, in)
		if out.Decision == "continue" {
			cont = true
			if out.Reason != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(out.Reason)
			}
		}
	}
	return cont, sb.String()
}

type eventKind int

const (
	eventPre eventKind = iota
	eventPost
	eventStop
)

// matchingHooks returns hooks whose matcher + If filter both pass.
// Stop hooks ignore matcher/If (they always fire when present).
//
// matchingHooks 返 matcher + If filter 都过的 hook。Stop hook 忽略
// matcher/If（恒触发）。
func (r *Runner) matchingHooks(kind eventKind, toolName string, args json.RawMessage) []permdomain.HookSpec {
	settings := r.settings.GetRules()
	if settings == nil {
		return nil
	}
	var pool []permdomain.HookSpec
	switch kind {
	case eventPre:
		pool = settings.Hooks.PreToolUse
	case eventPost:
		pool = settings.Hooks.PostToolUse
	case eventStop:
		pool = settings.Hooks.Stop
	}
	if kind == eventStop {
		return pool
	}
	out := make([]permdomain.HookSpec, 0, len(pool))
	for _, h := range pool {
		if !matchesMatcher(h.Matcher, toolName) {
			continue
		}
		if h.If != "" && !MatchesRule(h.If, toolName, args) {
			continue
		}
		out = append(out, h)
	}
	return out
}

// matchesMatcher uses Go regexp (simpler subset than Bash glob) on the
// tool name. Empty matcher = match all. "Bash|Edit" is a valid regex.
//
// matchesMatcher 用 Go regexp（比 Bash glob 简单的子集）匹配 tool 名。
// 空 matcher = match all。"Bash|Edit" 是合法 regex。
func matchesMatcher(matcher, toolName string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	re, err := regexp.Compile("^(?:" + matcher + ")$")
	if err != nil {
		return false
	}
	return re.MatchString(toolName)
}

// MatchesRule is the bridge to permissionsgate's rule matcher — declared
// as a package-level variable so tests can stub. Default points at the
// real impl injected by gate's init().
//
// MatchesRule 桥到 permissionsgate 的规则匹配——包级变量让测试可 stub。
// 默认指向 gate init() 注入的真实实现。
var MatchesRule = func(rule string, toolName string, args json.RawMessage) bool {
	// Trivial bare-name fallback if gate hasn't registered the real impl.
	// 真实 impl 未注册时的裸名 fallback。
	verb := rule
	if i := strings.IndexByte(rule, '('); i >= 0 {
		verb = rule[:i]
	}
	return strings.TrimSpace(verb) == toolName
}

// runOne spawns + parses one hook invocation. Returns an empty
// HookOutput on any error (logged); short-circuit semantics are
// handled by the caller's per-hook loop.
//
// runOne spawn + 解析一次 hook 调用。任何错返空 HookOutput（log）；
// 短路语义由 caller per-hook 循环处理。
func (r *Runner) runOne(ctx context.Context, h permdomain.HookSpec, in permdomain.HookInput) permdomain.HookOutput {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(in)
	if err != nil {
		r.log.Warn("hook input marshal failed", zap.Error(err))
		return permdomain.HookOutput{}
	}

	cmd := exec.CommandContext(cctx, h.Command, h.Args...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else if cctx.Err() != nil {
			r.log.Warn("hook timeout",
				zap.String("command", h.Command),
				zap.Duration("timeout", timeout),
				zap.String("event", in.HookEventName))
			return permdomain.HookOutput{}
		} else {
			r.log.Warn("hook spawn failed",
				zap.String("command", h.Command),
				zap.Error(err))
			return permdomain.HookOutput{}
		}
	}

	switch exitCode {
	case 0:
		// Happy path — parse stdout.
		// 正常 — 解析 stdout。
		var out permdomain.HookOutput
		if stdout.Len() > 0 {
			if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
				r.log.Debug("hook stdout not JSON (treating as no-op)",
					zap.String("command", filepath.Base(h.Command)),
					zap.Error(err))
				return permdomain.HookOutput{}
			}
		}
		return out
	case 2:
		// Blocking error — synthesize a deny Decision with stderr as reason.
		// blocking 错——合成 deny Decision，stderr 作 reason。
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = "hook exited with code 2 (blocking)"
		}
		r.log.Info("hook blocked tool",
			zap.String("command", filepath.Base(h.Command)),
			zap.String("event", in.HookEventName),
			zap.String("reason", truncate(reason, 200)))
		return permdomain.HookOutput{
			Decision: permdomain.ActionDeny,
			Reason:   reason,
		}
	default:
		// Non-blocking error — log + continue.
		// 非 blocking 错 —— log + 继续。
		r.log.Warn("hook non-blocking failure",
			zap.String("command", filepath.Base(h.Command)),
			zap.Int("exitCode", exitCode),
			zap.String("stderr", truncate(stderr.String(), 200)))
		return permdomain.HookOutput{}
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Bridge between gate's MatchesRule and hooks pkg: gate's init.go calls
// SetMatchesRule(...) so MatchesRule above is wired to the real impl
// without an import cycle. Tests can also stub via this entry point.
//
// gate 与 hooks 间桥：gate init.go 调 SetMatchesRule(...) 把上面的
// MatchesRule 接到真实 impl，避免循环 import。测试也可经此 stub。
func SetMatchesRule(fn func(rule, toolName string, args json.RawMessage) bool) {
	if fn != nil {
		MatchesRule = fn
	}
}

var _ = fmt.Sprintf // tidy import if future fmt usage added
