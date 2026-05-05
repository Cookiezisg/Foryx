// Package pathguard is a deny-list path safety layer for filesystem tools
// (Read / Write / Edit / Glob / Grep). See documents/version-1.2/service-design-documents/02-tools-deep/03-shell.md (decision D5) for design + Bash exemption rationale.
//
// Package pathguard 是文件系统 tool（Read / Write / Edit / Glob / Grep）
// 的路径黑名单守卫层。设计 + Bash 豁免理由见 02-tools-deep/03-shell.md 决策 D5。
package pathguard

import (
	"os"
	"path/filepath"
	"strings"
)

// PathGuard decides whether a tool may operate on absPath.
// reason is the human-readable explanation surfaced in tool_result errors.
//
// PathGuard 决定 tool 是否可以操作 absPath。
// reason 是 allowed=false 时 tool_result 里的人话解释。
type PathGuard interface {
	Allow(absPath string) (allowed bool, reason string)
}

// DefaultDenyList covers TCC blind spots, cross-platform credential stores,
// system-critical paths, and Forgify's own data dir. Trailing "/" = directory
// prefix; no slash = exact file. "~/" expands at New() time. Rules whose
// expanded path is not absolute on the running OS get silently dropped — so
// the list can stay cross-platform without per-OS files.
//
// DefaultDenyList 覆盖 TCC 盲区、跨平台凭据存储、系统关键路径、Forgify 自身
// 数据目录。结尾 "/" = 目录前缀；无 "/" = 精确文件匹配。"~/" 在 New() 时展开。
// 展开后非绝对路径的规则静默丢弃——单文件即可跨平台共存。
var DefaultDenyList = []string{
	// macOS / Linux system-critical
	"/etc/", "/usr/", "/sys/", "/bin/", "/sbin/",
	"/private/etc/", "/private/var/", "/System/", "/Library/Keychains/",

	// Linux runtime + secrets (proc env / k8s / systemd-creds tokens)
	"/proc/", "/run/secrets/", "/var/run/secrets/", "/sys/class/",

	// Windows system + DPAPI / Credential Manager
	"C:/Windows/", "C:/ProgramData/Microsoft/Crypto/",
	"~/AppData/Roaming/Microsoft/Credentials/",
	"~/AppData/Local/Microsoft/Credentials/",
	"~/AppData/Roaming/Microsoft/Crypto/",
	"~/AppData/Roaming/Microsoft/Protect/",
	"~/AppData/Local/Microsoft/Vault/",

	// Unix user credentials (TCC blind spots)
	"~/.ssh/", "~/.aws/", "~/.gnupg/", "~/.netrc", "~/.config/git-credentials",
	"~/.docker/config.json", "~/.kube/config",

	// Browser saved logins — encrypted but locally decryptable
	"~/Library/Application Support/Google/Chrome/Default/Login Data",
	"~/.config/google-chrome/Default/Login Data",
	"~/AppData/Local/Google/Chrome/User Data/Default/Login Data",
	"~/AppData/Local/Microsoft/Edge/User Data/Default/Login Data",

	// Forgify's own state
	"~/.forgify/",
}

type rule struct {
	path  string // cleaned absolute path
	isDir bool   // directory prefix vs exact file
}

type defaultGuard struct {
	rules []rule
}

// New returns a PathGuard that denies paths matching denyList. Trailing "/"
// = directory prefix; "~/" expands against $HOME. Entries that fail to
// expand to an absolute path are silently dropped (fail-open is fine for a
// defense-in-depth layer; the design doc explains why).
//
// New 返回拒绝匹配 denyList 的 PathGuard。结尾 "/" = 目录前缀；
// "~/" 按 $HOME 展开。展开后非绝对路径静默丢弃——defense-in-depth 层 fail-open
// 可接受，详见设计文档。
func New(denyList []string) PathGuard {
	home, _ := os.UserHomeDir()
	rules := make([]rule, 0, len(denyList))
	for _, raw := range denyList {
		isDir := strings.HasSuffix(raw, string(filepath.Separator)) || strings.HasSuffix(raw, "/")
		expanded := raw
		if strings.HasPrefix(expanded, "~/") {
			if home == "" {
				continue
			}
			expanded = filepath.Join(home, expanded[2:])
		}
		if !filepath.IsAbs(expanded) {
			continue
		}
		rules = append(rules, rule{
			path:  filepath.Clean(expanded),
			isDir: isDir,
		})
	}
	return &defaultGuard{rules: rules}
}

// NewDefault returns a PathGuard configured with DefaultDenyList.
//
// NewDefault 返回用 DefaultDenyList 配置的 PathGuard。
func NewDefault() PathGuard {
	return New(DefaultDenyList)
}

// Allow checks absPath against the deny rules. Relative paths are rejected
// outright — callers must resolve before calling.
//
// Allow 按黑名单规则检查 absPath。相对路径直接拒——调用方先解析。
func (g *defaultGuard) Allow(absPath string) (bool, string) {
	if !filepath.IsAbs(absPath) {
		return false, "path must be absolute: " + absPath
	}
	cleaned := filepath.Clean(absPath)
	for _, r := range g.rules {
		if r.isDir {
			if cleaned == r.path || strings.HasPrefix(cleaned, r.path+string(filepath.Separator)) {
				return false, "path is denied by safety guard: " + r.path
			}
		} else if cleaned == r.path {
			return false, "path is denied by safety guard: " + r.path
		}
	}
	return true, ""
}
