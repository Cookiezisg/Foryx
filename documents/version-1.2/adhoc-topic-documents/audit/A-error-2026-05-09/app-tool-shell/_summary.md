# Package audit summary: internal/app/tool/shell

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: forbidden silent fallback — must surface errors or document soft-fail with rationale + audit log. shell tool's main §S3 risk is bash auto-route silent fallback (commit B2 / 888739c fixed: now surfaces sandbox-prep failures to LLM via formatAutoRouteError instead of running on system shell). Best-effort cleanup paths (`_ = Process.Kill()` on shutdown, `Cmd.Process.Kill()` returning ESRCH = "already done") are documented carve-outs.
- **§S9 detached ctx 终态写**: terminal-state writes use detached context. shell package has zero DB writes (ProcessManager is in-memory only, backend-process scoped); the §S9-relevant pattern here is **bash.go runBackground using `context.Background()`** (line 533) so background commands outlive the chat-turn ctx — documented carve-out matching spawn.go::trackedHandle.unregister.
- **§S15 ID 生成**: `idgenpkg.New("bsh")` in manager.go for background shell process IDs. "bsh" prefix matches §S15 spec list canonical entry. Internal panic-on-rand-fail invariant.
- **§S16 错误 wrap 格式**: `<pkg>.<Method>:` prefix + %w. Most call sites comply; bash.go::maybeAutoRoute has 4 fmt.Errorf calls missing prefix (LOW), and kill.go::ValidateInput uses inline errors.New rather than package-level var (LOW).
- **§S17 errmap 单一事实源**: shell package defines 3 sentinels (ErrEmptyCommand, ErrInvalidTimeout, ErrEmptyBashID, ErrProcessNotFound). All four are §S18 ValidateInput / Tool-framework consumed, NEVER reaching errmap — Tool framework converts ValidateInput errors and Execute-returned errors to tool_result strings before any handler path. **No errmap registration required**.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| bash.go | 643 | 24 | 14 | 4 | 0 | 6 |
| bash_route.go | 382 | 2 | 2 | 0 | 0 | 0 |
| manager.go | 311 | 8 | 7 | 0 | 0 | 1 |
| output.go | 181 | 7 | 7 | 0 | 0 | 0 |
| kill.go | 110 | 6 | 5 | 0 | 0 | 1 |
| shell.go | 79 | 1 | 1 | 0 | 0 | 0 |
| **TOTAL** | **1706** | **48** | **36** | **4** | **0** | **8** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (§S16 prefix style in bash.go::maybeAutoRoute) | 4 | bash.go:#8 (sandbox not wired), #9 (sandbox not ready), #10 (no conv ID), #12 (env install failed wrap missing prefix) | FOUND |
| LOW (§S3 silent without ritual comment) | 1 | manager.go:#8 (`_ = p.Cmd.Process.Kill()` on shutdown — function-level doc comment exists, missing inline `// _ = err` ritual) | FOUND |
| LOW (§S3 silent fallback fallback to "/") | 1 | bash.go:#16 (resolveCwd silent os.Getwd err → "/" fallback; AgentState-missing AND os.Getwd-fail compound condition is essentially impossible) | FOUND |
| LOW (§S16 sentinel-as-var consistency) | 1 | kill.go:#2 (inline `errors.New` rather than package-level `var ErrEmptyShellID`) | FOUND |
| LOW (§S17 reqctxpkg sentinel-chain truncation) | 1 | bash.go:#10 (could wrap `reqctxpkg.ErrMissingConversationID` with %w to preserve sentinel chain) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

Shell package's 4 sentinels (ErrEmptyCommand, ErrInvalidTimeout, ErrEmptyBashID, ErrProcessNotFound) are all §S18 Tool-framework-consumed and never reach errmap.go. Verified: none registered in errmap.go (and per §S18 they shouldn't be). Tool framework's contract: ValidateInput errs → tool_result string conversion; Execute-returned errs → tool_result string. Errmap path bypassed entirely for tool-tier errors.

### Auto-route §S3 compliance (B2 commit 888739c verification)

bash.go::Execute → maybeAutoRoute → formatAutoRouteError chain is the §S3-critical surface. Verified:

| Step | Site | §S3 verdict |
|---|---|---|
| detect runtime kind | bash_route.go::detectRuntime | ✓ pure function, can't error |
| sandbox missing/not ready/no convID | bash.go:#8/#9/#10 (maybeAutoRoute) | ✓ **POST-FIX** — returns err; caller (Execute site #7) routes to formatAutoRouteError → friendly tool_result for LLM |
| sandbox env install failed | bash.go:#12 | ✓ **POST-FIX** — same chain; sandbox sentinel preserved through %w |
| friendly tool_result format | bash.go::formatAutoRouteError (lines 259-267) | ✓ **POST-FIX** — body explicitly says "command was NOT executed (running on system shell would return misleading data — e.g. system Python 3.9.6 instead of conv 3.12 venv)" |

The B2 fix is intact and validated by the audit. The 4 LOW §S16 prefix issues at sites #8/#9/#10/#12 are call-site grep traceability concerns; functional B2 behavior is preserved.

### owner.ID PATH-meta validation (B1 commit 3cdf18a / e36f890 verification)

