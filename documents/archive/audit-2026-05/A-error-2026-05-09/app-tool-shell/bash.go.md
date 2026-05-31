# audit: backend/internal/app/tool/shell/bash.go

LOC: 643
Read: full file (lines 1-643)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | bash.go:78 | `var ErrEmptyCommand = errors.New("command is required and must be non-empty")` | A.5 | OK | sentinel definition; package-level var Err...; consumed by ValidateInput, NEVER reaches errmap (Tool framework converts ValidateInput err → tool_result string per §S18). | N-A | — | — | — |
| 2 | bash.go:82 | `var ErrInvalidTimeout = fmt.Errorf("timeout must be between 0 and %d ms", maxTimeoutMS)` | A.5 | OK | sentinel-via-fmt.Errorf — value frozen at init; used as sentinel reference. Same N/A as #1. | N-A | — | — | — |
| 3 | bash.go:174-176 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("Bash.ValidateInput: %w", err) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w. | N-A | — | — | — |
| 4 | bash.go:178 | `return ErrEmptyCommand` | A.4/A.5 | OK | direct sentinel return — innermost layer. | N-A | — | — | — |
| 5 | bash.go:181 | `return ErrInvalidTimeout` | A.4/A.5 | OK | direct sentinel return. | N-A | — | — | — |
| 6 | bash.go:206-208 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("Bash.Execute: %w", err) }` | A.4 | OK | §S16 canonical. | N-A | — | — | — |
| 7 | bash.go:231-234 | `extraPath, autoRouteErr := t.maybeAutoRoute(ctx, cmdText); if autoRouteErr != nil { return formatAutoRouteError(autoRouteErr), nil }` | A.1 | POST-FIX OK | This is the B2 fix (commit 888739c) — auto-route error surfaced to LLM as friendly tool result instead of silently falling through to system shell. Cited in package doc + inline comment lines 224-230. | N-A | — | — | — |
| 8 | bash.go:290-292 | `return nil, fmt.Errorf("sandbox service not wired (this is a server build / config issue — please report)")` | A.4 | EDGE | §S16: NO pkg.Method prefix (`shelltool.Bash.maybeAutoRoute:`), NO sentinel, NO `%w`. Reachability: only triggers when Bash is wired without sandbox service — server boot misconfig. Wrapped at site #7 callsite into formatAutoRouteError → string for LLM, so never reaches errmap. **However**: if any future code path bubbles this up directly (e.g. health-check), it'd be unmapped. | LOW | only triggers on server-build/config bug; LLM still sees friendly "auto-route failed" string via formatAutoRouteError | add prefix: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox service not wired ...")` | FOUND |
| 9 | bash.go:294-299 | `bootErr := t.sandbox.BootstrapError(); reason := "bootstrap incomplete"; if bootErr != nil { reason = "bootstrap failed: " + bootErr.Error() }; return nil, fmt.Errorf("sandbox not ready (%s) — %s commands cannot run safely on the system shell", reason, kind)` | A.4 | EDGE | §S16: missing `shelltool.Bash.maybeAutoRoute:` prefix. Also: bootErr is collapsed into string via err.Error() — sentinel chain lost (e.g. if BootstrapError returns sandboxdomain.ErrRuntimeInstallFailed, errors.Is can't see it). Same EDGE as #8 — wrapped at #7 to LLM-friendly tool result. | LOW | sentinel-chain truncation only matters if a caller wants to errors.Is the bootstrap-failure cause (none currently) | wrap with %w for bootErr: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox not ready (%s) — %s commands need isolation: %w", reason, kind, bootErr)` (with safe nil-check) | FOUND |
| 10 | bash.go:301-304 | `convID, ok := reqctxpkg.GetConversationID(ctx); if !ok || convID == "" { return nil, fmt.Errorf("no conversation context — %s commands need a conversation-scoped sandbox env", kind) }` | A.4 | EDGE | §S16: missing prefix; no sentinel. ALSO: `reqctxpkg.ErrMissingConversationID` exists in errmap.go (line 166 verified) — should reuse with %w. | LOW | hits LLM-friendly tool_result via #7 wrap; loses ability for handler-level errors.Is(err, reqctxpkg.ErrMissingConversationID) discrimination | wrap: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: %w: %s commands need a conversation-scoped sandbox env", reqctxpkg.ErrMissingConversationID, kind)` | FOUND |
| 11 | bash.go:319-323 | `owner := sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindConversation, ID: convID + "_" + kind, ...}` | A.5 | POST-FIX OK | Comment lines 305-318 explicitly walks through the B1 regression fix (`:` → `_`). owner.ID now consistent with sandboxdomain.ErrInvalidOwnerID validation (e36f890). | N-A | — | — | — |
| 12 | bash.go:337-350 | `env, err := installprogresspkg.Run(ctx, ..., func(progress) { return t.sandbox.EnsureEnv(...) }); if err != nil { return nil, fmt.Errorf("sandbox env install failed (%s for %s): %w", kind, convID, err) }` | A.4 | EDGE | §S16: has %w ✓ but missing `shelltool.Bash.maybeAutoRoute:` prefix. Sentinel chain (sandboxdomain.ErrEnvCreateFailed / ErrInvalidOwnerID) preserved through inner %w. | LOW | identical UX (sentinel reaches errmap via inner %w wrap); harder to grep call site | wrap: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox env install failed (%s for %s): %w", kind, convID, err)` | FOUND |
| 13 | bash.go:391-396 | `if target == "" { if h, err := os.UserHomeDir(); err == nil { target = h } else { return "Cannot resolve home directory: " + err.Error(), nil } }` | A.1 | OK | os.UserHomeDir failure → soft-fail tool result string for LLM; documented carve-out per §S18 "failure paths return friendly tool_result". Caller (Bash.Execute) treats string as result — LLM reads + adapts. | N-A | — | — | — |
| 14 | bash.go:406-409 | `info, err := os.Stat(target); if err != nil { return fmt.Sprintf("cd: %s: %v", target, err), nil }` | A.1 | OK | os.Stat failure → friendly tool_result for LLM (mimics shell's `cd: <path>: No such file or directory`). Same §S18 carve-out as #13. | N-A | — | — | — |
| 15 | bash.go:414-417 | `state, ok := reqctxpkg.GetAgentState(ctx); if !ok { return "cd: agent state missing — cwd not persisted across calls. Subsequent commands will use the process default cwd.", nil }` | A.1 | OK | AgentState absent → soft-fail tool result. Comment at lines 384-389 explicitly documents intent. | N-A | — | — | — |
| 16 | bash.go:432-435 | `if c, err := os.Getwd(); err == nil { return c }; return "/"` | A.1 | EDGE | §S3: os.Getwd err silently dropped, fallback to `"/"`. **Reachability**: extremely rare (process cwd unreadable, e.g. parent dir deleted). **Fallback risk**: returning `"/"` may differ from intended cwd — commands run in unexpected dir. **However**: this only fires when AgentState is also missing AND process cwd unreadable, which is essentially impossible in normal operation. | LOW | minor — commands run in `/` if both AgentState missing AND os.Getwd fails (rare path) | add log.Warn somewhere reachable (resolveCwd has no logger param — would need plumbing) OR accept as documented fallback. Could add a package-level `var resolveCwdLog *zap.Logger` set at init. | FOUND |
| 17 | bash.go:454-456 | `err := cmd.Run(); output := capOutput(buf.Bytes())` | A.1 | OK | err captured into outer var — handled in switch below. | N-A | — | — | — |
| 18 | bash.go:458-470 | `case errors.Is(runCtx.Err(), context.DeadlineExceeded): ...; case errors.Is(runCtx.Err(), context.Canceled): ...` | A.4 | OK | ctx-error switch handles timeout + cancel cases distinctly. Comment at lines 461-469 explicitly documents the cancel case to avoid misleading LLM. | N-A | — | — | — |
| 19 | bash.go:471-476 | `var exitErr *exec.ExitError; if errors.As(err, &exitErr) { return ..., exitErr.ExitCode() }; return formatForegroundResult(output, -1, "exec failed: "+err.Error())` | A.4 | OK | errors.As destructures *exec.ExitError; non-ExitError path returns "exec failed: <err>" — string concat OK because target is LLM-facing tool result, not a propagated error. | N-A | — | — | — |
| 20 | bash.go:533 | `cmd := buildShellCmd(context.Background(), command, cwd, extraPath)` | A.2 | POST-FIX OK | **§S9 example**: background commands deliberately use `context.Background()` per comment at lines 522-532 — child processes outlive single chat turn. Same pattern as spawn.go::trackedHandle.unregister. Documented carve-out. | N-A | — | — | — |
| 21 | bash.go:535-542 | `stdout, err := cmd.StdoutPipe(); if err != nil { return fmt.Sprintf("Failed to open stdout pipe: %v", err), nil }; stderr, err := cmd.StderrPipe(); ...` | A.1/A.4 | OK | OS pipe-creation failures → friendly tool_result (LLM-facing string). Same §S18 carve-out — never propagated as error. | N-A | — | — | — |
| 22 | bash.go:544-546 | `if err := cmd.Start(); err != nil { return fmt.Sprintf("Failed to start background command: %v", err), nil }` | A.1 | OK | same carve-out as #21. | N-A | — | — | — |
| 23 | bash.go:568-588 | Reaper goroutine: `err := cmd.Wait(); switch { case err == nil: proc.markFinished(StatusExited, 0); default: var exitErr *exec.ExitError; if errors.As(err, &exitErr) { ... markFinished(StatusExited, exitErr.ExitCode()) ... } else { proc.markErrored(err) } }` | A.1/A.4 | OK | Reaper handles both *exec.ExitError (exit code) and signal-killed (-1) cases distinctly. Errored path stores err in BgProcess.lastError. No silence. | N-A | — | — | — |
| 24 | bash.go:599-611 | pumpReader: `n, err := r.Read(buf); if n > 0 { proc.appendOutput(buf[:n]) }; if err != nil { return }` | A.1 | OK | io.Reader.Read err includes EOF as termination signal (canonical Go pattern). Returning silently is correct — appending captured bytes happened first. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (site #16 EDGE LOW — borderline acceptable fallback)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (Bash tool returns strings, doesn't write DB)
  - ctx 来源: site #20 uses context.Background() deliberately for background spawn (POST-FIX OK §S9 example)
  - violations: not present

A.3 §S15 ID 生成:
  - ID generation calls: none in bash.go (delegates to manager.go::Register which calls idgenpkg)
  - violations: N/A — bash.go doesn't generate IDs

A.4 §S16 错误 wrap 格式:
  - violations: sites #8, #9, #10, #12 (LOW — missing `shelltool.Bash.maybeAutoRoute:` pkg.Method prefix on 4 fmt.Errorf calls inside maybeAutoRoute). All wrapped by formatAutoRouteError into LLM-friendly tool result via site #7 — never reach errmap, so functional impact is zero. Pure call-site grep traceability concern.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyCommand (line 78), ErrInvalidTimeout (line 82)
  - 已登记 errmap: N/A
  - missing: N/A — both are ValidateInput-returned sentinels per §S18 Tool framework. Framework converts to tool_result, never propagates to handler/errmap.
