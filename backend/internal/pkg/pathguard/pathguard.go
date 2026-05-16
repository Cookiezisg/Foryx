// Package pathguard is a deny-list path safety layer for filesystem tools.
//
// Package pathguard 是文件系统 tool 的路径黑名单守卫层。
package pathguard

import (
	"os"
	"path/filepath"
	"strings"
)

// PathGuard decides whether a tool may operate on absPath; reason surfaces
// in tool_result. AllowWrite extends Allow with extra write-only deny
// rules (e.g. .git/ — readable to inspect history, never writable from
// an AI tool). V1.2 §3 final-sweep added the write-side split so
// permissions can be tightened without breaking read tools.
//
// PathGuard 决定 tool 是否可操作 absPath；reason 进 tool_result。AllowWrite
// 在 Allow 基础上加额外写专属 deny 规则（如 .git/——可读看历史但 AI
// tool 永不该写）。V1.2 §3 加读写分离让写收紧不破坏读。
type PathGuard interface {
	Allow(absPath string) (allowed bool, reason string)
	AllowWrite(absPath string) (allowed bool, reason string)
}

// DefaultDenyList: trailing "/" = directory prefix; no slash = exact file; "~/" expands at New().
//
// DefaultDenyList：结尾 "/" = 目录前缀；无 "/" = 精确文件；"~/" 在 New() 时展开。
var DefaultDenyList = []string{
	"/etc/", "/usr/", "/sys/", "/bin/", "/sbin/",
	"/private/etc/", "/private/var/", "/System/", "/Library/Keychains/",

	"/proc/", "/run/secrets/", "/var/run/secrets/", "/sys/class/",

	"C:/Windows/", "C:/ProgramData/Microsoft/Crypto/",
	"~/AppData/Roaming/Microsoft/Credentials/",
	"~/AppData/Local/Microsoft/Credentials/",
	"~/AppData/Roaming/Microsoft/Crypto/",
	"~/AppData/Roaming/Microsoft/Protect/",
	"~/AppData/Local/Microsoft/Vault/",

	"~/.ssh/", "~/.aws/", "~/.gnupg/", "~/.netrc", "~/.config/git-credentials",
	"~/.docker/config.json", "~/.kube/config",

	"~/Library/Application Support/Google/Chrome/Default/Login Data",
	"~/.config/google-chrome/Default/Login Data",
	"~/AppData/Local/Google/Chrome/User Data/Default/Login Data",
	"~/AppData/Local/Microsoft/Edge/User Data/Default/Login Data",

	"~/.forgify/",
}

// DefaultWriteOnlyExtras lists paths the AI may READ but never WRITE
// (V1.2 §3 final-sweep). Patterns use the same syntax as DefaultDenyList
// — trailing "/" = directory prefix. WriteOnly = additional layer on
// top of DefaultDenyList; full write deny set = DefaultDenyList ∪ Extras.
//
// DefaultWriteOnlyExtras 列 AI 可读不可写的路径（V1.2 §3）。语法同
// DefaultDenyList。完整写 deny = DefaultDenyList ∪ Extras。
var DefaultWriteOnlyExtras = []string{
	// VCS — AI must never rewrite git history / hooks / refs.
	// VCS——AI 永远不该改写 git 历史 / hooks / refs。
	".git/",

	// Env / secrets — readable to debug, never writable (might overwrite
	// real secrets with placeholder strings).
	// env / 秘密——可读供调试，绝不能写（防覆盖真实 secret 为占位）。
	".env",
	".env.local",
	".env.production",
	".envrc",

	// Package manager output — should be regenerated from package.json
	// / pyproject.toml, never hand-written.
	// 包管理输出——应从 package.json / pyproject.toml 重建，绝不手写。
	"node_modules/",
	".venv/",
	"venv/",
	"__pycache__/",
}

type rule struct {
	path  string
	isDir bool
}

type defaultGuard struct {
	// rules guards both Read and Write. writeOnlyRules apply additionally
	// to Write only — they're the "VCS / env / cache" extras that allow
	// inspection but block mutation.
	// rules 守读 + 写。writeOnlyRules 仅写额外生效——VCS / env / 缓存类
	// 允许查看但禁修改。
	rules          []rule
	writeOnlyRules []rule
}

// New returns a PathGuard denying paths matching denyList; non-absolute
// rules silently dropped. AllowWrite uses the same denyList — pass
// NewWithWriteExtras for a separate write-only deny set.
//
// New 返拒绝 denyList 的 PathGuard；非绝对路径规则静默丢弃。
// AllowWrite 用同一 denyList——分离写 deny 集走 NewWithWriteExtras。
func New(denyList []string) PathGuard {
	return NewWithWriteExtras(denyList, nil)
}

