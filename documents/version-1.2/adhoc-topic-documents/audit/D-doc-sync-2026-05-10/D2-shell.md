# D2 — `service-design-documents/shell.md` ↔ `internal/app/tool/shell/` Sync Audit

**Doc**: `documents/version-1.2/service-design-documents/shell.md` (404 lines)
**Code**: `backend/internal/app/tool/shell/` (6 files: `shell.go`, `bash.go`, `bash_route.go`, `manager.go`, `output.go`, `kill.go`)
**Spec authorities**: CLAUDE.md §S14 (doc-sync) + §S18 (Tool interface)

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| **Entire `bash_route.go` file (383 lines)** — Bash sandbox auto-routing system. Detects runtime-bound commands (`pip`, `python`, `npm`, `cargo`, `go`, `gem`, `mvn`, etc.) via mvdan.cc/sh/v3 AST walk + classifies into runtime kinds, then prepends per-conversation sandbox env's bin dir(s) to PATH. Coverage: nested `bash -c "pip ..."`, `env VAR=val python ...`, `/usr/bin/python3 ...`, chained `cd && python ...`, command substitution, `which python3`. Static escapes (`eval`, `source`) are best-effort. **None of this exists in design doc**. | `bash_route.go:1-383` (whole file) | HIGH |
| `Bash` struct has `sandbox *sandboxapp.Service` field for auto-route — design doc Bash struct (§5.2 implicit, §4.1 surface) shows no sandbox dependency | `bash.go:149-152` | HIGH |
| `NewShellTools(sandbox *sandboxapp.Service)` factory takes sandbox parameter — doc §4.4 shows `func NewShellTools() *ShellTools` (no params) | `shell.go:69` vs shell.md:204 | HIGH |
| `Bash.Execute` calls `t.maybeAutoRoute(ctx, cmdText)` before run; on auto-route failure for runtime command returns `formatAutoRouteError(autoRouteErr)` ("Sandbox auto-route could not prepare the runtime…") — doc §2 端到端推演 / §3 决策表 / §4.1 返回特殊情况均不提 | `bash.go:231-234, 259-267, 285-353` | HIGH |
| `Bash` description text contains big "Sandbox auto-routing" paragraph LLM-facing — describes auto-route + escape limitations (`eval`, `source`, `$(<dynamic>)`). Design doc §4.1 description column shows none of this LLM-facing copy. | `bash.go:98-100` | HIGH |
| `runtimeDetectors` regex list — 8 runtimes: python, node, rust, go, ruby, php, java, dotnet. Doc §1 says "故意不带 banned-command 列表" (correct) but never mentions runtime detection list. New runtime additions require updates to this list per code's TODO comment "extend: add one row here + one matching MiseInstaller registration in main.go" — design doc has no extension protocol section. | `bash_route.go:59-68` | MED |
| `Bash` runForeground / runBackground accept `extraPath []string` parameter for sandbox path prepend — doc §5.x impl-points sections show no PATH manipulation | `bash.go:445, 521, 629` | MED |
| `prependPath` / `envBinDirsForKind` helpers — Per-runtime PATH manipulation (Python venv `bin/Scripts`, Node `node_modules/.bin`, Rust/Go `bin`, Ruby `bundle/bin`, PHP `vendor/bin`) — doc has no equivalent | `bash_route.go:317-370` | MED |
| `shell.go` package doc still claims auto-route is silent fallthrough on nil sandbox; actual `bash.go:288-300` returns error to LLM when sandbox NOT ready (i.e. NOT silent) — package doc contradicts itself | `shell.go:8-11` (says no banned list, OK) vs `bash.go:288-300` (returns auto-route error) | LOW |
| `installprogresspkg.Run(ctx, ..., EnsureEnv)` wrap — install progress streamed as progress block under in-flight Bash tool_call (parent block from ctx). Per §S18 §3 this IS the Emitter pattern. Doc §2 端到端推演 doesn't mention progress streaming during sandbox env preparation. | `bash.go:337-347` | MED |
| `Snapshot` / `Snapshots()` API on `ProcessManager` for `/dev/bash-processes` inspection endpoint — doc §5.2 ProcessManager only mentions Register/Get/Remove/Stop, not Snapshots() | `manager.go:222-290` | LOW |

