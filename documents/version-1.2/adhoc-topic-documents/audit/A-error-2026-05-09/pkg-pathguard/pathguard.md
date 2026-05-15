# File trace: backend/internal/pkg/pathguard/pathguard.go

LOC: 128

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | pathguard.go:79-100 | `func New(denyList []string) PathGuard { home, _ := os.UserHomeDir(); rules := make([]rule, 0, len(denyList)); for _, raw := range denyList { isDir := strings.HasSuffix(...); expanded := raw; if strings.HasPrefix(expanded, "~/") { if home == "" { continue } expanded = filepath.Join(home, expanded[2:]) } if !filepath.IsAbs(expanded) { continue } rules = append(rules, ...) } return &defaultGuard{rules: rules} }` | A.1 | EDGE | §S3: Two silent-skip paths exist: (a) line 80 `home, _ := os.UserHomeDir()` — bare `_` ignores error with **no inline comment**; (b) line 86-88 `if home == "" { continue }` silently drops `~/`-prefixed rules; (c) line 91-93 `if !filepath.IsAbs(expanded) { continue }` silently drops non-abs entries. Items (b) + (c) are **documented at godoc** (lines 71-78: "Entries that fail to expand to an absolute path are silently dropped (fail-open is fine for a defense-in-depth layer; the design doc explains why)"). Item (a) — the bare `_` on `os.UserHomeDir` — is **not** §S3-annotated at the line; relies on (b) catching the consequence (`if home == ""`). **Strict §S3 reading**: bare `_` SHOULD have inline comment per §S3 typical violation list item 1 ("`_ = err` 没注释为什么忽略"). **Lenient reading**: the immediate `if home == ""` 6 lines down is the comment-by-code, and the entire paragraph is documented. **Verdict**: LOW EDGE — the silence is well-contained but the inline annotation is missing. Adding `// _ = err — bestEffort: home empty is handled below` would close it. **Security implication**: empty home string means **all `~/`-prefixed deny rules are dropped** — `~/.ssh/`, `~/.aws/`, `~/.gnupg/`, `~/.netrc`, browser login paths etc. become unguarded. On a system without HOME (rare but possible: container init, systemd unit without HomeDir, Windows in unusual config), Read/Write/Edit tools could touch `~/.ssh/`. Defense-in-depth claim relies on subprocess sandbox to backstop, but Forgify currently doesn't sandbox these tools. | LOW | On a HOME-less environment (rare), `~/.ssh/` and other Unix credential dirs become unguarded for fs tools. Forgify's primary sandbox layer (D5) treats this as defense-in-depth, but the layer is currently the only guard. | Add inline annotation: `home, _ := os.UserHomeDir() // best-effort: empty home → ~/-rules dropped (§S3 fail-open per godoc)`. Optionally: log a `Warn` at New() if `home == ""` to surface it in startup logs. | — |
| 2 | pathguard.go:32-60 | `var DefaultDenyList = []string{ "/etc/", "/usr/", ..., "C:/Windows/", "~/AppData/...", "~/.ssh/", ..., "~/.forgify/" }` | A.5 | OK | §S15/S17: not ID generation, not sentinel. This is a static deny-list. Cross-platform paths (mac/Linux + Windows + Unix HOME) coexist by silently dropping non-applicable entries at New() (line 91-93). Documented design (godoc lines 25-31). No errmap concern. | — | — | — | — |
| 3 | pathguard.go:113-128 | `func (g *defaultGuard) Allow(absPath string) (bool, string) { if !filepath.IsAbs(absPath) { return false, "path must be absolute: " + absPath }; cleaned := filepath.Clean(absPath); for _, r := range g.rules { if r.isDir { if cleaned == r.path || strings.HasPrefix(cleaned, r.path+string(filepath.Separator)) { return false, "..." } } else if cleaned == r.path { return false, "..." } } return true, "" }` | A.1/A.4 | OK | §S3: returns `(bool, string)` — no error type. The `(allowed, reason)` shape is the documented contract (godoc lines 14-21). reason string is consumed by tool callers at e.g. `edit.go:224-226 if ok, reason := t.pathGuard.Allow(args.FilePath); !ok { return reason, nil }` — surfaces to LLM as tool_result text. §S16 N/A: no `fmt.Errorf`. **Path traversal**: `filepath.Clean` at line 117 normalizes `../` etc. before prefix match, so an attacker passing `/tmp/foo/../etc/passwd` becomes `/etc/passwd` and gets caught by `/etc/`. Confirmed by inspection. **Symlink**: `filepath.Clean` does NOT resolve symlinks — a symlink at `/tmp/back-door → /etc` would currently bypass the guard. This is a known limitation (deny-list approach inherent to D5 design). Not a new violation. | — | — | — | — |
| 4 | pathguard.go:113-116 | `func (g *defaultGuard) Allow(absPath string) (bool, string) { if !filepath.IsAbs(absPath) { return false, "path must be absolute: " + absPath } ... }` | A.1 | OK | §S3: relative path → returned with explicit reason. No swallow — reason is delivered to tool_result so LLM sees "path must be absolute". Documented at godoc lines 109-112. | — | — | — | — |

## File summary

4 sites total. 3 OK, 1 EDGE (LOW). 0 strict violations.

The single EDGE concern: `os.UserHomeDir` error is silently dropped (line 80) without inline §S3 annotation. The consequence (HOME-less env → `~/`-prefixed deny rules dropped) is a real security trade-off, mitigated by:
1. Documented design intent (godoc lines 71-78 declares fail-open)
2. Immediate handling 6 lines down (`if home == "" { continue }`)
3. Defense-in-depth claim (subprocess sandbox is supposed to backstop)

What's missing: inline annotation at line 80 explaining the silence + ideally a startup `Warn` log if home is empty (so the user knows their guard list is partial).

No business IDs (§S15 N/A). No terminal-state writes (§S9 N/A). No sentinels (§S17 N/A). No `fmt.Errorf` (§S16 N/A).

## Notes for fix authors

If site #1 is fixed, the suggested annotation:

```go
// best-effort: empty home → ~/-rules silently dropped (§S3 fail-open per godoc)
// Practical impact: containers/systemd units without HOME lose Unix-credentials
// guard; primary defense remains subprocess sandbox per D5.
home, _ := os.UserHomeDir()
```

Or even better, surface at startup:

```go
home, err := os.UserHomeDir()
if err != nil || home == "" {
    log.Warn("pathguard: HOME unset, ~/-prefixed deny rules dropped",
        zap.Error(err))
}
```

(But this requires threading a logger into `New`, breaking current zero-arg `NewDefault`.)