// NewWithWriteExtras returns a PathGuard with separate read + write
// deny lists. Write deny set = denyList ∪ writeOnlyExtras. Both lists
// follow the same trailing-"/" + "~/" expansion conventions.
//
// NewWithWriteExtras 返带分离读 + 写 deny 列表的 PathGuard。
// 写 deny 集 = denyList ∪ writeOnlyExtras。两列表共用同语法。
func NewWithWriteExtras(denyList, writeOnlyExtras []string) PathGuard {
	return &defaultGuard{
		rules:          parseRules(denyList),
		writeOnlyRules: parseRules(writeOnlyExtras),
	}
}

// NewDefault returns a PathGuard configured with DefaultDenyList +
// DefaultWriteOnlyExtras (V1.2 §3 final-sweep).
//
// NewDefault 返用 DefaultDenyList + DefaultWriteOnlyExtras 配置的 PathGuard。
func NewDefault() PathGuard {
	return NewWithWriteExtras(DefaultDenyList, DefaultWriteOnlyExtras)
}

func parseRules(raw []string) []rule {
	home, _ := os.UserHomeDir()
	out := make([]rule, 0, len(raw))
	for _, p := range raw {
		isDir := strings.HasSuffix(p, string(filepath.Separator)) || strings.HasSuffix(p, "/")
		expanded := p
		if strings.HasPrefix(expanded, "~/") {
			if home == "" {
				continue
			}
			expanded = filepath.Join(home, expanded[2:])
		}
		// Relative patterns (e.g. ".git/", "node_modules/") are matched
		// against the basename + ancestor chain via matchPath below;
		// absolute patterns continue to need filepath.IsAbs.
		// 相对模式（如 ".git/"）经下方 matchPath 匹配 basename + 祖先链；
		// 绝对模式仍需 filepath.IsAbs。
		isRelative := !filepath.IsAbs(expanded)
		out = append(out, rule{
			path:  filepath.Clean(expanded),
			isDir: isDir,
		})
		_ = isRelative // future field if we add anchored matching
	}
	return out
}

// Allow checks absPath against read+write deny rules. Write-only extras
// are NOT applied (use AllowWrite for those).
//
// Allow 按读 + 写 deny 规则检查 absPath。写专属 extras 不在此查（用 AllowWrite）。
func (g *defaultGuard) Allow(absPath string) (bool, string) {
	return checkRules(absPath, g.rules)
}

// AllowWrite checks absPath against the union of read+write deny rules
// and the write-only extras (.git/, .env, node_modules/ etc).
//
// AllowWrite 按读 + 写 deny + 写专属 extras 的并集检查 absPath。
func (g *defaultGuard) AllowWrite(absPath string) (bool, string) {
	if ok, reason := checkRules(absPath, g.rules); !ok {
		return false, reason
	}
	return checkRules(absPath, g.writeOnlyRules)
}

func checkRules(absPath string, rules []rule) (bool, string) {
	if !filepath.IsAbs(absPath) {
		return false, "path must be absolute: " + absPath
	}
	cleaned := filepath.Clean(absPath)
	for _, r := range rules {
		if filepath.IsAbs(r.path) {
			// Absolute rule — anchored exact / prefix match.
			// 绝对规则——锚定精确 / 前缀匹配。
			if r.isDir {
				if cleaned == r.path || strings.HasPrefix(cleaned, r.path+string(filepath.Separator)) {
					return false, "path is denied by safety guard: " + r.path
				}
			} else if cleaned == r.path {
				return false, "path is denied by safety guard: " + r.path
			}
			continue
		}
		// Relative rule (e.g. ".git/", ".env", "node_modules/") — match
		// any path segment in the cleaned path. .git/ matches both
		// "/proj/.git/HEAD" (segment) and "/proj/.git" (exact suffix).
		// 相对规则——匹配 cleaned 路径任一段。.git/ 匹配 "/proj/.git/HEAD"
		// （段）和 "/proj/.git"（精确后缀）。
		if r.isDir {
			seg := string(filepath.Separator) + r.path + string(filepath.Separator)
			if strings.Contains(cleaned, seg) ||
				strings.HasSuffix(cleaned, string(filepath.Separator)+r.path) {
				return false, "path is denied by safety guard: " + r.path
			}
		} else {
			// File-name match: basename equals the rule.
			// 文件名匹配：basename 等于规则。
			if filepath.Base(cleaned) == r.path {
				return false, "path is denied by safety guard: " + r.path
			}
		}
	}
	return true, ""
}
