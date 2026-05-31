# Package audit summary: internal/pkg/pathguard

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: One LOW EDGE — `home, _ := os.UserHomeDir()` at line 80 lacks inline annotation explaining why the error is dropped. The consequence (empty HOME → all `~/`-prefixed deny rules silently dropped) is documented at godoc lines 71-78 as deliberate fail-open, and the immediate `if home == ""` handling 6 lines down is comment-by-code. Strict §S3 reading: bare `_ = err` requires inline comment. Lenient: documented + handled. Marked LOW EDGE.
- **§S9 detached ctx 终态写**: **N/A** — no `ctx` parameter anywhere. PathGuard is a synchronous predicate evaluator. New() runs once at startup; Allow() is read-only string match.
- **§S15 ID 生成**: **N/A** — no business ID generation.
- **§S16 错误 wrap 格式**: **N/A** — zero `fmt.Errorf` / `errors.New`. Allow returns `(bool, string)` not error. New silently drops invalid entries (no error path).
- **§S17 errmap 单一事实源**: **N/A** — no sentinels defined. Path-deny outcomes flow as tool_result text via callers at `filesystem/{read,write,edit}.go` and `search/{glob,grep}.go`, never reach `responsehttpapi.FromDomainError`.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| pathguard.go | 128 | 4 | 3 | 0 | 0 | 1 |
| **TOTAL** | **128** | **4** | **3** | **0** | **0** | **1** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 1 | FOUND (site #1, missing §S3 inline annotation on `_ = err` from `os.UserHomeDir`) |

**Net: 0 strict violations; 1 LOW EDGE annotation gap**.

## Cross-cutting

### Security model: deny-list with fail-open

PathGuard is a **deny-list** (vs allow-list) defense layer. Per godoc + design doc reference at line 1-5 (D5 in `02-tools-deep/03-shell.md`):

- **Trust boundary**: Local single-user. Bash is exempt from PathGuard entirely (treated as user's daily shell command proxy). Filesystem tools (Read/Write/Edit/Glob/Grep) are guarded.
- **Defense-in-depth claim**: Subprocess sandbox (mise + per-plugin env isolation) is the primary safeguard. PathGuard is a secondary "common pitfall" filter.
- **Fail-open**: Invalid entries silently dropped at construction; an empty HOME drops `~/`-prefixed rules; rules whose expanded path isn't absolute on the running OS get dropped. Documented as acceptable for a defense-in-depth layer.

### Path-traversal: handled. Symlink: not handled.

`filepath.Clean(absPath)` at Allow() line 117 collapses `..` segments before prefix match — `/tmp/foo/../etc/passwd` normalizes to `/etc/passwd` and matches `/etc/`. Verified.

`filepath.Clean` does **not** resolve symlinks. An attacker who could create `/tmp/back-door → /etc` would bypass the guard. This is a deny-list-inherent limitation (cannot enumerate every path to which a symlink could point). Not a new audit finding — design constraint.

### Tool callers correctly handle reason string

5 callers verified — same shape:

```go
if ok, reason := t.pathGuard.Allow(args.FilePath); !ok {
    return reason, nil
}
```

Returns the reason as **tool_result content** (not error) so the LLM sees the human-readable denial reason. This matches the godoc contract that `reason` is "the human-readable explanation surfaced in tool_result errors". Confirmed at:

- `app/tool/filesystem/edit.go:224-226`
- `app/tool/filesystem/write.go:183`
- `app/tool/filesystem/read.go:219`
- `app/tool/search/glob.go:208`
- `app/tool/search/grep.go:258`

### Why no errmap entry

PathGuard never returns an error. Allow returns `(bool, string)`. The string is delivered as tool_result content to the LLM, never traverses the HTTP error path. errmap registration correctly absent.

### The single LOW EDGE: bare `_` on `UserHomeDir`

```go
home, _ := os.UserHomeDir()
```

Per CLAUDE.md §S3 typical violation list item 1 ("`_ = err` 没注释为什么忽略"), this technically requires an inline comment. The reasoning IS spelled out in godoc 6 lines above and the empty-string consequence is handled 6 lines below — but the literal "comment at the swallow site" rule isn't met.

Practical security implication: in a HOME-less environment (containers without `HOME=`, systemd units without `HomeDir=`, Windows in unusual config), all `~/`-prefixed deny rules get silently dropped. That removes guards for:

- `~/.ssh/` — SSH private keys
- `~/.aws/` — AWS credentials
- `~/.gnupg/` — GPG keys
- `~/.netrc` — pre-2026 service credentials
- `~/.docker/config.json`, `~/.kube/config`
- Browser saved-login files
- All Windows Credentials/Crypto/Vault paths
- Forgify's own state (`~/.forgify/`)

The **primary defense** (subprocess sandbox per D5) is supposed to backstop — but the current Forgify implementation does NOT sandbox the filesystem tools when PathGuard is the only layer (sandbox is for forge subprocess execution, not for the LLM's Read/Write/Edit calls within the agent loop's process).

**Recommendation**: at minimum, add inline annotation; ideally surface at startup via Warn log if HOME is unset.

## Spot-check (random clean sites)

3 sites picked across the file:

1. **pathguard.go:1-6** (package doc): bilingual godoc, references D5 design doc by path. Verified design doc reference is correct (`documents/version-1.2/service-design-documents/02-tools-deep/03-shell.md`).
2. **pathguard.go:32-60** (DefaultDenyList): visually inspected — covers macOS TCC blind spots (Login Data, Keychains), Linux runtime secrets (/proc, /run/secrets, systemd-creds), Windows DPAPI/Credential Manager/Vault, Unix user dotfiles (.ssh, .aws, .gnupg, .netrc, .docker, .kube), and Forgify's own data dir. Coverage matches what an LLM tool-use threat model would expect.
3. **pathguard.go:117-126** (Allow main loop): for-range over rules, dir-prefix check uses `+ string(filepath.Separator)` to avoid `/etcd` matching `/etc/` rule (prefix without separator would yield false positives). Mechanism correct.

All 3 spot-checks confirmed mechanism, not rubber-stamping.

## Recommended fix priorities

**LOW** (1):

1. (LOW) site #1: add inline annotation at line 80 (`home, _ := os.UserHomeDir()`) explaining the §S3 fail-open. Optional escalation: log Warn at startup when HOME is unset.

## Out-of-scope notes

1. **Symlink resolution** — currently `filepath.Clean` only normalizes textual `..` segments, not symlinks. Adding `filepath.EvalSymlinks` would close this gap but is significantly more expensive (per-call disk hit) and breaks idempotency expectations for non-existent paths (EvalSymlinks errors on missing files). Trade-off should be made at the design-doc level, not in this audit.
2. **Allow-list alternative** — a workspace-rooted allow-list (e.g., only allow paths under `cwd` or `~/Projects/`) would close the symlink hole and the HOME-less hole simultaneously, but would break the "Bash exempt + filesystem-tool guarded" symmetry currently designed in D5. Out of scope.
3. **Logger threading** — `New(denyList []string)` has no logger parameter. Adding one would let New emit a Warn when HOME is empty, but breaks the current zero-arg `NewDefault()` shape. Could be added as `NewWithLog(denyList, log)` keeping `New` and `NewDefault` unchanged. Not required for §S3 compliance — the inline comment alone suffices.