Verified bash.go:#11 uses `convID + "_" + kind` (NOT `:`) per the inline comment at lines 305-318 explicitly walking through the regression history. Compatible with sandboxdomain.ErrInvalidOwnerID validation at sandbox.go:#23 (added in commit e36f890). No regression risk in shell package.

### Detached ctx (§S9) audit

| Write | Site | Ctx | §S9 verdict |
|---|---|---|---|
| Background subprocess start | bash.go:#20 (runBackground line 533) | **context.Background()** — detached | ✓ **POST-FIX OK** — explicit comment lines 522-532 documents why: bg children outlive chat turn |
| ProcessManager.Register | manager.go:#5 | (no ctx — in-memory map) | ✓ N/A |
| ProcessManager.Stop | manager.go:#8 | (no ctx — process kill) | ✓ N/A — backend-shutdown path |

shell package's §S9 design is correct: foreground commands ride the request ctx (cancellation kills them), background commands explicitly detach. No DB writes anywhere, so no terminal-write §S9 concerns.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

7 sites picked from `OK` set across 6 files:

1. **bash.go:#3** (line 174-176, ValidateInput JSON unmarshal): verified — `fmt.Errorf("Bash.ValidateInput: %w", err)`. pkg.Method prefix ✓, %w ✓. Compliance literal.
2. **bash.go:#11** (owner.ID `_` separator): verified — exact `convID + "_" + kind` form per the B1 fix; comment explicitly walks through PATH-meta regression history (lines 305-318). Compatible with sandboxdomain.ErrInvalidOwnerID validation in sandbox.go.
3. **bash.go:#18** (ctx-cancel exec.ExitError handling): verified — `errors.Is(runCtx.Err(), context.Canceled)` distinguishes user-cancel from command-side crash. Comment lines 461-469 explicit about avoiding "exec failed: signal: killed" misleading message.
4. **bash_route.go:#1** (parser fallback): verified — first-token fallback for parse failure is documented at file header lines 78-82. AST-based detection covers nested constructs that first-token regex cannot — the fallback is not a §S3 silence but a documented degradation.
5. **manager.go:#5** (idgenpkg.New("bsh")): verified — exact §S15 form; "bsh" prefix matches CLAUDE.md §S15 spec list canonical entry "bsh_ Bash 后台 shell 进程". idgenpkg internal panic-on-rand-fail per spec.
6. **output.go:#7** (regexp.MustCompile after ValidateInput): verified — inline comment lines 121-122 explicit about safety: "Validated in ValidateInput; safe to MustCompile here." Two-phase validation (Validate → Execute) is the documented Tool framework contract.
7. **kill.go:#5** (Process.Kill ESRCH carve-out): verified — comment lines 93-95 explicit: "Best-effort; kill on already-exited proc returns ESRCH which we treat as 'already done' rather than user-facing error". Standard POSIX semantics, idempotent design.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The package's primary §S3 surface (auto-route) is **POST-FIX OK** per the B2 commit, and the audit's primary findings (4 LOW §S16 prefix style + 4 LOW miscellaneous) are pure call-site traceability / consistency concerns with no functional impact.

## Recommended fix priorities

1. **bash.go::maybeAutoRoute §S16 prefix sweep** (LOW × 4) — sites #8/#9/#10/#12 should add `shelltool.Bash.maybeAutoRoute:` prefix. Pure call-site traceability; no behavior change. Single sweep commit.

2. **bash.go:#10 reqctxpkg sentinel preservation** (LOW) — wrap with `reqctxpkg.ErrMissingConversationID` so handler-level errors.Is can discriminate. Currently impossible because the err goes through formatAutoRouteError → tool_result string, but future code paths might benefit.

3. **kill.go:#2 promote inline errors.New to package-level var** (LOW) — for consistency with ErrEmptyCommand / ErrEmptyBashID.

4. **manager.go:#8 inline ritual comment** (LOW) — `_ = err // best-effort kill on shutdown; OS reaps orphans` for spec literal compliance. Or accept function-level doc comment as sufficient.

5. **bash.go:#16 resolveCwd silent fallback** (LOW) — log when both AgentState missing AND os.Getwd fails (combined edge case essentially impossible in normal operation; could WAIVE).

All findings are LOW; no immediate-action MED/HIGH. Package is healthy enough that all 8 EDGE items can batch as a single sweep commit when convenient, or remain FOUND as documented design choices.

## Out-of-scope notes

1. **§S18 9-method conformance** — Bash, BashOutput, KillShell each implement all 9 required methods (Identity 3 + Static metadata 3 + Args-dependent 2 + Execute 1). Verified by inspection. Bash.RequiresWorkspace() returns false with explicit comment per `02-tools-deep/03-shell.md` decision D5 (Bash deliberately bypasses PathGuard for single-user local context).
2. **Standard-injected fields** (summary / destructive / execution_group) — not directly visible in shell tool source; framework injects via ToolEvent path. Out of audit scope.
3. **PathGuard / NeedsReadFirst metadata** — Bash returns false on both, deliberately per package doc lines 14-23. No §S3 / §S9 concern but worth a Phase B audit cross-check.