---

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §4.4 main.go装配 example: `shells := shelltool.NewShellTools()` (no args) — actual signature requires `*sandboxapp.Service` | shell.md:218 vs `shell.go:69` | HIGH (already counted in In-code-not-doc above; doc-side mirror) |
| §3 决策 row "Banned cmd metadata" final row says only "IsReadOnly=false（Bash）/ true（BashOutput）/ false（KillShell）" — descriptive only, but conspicuous absence: no "auto-route" row | shell.md:118 | MED |
| §6 Safety boundaries: "故意不带 banned-command list" + "不走 PathGuard" rows correct. Missing: "auto-route surface = sandbox env preparation safety implications" — i.e. runtime commands now go through mise-managed env, NOT raw system PATH; this is a security posture change worth documenting | shell.md:357-367 | MED |
| §7 测试覆盖 doesn't list `bash_route_test.go` — only mentions `bash_test.go` (14 tests). Actual code has `bash_route_test.go` (9 tests covering AST detection, env wrapper, which command, etc.) | shell.md:374 vs `bash_route_test.go` | LOW |
| §8 与其他 domain 的关系 lists "agentstate / chat / filesystem-search / forge / events / errmap" but doesn't list `sandbox` — per code, Bash now has direct dependency on `sandboxapp.Service` for auto-route | shell.md:387-393 | HIGH |
| §9 演化方向 says "Per-conversation 后台进程清理" + others; the auto-route system itself was a non-trivial Phase 5 evolution but has no entry — wasn't anticipated as evolution direction | shell.md:399-404 | LOW |

---

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| `BgProcess.Cmd *exec.Cmd` field — doc §5.2 ProcessManager pseudocode lines 270-285 use lowercase struct fields (`cmd *exec.Cmd`); actual struct has uppercase `Cmd` (exported), used by `Stop()` external loop | `manager.go:82` | LOW |
| `BashOutput.Execute` line 103 signature uses `_ context.Context` (unused) — doc §2 端到端推演 BashOutput shows ctx is consumed for cancellation, but actual code doesn't propagate ctx through `proc.drainNew()` (drainNew has no ctx param) | `output.go:103, manager.go:124` | LOW |
| Doc §2 端到端推演 KillShell flow says "存在但已 finished → 'Background shell ID already finished; removed from registry.'"; actual code path: Get() succeeds even for finished proc (still in registry until removed), Process.Kill() returns nil OR error. Code returns "already finished" only when Kill returned err (line 96-97 `if err := ...Kill(); err == nil { wasRunning = true }`). For an already-exited but still-registered proc, Kill might succeed (depends on reaper timing). Doc claim "已 finished → already finished" message is approximate. | `kill.go:91-105` | LOW |

---

## Sub-check

- **Tool list aligned**: yes — doc §4 lists Bash / BashOutput / KillShell; code factory `NewShellTools` returns those 3 in order.
- **9-method interface aligned**: yes — Each tool implements all 9 methods. `var _ toolapp.Tool = ...` checks at `bash.go:643`, `output.go:181`, `kill.go:110`.
- **Static metadata (IsReadOnly / NeedsReadFirst / RequiresWorkspace) aligned**: yes — All three tools match §S18 §8 table:
  - Bash: `(false, false, false)` ✓ — `bash.go:162-164`
  - BashOutput: `(true, false, false)` ✓ — `output.go:69-71`
  - KillShell: `(false, false, false)` ✓ — `kill.go:50-52`
- **Parameters schema aligned**: yes (mostly) — Bash schema fields (command, description, run_in_background, timeout) match doc §4.1 Args. BUT Bash description LLM-text contains an entire auto-route paragraph absent from doc. BashOutput / KillShell schemas align with doc §4.2 / §4.3.
- **Emit pattern (eventlog Emitter)**: **partial** — `bash.go:337-347` uses `installprogresspkg.Run(ctx, ..., EnsureEnv)` which streams sandbox install as progress block under tool_call parent. This IS §S18 §3 Emitter usage but **doc never mentions it**. Other shell tool methods don't emit (return final string).
- **Sentinel/errmap**: `ErrEmptyCommand`, `ErrInvalidTimeout`, `ErrEmptyBashID`, `ErrProcessNotFound` are tool-internal — never reach handler (per design — tools return friendly strings). Per §S17 don't need errmap. Doc §8 says "errmap 无登记" — aligned.

---

## Summary

**4 HIGH / 5 MED / 5 LOW** — shell.md is the **most stale of the 4 tool-domain docs**. The dominant issue: **entire Bash sandbox auto-route subsystem (bash_route.go + Bash.maybeAutoRoute + sandbox dep in factory) is invisible in design doc**. This is a structural gap, not drift:

1. **Factory signature breaks** for any reader trying to instantiate `NewShellTools()` per doc — sandbox arg is required.
2. **Bash tool description (LLM-facing)** has 2 paragraphs about auto-route the design doc never references, so reader can't reconcile what LLM "should know".
3. **§3 决策 / §6 安全边界** present old "no banned list, no PathGuard" story but miss the auto-route security posture that fundamentally changes the threat model (`pip install` no longer touches host system Python).
4. **§8 cross-domain relations** lacks the new sandbox dependency.

Recommended (out of audit scope): Add a §10 "Sandbox auto-route" section covering bash_route.go behaviour (AST-based runtime detection, escape coverage, runtime-detector extension protocol) + update §3 / §6 / §8 to reflect.
