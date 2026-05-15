# audit: backend/internal/app/tool/filesystem/read.go

LOC: 333
Read: full file (lines 1-333)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix |
|---|---|---|---|---|---|---|---|---|
| 1 | read.go:54-69 | `var ErrEmptyFilePath = errors.New("file_path is required") ... ErrPathNotAbsolute / ErrNegativeOffset / ErrNegativeLimit` | A.5 | OK | 4 tool-validation sentinels. Returned via `ValidateInput` → caught by §S18 Tool framework → converted to friendly tool_result string. Never reach `responsehttpapi.FromDomainError`. errmap registration N/A per §S17 "完全包内 / 跨包但只在 service 层消费" carve-out | N-A | — | — |
| 2 | read.go:153-155 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("Read.ValidateInput: %w", err) }` | A.4 | OK | §S16 canonical: `<pkg>.<Method>:` (where pkg is tool name, consistent with toolapp convention) + %w | N-A | — | — |
| 3 | read.go:156-167 | bare-return of validation sentinels (Empty/NotAbsolute/Negative*) | A.4 | OK | §S16 spec example: "直接返 sentinel（最里层无需 wrap）" | N-A | — | — |
| 4 | read.go:204-206 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("Read.Execute: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — |
| 5 | read.go:219-221 | `if ok, reason := t.pathGuard.Allow(args.FilePath); !ok { return reason, nil }` | A.1 | OK | §S18 RequiresWorkspace=true ✓ self-checks via pathGuard. PathGuard refusal returned as LLM-friendly string with `nil` error per §S18 tool_result contract — not §S3 silent fallback (refusal IS the signal) | N-A | — | — |
| 6 | read.go:225-228 | `info, err := os.Stat(cleaned); if err != nil { return statErrorMessage(cleaned, err), nil }` | A.1 | OK | os errors mapped to friendly LLM strings (NotExist / Permission / generic) per §S18 — LLM can recover. Doc comment lines 186-189 explicitly documents this convention | N-A | — | — |
| 7 | read.go:235-238 | empty file case: `markSeen + return system-reminder` | A.1 | OK | Defensive UX — empty file isn't an error but the LLM might be confused. Mark Read so subsequent Edit/Write of the file passes guard | N-A | — | — |
| 8 | read.go:240-243 | `f, err := os.Open(cleaned); if err != nil { return statErrorMessage..., nil }` | A.1 | OK | same friendly-string pattern | N-A | — | — |
| 9 | read.go:244 | `defer f.Close()` | A.1 | OK | §S3 spec carve-out: "defer f.Close() 在只读路径（Close 返错对调用方无意义）" — Read is exclusively read-only | N-A | — | — |
| 10 | read.go:253-264 | scanner loop `for scanner.Scan() { ... }` (no err check inside loop, post-loop check at line 265) | A.1 | OK | scanner.Err() correctly checked after loop at line 265 (read.go's own pattern). Compare to infra/sandbox audit which fixed missing scanner.Err check (commit d2b8af8) — read.go was already correct | N-A | — | — |
| 11 | read.go:265-269 | `if err := scanner.Err(); err != nil { return Failed to read..., nil }` | A.1 | OK | scanner err returned as friendly LLM string with nil Go error. Doc comment lines 266-268 cites the typical case (line exceeds maxScannerLineBytes) | N-A | — | — |
| 12 | read.go:273-275 | `if written >= args.Limit && hasMoreLines(scanner) { ... truncation hint }` (calls scanner.Scan one more time at line 315) | A.1 | EDGE | hasMoreLines consumes one more Scan to peek. Doc at line 308-313 documents that the scanner is consumed regardless. **Edge concern**: if that final Scan errors (e.g. mid-file truncation), the error is silently dropped — but at this point we've already decided to emit the truncation hint and the user impact is "hint says truncated; actually maybe not". LOW severity, OK by §S3 (best-effort hint, not silent fallback) | LOW | hint accuracy in rare scanner-error-during-peek case | could check `scanner.Err() == nil` before treating Scan==false as "no more lines"; or accept the current best-effort behavior | — |
| 13 | read.go:295-306 | `statErrorMessage` helper — switch on `errors.Is(err, fs.ErrNotExist)` / `fs.ErrPermission` / default | A.4 | OK | Properly uses errors.Is to discriminate — no string matching. Returns LLM-friendly string per Execute's friendly-error contract | N-A | — | — |
| 14 | read.go:325-329 | `markSeen: if state, ok := reqctxpkg.GetAgentState(ctx); ok { state.MarkRead(...) }; else: no-op` | A.1/A.2 | EDGE | Documented carve-out (lines 318-324): "AgentState 缺失（chat 层未注入）时 no-op——Read 仍成功，但后续对该 path 的 Edit/Write 需重新 Read". This is graceful degradation, not silent failure — Read result is still valid; only chained Edit/Write loses the cache. **However**: a server-side wiring bug (chat layer fails to inject AgentState) goes invisible — a Warn log would catch it for ops. | LOW | server-wiring bug masked; user just gets re-asked to Read on Edit/Write | optional: `s.log.Warn("Read.markSeen: AgentState missing — chat layer wiring bug")`; but Read struct has no logger currently — adding one is signature change. WAIVE-able if accept the documented degraded behavior | — |
| 15 | read.go:333 | `var _ toolapp.Tool = (*Read)(nil)` | — | OK | compile-time interface check; nothing to audit | N-A | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site #12 (hasMoreLines scanner peek error silently dropped — LOW EDGE, hint-only); site #14 (markSeen AgentState miss — documented graceful degrade)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none — Read is read-only; markSeen writes to in-memory AgentState (per-conv), not DB
  - 各自 ctx 来源: N/A
  - violations: N/A — package doesn't do DB terminal writes (Read is intrinsically read-only per §S18 IsReadOnly=true)

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — Read is read-only and produces no business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - sites verified: #2 / #4 (json.Unmarshal wraps); #3 (bare-sentinel returns at validation deepest layer — §S16 spec OK)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyFilePath, ErrPathNotAbsolute, ErrNegativeOffset, ErrNegativeLimit (4 in this file)
  - 已登记 errmap: none
  - missing: N/A — all 4 are tool-ValidateInput sentinels; the §S18 Tool framework intercepts Execute/ValidateInput errors and converts to friendly tool_result strings before they ever reach `responsehttpapi.FromDomainError`. errmap registration is for handler-path sentinels only per §S17 spec literal "完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记"
