# audit: backend/internal/app/forge/sandbox_adapter.go

LOC: 316
Read: full file (lines 1-316)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | sandbox_adapter.go:79-88 | `func (a *SandboxAdapter) PythonPath() string { a.pythonPathOnce.Do(func() { path, err := a.svc.EnsureTool(context.Background(), "python", ""); if err != nil { return }; a.pythonPath = path }); return a.pythonPath }` | A.1/A.4 | **VIOLATION** | **§S3 silent fallback**: EnsureTool error is silently swallowed inside sync.Once.Do — no log, no fallback indication. Doc comment claims "PythonPath returns '' if EnsureTool fails — caller treats '' as 'AST parse unavailable' and degrades gracefully" — but downstream caller (forge.go::ParseCode → ast.go::parseForgeCode) treats "" as **fall back to system PATH `python3`** (ast.go:151-153), NOT "AST parse unavailable". So the silent failure means: bundled Python sandbox initialization fails → silently use system Python → forges run against unintended Python version with whatever system packages are on PATH. This is a real correctness issue masked as a graceful degradation. | **HIGH** | sandbox bootstrap failure → PythonPath returns "" → ast/run paths use system python3 instead of bundled python-build-standalone → user's forges may run against wrong Python version with arbitrary system packages. **Worse than the bash auto-route fallback bug** (commit B2 888739c) — same defect class but in forge run path. No log surfaces the bootstrap failure either. | (a) log Warn on EnsureTool failure: `s.log.Warn("forgeapp.SandboxAdapter.PythonPath: bundled Python unavailable; AST parsing will fall back to system python3", zap.Error(err))`; (b) reconsider the contract — should "" really fall through to system python3 in ast.go:151? Per the §S3 lesson from bash auto-route (3cdf18a), silent degradation that runs user code against unintended interpreter is the canonical anti-pattern. Recommend: surface the error so caller knows to error rather than silently using system Python. NOTE: SandboxAdapter has no logger field currently — needs adding | FOUND |
| 2 | sandbox_adapter.go:97-117 | `func (a *SandboxAdapter) Sync(ctx, req) error { ...; if _, err := a.svc.EnsureEnv(ctx, owner, spec, stream); err != nil { return &SyncError{Cause: err, Stderr: err.Error()} }; return nil }` | A.4 | EDGE | §S16: returns `*SyncError` (which has Unwrap() → Cause → preserves sentinel chain via errors.Is). NO `forgeapp.SandboxAdapter.Sync:` prefix wrapping — by design, the return type IS the wrap. Style differs from the canonical fmt.Errorf wrap but functionally compliant via the SyncError type. | LOW | none — type-based wrapping works for errors.As/Is | optional: also add prefix at outer level: `return fmt.Errorf("forgeapp.SandboxAdapter.Sync: %w", &SyncError{...})` for grep traceability; but unconventional and may break errors.As(err, &sandboxErr) if errors.As doesn't drill | FOUND |
| 3 | sandbox_adapter.go:131-148 | `func Run(ctx, req): if err := os.MkdirAll(verDir, 0o755); err != nil { return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: mkdir verDir: %w", err) }; ...; if err := writeAtomic(...); err != nil { return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: write main.py: %w", err) }; inputJSON, err := json.Marshal(req.Input); if err != nil { return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: marshal input: %w", err) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + `%w` throughout. Multi-stage prefix gives precise call-site loc. | N-A | — | — | — |
| 4 | sandbox_adapter.go:140-143 | `funcName, err = extractFuncName(req.Code); if err != nil { return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: %w", err) }` | A.4 | OK | §S16 canonical wrap; extractFuncName error preserved | N-A | — | — | — |
| 5 | sandbox_adapter.go:159-169 | `res, spawnErr := a.svc.Spawn(ctx, owner, sandboxdomain.SpawnOpts{...}); if spawnErr != nil { return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: %w", spawnErr) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 6 | sandbox_adapter.go:188-190 | `if err := json.Unmarshal(res.Stdout, &output); err != nil { output = strings.TrimSpace(string(res.Stdout)) }` | A.1 | OK | §S3: silent JSON parse fallback to raw string is **explicitly documented** at lines 184-186 ("Stdout is JSON by convention; raw string fallback for forges that printed something else"). Caller sees the raw string in output. §S3 carve-out: documented intent + observable user behavior (Output field still populated). | N-A | — | — | — |
| 7 | sandbox_adapter.go:201-216 | `WriteCodeFile: if err := os.MkdirAll(verDir, 0o755); err != nil { return fmt.Errorf("forgeapp.SandboxAdapter.WriteCodeFile: mkdir: %w", err) }; ...; funcName, err = extractFuncName(code); if err != nil { return fmt.Errorf("forgeapp.SandboxAdapter.WriteCodeFile: %w", err) }; ...; return writeAtomic(...)` | A.4 | EDGE | §S16: WriteCodeFile returns from writeAtomic at line 215 BARE without a `forgeapp.SandboxAdapter.WriteCodeFile:` wrap. writeAtomic returns os errors directly (line 311-315). The bare return loses call-site context. | LOW | identical UX (os errors propagate); harder to grep | wrap: `if err := writeAtomic(...); err != nil { return fmt.Errorf("forgeapp.SandboxAdapter.WriteCodeFile: write: %w", err) }; return nil` | FOUND |
| 8 | sandbox_adapter.go:224-243 | `Destroy: envs, err := a.svc.ListEnvs(...); if err != nil { return fmt.Errorf("forgeapp.SandboxAdapter.Destroy: list envs: %w", err) }; ...; if err := a.svc.Destroy(ctx, owner); err != nil { return fmt.Errorf("forgeapp.SandboxAdapter.Destroy %s: %w", owner.ID, err) }; ...; if err := os.RemoveAll(forgeDir); err != nil { return fmt.Errorf("forgeapp.SandboxAdapter.Destroy: rm %s: %w", forgeDir, err) }` | A.4 | OK | §S16 canonical throughout | N-A | — | — | — |
| 9 | sandbox_adapter.go:251-257 | `DestroyEnv: ...; return a.svc.Destroy(ctx, owner)` | A.4 | EDGE | §S16: bare passthrough — Service.Destroy wraps internally; sentinel preserved. Inconsistent with Destroy() (#8) which wraps. | LOW | identical UX; harder to grep | wrap: `return fmt.Errorf("forgeapp.SandboxAdapter.DestroyEnv %s: %w", owner.ID, a.svc.Destroy(ctx, owner))` — but be careful since wrapping a nil err produces "...: <nil>" with non-nil result; use the standard if-err pattern instead | FOUND |
| 10 | sandbox_adapter.go:292-304 | `func extractFuncName(code string) (string, error) { ...; if idx := strings.IndexAny(rest, "(: "); idx > 0 { return rest[:idx], nil } }; return "", fmt.Errorf("no function definition found in code")` | A.4 | EDGE | §S16: line 303 returns `fmt.Errorf("no function definition...")` with NO `forgeapp.extractFuncName:` prefix and NO sentinel. Should either be a sentinel (`forgedomain.ErrNoFunctionDef`) or have pkg-method prefix. Caller (sandbox_adapter.go #4) wraps with `forgeapp.SandboxAdapter.Run: %w` so prefix accumulates, but the inner err has no sentinel for errors.Is discrimination. | LOW | identical UX; errors.Is can't distinguish "no function def" from other str-only errors | introduce `forgedomain.ErrNoFunctionDef` sentinel + register errmap (or reuse `forgedomain.ErrASTParseError` since it's the same defect class) | FOUND |
| 11 | sandbox_adapter.go:310-316 | `func writeAtomic(path, data, mode): if err := os.WriteFile(tmp, data, mode); err != nil { return err }; return os.Rename(tmp, path)` | A.4 | EDGE | §S16: bare-return on `os.WriteFile` and `os.Rename` errors — no `forgeapp.writeAtomic:` prefix. Caller (Run / WriteCodeFile) wraps with their own pkg.Method prefix (#3, #7), so call site is preserved at outer wrap. Style choice: helper hands raw os err to caller for context. **Issue**: if the temp file write succeeds but Rename fails, the .tmp file is left behind (no cleanup) — separate from §S16 but a §S3-adjacent concern (silent leftover). | LOW | identical UX (caller wraps); leftover .tmp on Rename failure | (a) wrap at writeAtomic for grep: `return fmt.Errorf("forgeapp.writeAtomic: %w", err)` for both os errors; (b) cleanup on rename fail: `if err := os.Rename(tmp, path); err != nil { _ = os.Remove(tmp); return fmt.Errorf(...) }` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #1 (HIGH — silent EnsureTool failure in PythonPath causes bundled-Python bootstrap failures to fall through to system python3 in ast.go; defect class similar to bash auto-route silent fallback B2 fix)
  - EDGE: site #6 documented json.Unmarshal fallback (OK); site #11 leftover tmp file (LOW)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none in this file (Sync/Run delegate to sandboxapp.Service which owns env writes; Destroy delegates similarly)
  - 各自 ctx 来源: N/A
  - violations: N/A — adapter is pass-through to sandboxapp; terminal writes happen inside that service (audited separately in app-sandbox)

A.3 §S15 ID 生成:
  - ID generation calls: none (uses `req.EnvID` produced by ComputeEnvID in sandbox_types.go; Owner.ID is `<forgeID>:<envID>` composition)
  - violations: N/A — file generates no business IDs (the colon-separated owner.ID format is owner-scoping not §S15 business ID)
  - **NOTE**: line 100 + 157 + 254 use `req.ForgeID + ":" + req.EnvID` — this is the SAME pattern that triggered the B1 bug (sandbox owner.ID with `:` broke PATH parsing). This is forge owner.ID specifically, not conv owner.ID. **Cross-cutting concern**: app-sandbox audit recently introduced `ErrInvalidOwnerID` rejecting `:` in owner.ID strings. **This forge adapter generates `<forgeID>:<envID>` owner.IDs that WILL be rejected by sandbox.go::EnsureEnv after the e36f890 fix.** Critical regression risk — needs verification.

A.4 §S16 错误 wrap 格式:
  - violations: not present strictly; but EDGE LOW on sites #2 (Sync uses type-based wrap not fmt.Errorf), #7 (WriteCodeFile bare return from writeAtomic), #9 (DestroyEnv bare passthrough), #10 (extractFuncName no prefix + no sentinel), #11 (writeAtomic helper bare returns)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap: forgedomain sentinels consumed via Spawn/EnsureEnv/Destroy chains — all already in errmap.go:78-91
  - missing: see #10 (extractFuncName no-function-def error has no sentinel — could be classified under `forgedomain.ErrASTParseError`)

## **CRITICAL Cross-cutting Concern: B1-Regression Risk**

**Site #1 issue (PythonPath silent failure)** + **forge owner.ID composition `<forgeID>:<envID>`** combined create a regression risk:

1. The B1 fix (commit 3cdf18a) changed bash auto-route from `cv_xxx:python` → `cv_xxx_python` to avoid PATH-meta `:` in owner.ID
2. The recent app-sandbox §S17 fix (e36f890) added `sandboxdomain.ErrInvalidOwnerID` rejecting `:` in owner.ID via `strings.ContainsAny(owner.ID, ":;= \t\n\r\x00")`
3. **forge SandboxAdapter at lines 100, 157, 254 STILL uses `<forgeID>:<envID>` format with literal `:`**
4. After e36f890 deploys, every forge Sync / Run / DestroyEnv call will be rejected with `ErrInvalidOwnerID`

**This is a HIGH-priority bug that the audit catches but isn't classified as such per-site because each individual occurrence is a single line of pass-through code.** The aggregate impact is: **forge package will be broken end-to-end after e36f890**.

**Recommended fix**: change `:` → `_` in all 3 sites (matching the bash auto-route fix), OR widen sandboxdomain's PATH-meta check to allow `:` for forge owner kind only (less clean — special-case logic in validation).

**Severity**: HIGH (cross-cutting — should be raised to top of recommended fix priorities in _summary).
