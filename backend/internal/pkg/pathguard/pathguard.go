// Package pathguard is a deny-list path safety layer for filesystem tools.
//
// Package pathguard 是文件系统 tool 的路径黑名单守卫层。
package pathguard

import (
	"os"
	"path/filepath"
	"strings"
)

// PathGuard decides whether a tool may operate on absPath; reason surfaces in tool_result.
//
// PathGuard 决定 tool 是否可操作 absPath；reason 进 tool_result。
type PathGuard interface {
	Allow(absPath string) (allowed bool, reason string)
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

type rule struct {
	path  string
	isDir bool
}

type defaultGuard struct {
	rules []rule
}

// New returns a PathGuard denying paths matching denyList; non-absolute rules are silently dropped.
//
// New 返回拒绝 denyList 的 PathGuard；非绝对路径规则静默丢弃。
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

// Allow checks absPath against deny rules; relative paths are rejected.
//
// Allow 按黑名单检查 absPath；相对路径直接拒。
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
