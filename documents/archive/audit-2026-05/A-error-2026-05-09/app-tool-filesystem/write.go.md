# audit: backend/internal/app/tool/filesystem/write.go

LOC: 281
Read: full file (lines 1-281)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix |
|---|---|---|---|---|---|---|---|---|
| 1 | write.go:118-138 | `ValidateInput: json.Unmarshal err → fmt.Errorf("Write.ValidateInput: %w", err); empty file_path → ErrEmptyFilePath; not abs → ErrPathNotAbsolute; nil content → errors.New("content field is required ...")` | A.4 | EDGE | line 135 `errors.New(...)` for missing content has no sentinel, no `<pkg>.<Method>:` prefix. Per §S16 spec literal: "**禁止**裸 `errors.New` 套娃丢失原 sentinel" — strictly speaking a bare errors.New IS allowed when there's no upstream sentinel to wrap (this is "innermost layer"), but it deviates from sibling sentinels like ErrEmptyFilePath (defined at file scope). Should either define `ErrContentMissing` sentinel for parity OR keep as-is (bare error at validation layer is functionally OK) | LOW | identical UX (LLM sees error string either way); style inconsistency only | introduce `ErrContentRequired = errors.New("content field is required ...")` at file scope for parity with read.go's sentinels | — |
| 2 | write.go:178-180 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("Write.Execute: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — |
| 3 | write.go:183-185 | `if ok, reason := t.pathGuard.Allow(args.FilePath); !ok { return reason, nil }` | A.1 | OK | §S18 RequiresWorkspace=true ✓ — Write self-checks via pathGuard.Allow as required by static metadata declaration. Refusal = friendly LLM string (§S18 contract) | N-A | — | — |
| 4 | write.go:191-200 | `parentInfo, err := os.Stat(parent); err handling: NotExist → friendly hint "use mkdir -p"; other err → friendly access msg; not-dir → friendly` | A.1 | OK | All os errors mapped to LLM-friendly strings with nil Go error. Includes case-specific hints (NotExist suggests mkdir -p) — defensive UX | N-A | — | — |
| 5 | write.go:203-207 | `existingInfo, statErr := os.Stat(cleaned); exists := statErr == nil; if exists && existingInfo.IsDir() → friendly` | A.1 | OK | Stat error implicitly classified as "doesn't exist" — but other errors (permission denied) also collapse to "doesn't exist" path here. **Subtle**: line 203 doesn't differentiate ErrNotExist from ErrPermission; if ErrPermission, `exists=false` and we bypass the must-Read-first guard, then attempt to write a fresh file at the path. **Recoverable** because the actual `os.CreateTemp` at line 226 would also hit permission denied and surface a friendly message. So no real bug, just a subtle confusion in the variable naming. | LOW | edge case: "permission denied on Stat" treated like "file doesn't exist"; subsequent CreateTemp surfaces real error | optional: differentiate `errors.Is(statErr, fs.ErrNotExist)` from "other error" branches; current behavior is robust because downstream catches | — |
| 6 | write.go:209-217 | must-Read-first: `state, hasState := reqctxpkg.GetAgentState(ctx); if !hasState { return "Cannot verify Read-first guard: agent state missing. Read the file first.", nil }` | A.1 | **OK (POSITIVE EXAMPLE)** | §S18 NeedsReadFirst=true ✓ — Write actually self-checks AgentState (not just metadata declaration). When state is missing, **refuses the overwrite** rather than silently allowing it. Doc comment at lines 211-216 explicitly documents the defense: "防御：服务端接线 bug。拒绝覆写以匹配 must-Read-first 不变量（静默放过会让整个守卫形同虚设）" — exactly the §S3 anti-silent-fallback discipline. Best practice across the audit. | N-A | — | — |
| 7 | write.go:218-221 | `if _, seen := state.WasRead(cleaned); !seen { return "File must be read first ...", nil }` | A.1 | OK | must-Read-first check returns LLM-friendly refusal | N-A | — | — |
| 8 | write.go:226-229 | `tmpFile, err := os.CreateTemp(parent, ".forgify-write-*"); if err != nil { return ... friendly, nil }` | A.1 | OK | tmp creation failure as friendly LLM string | N-A | — | — |
| 9 | write.go:231-235 | `cleanup := func() { _ = os.Remove(tmpPath) }` | A.1 | OK | §S3 carve-out: "panic 路径里的 cleanup" applies — best-effort cleanup of orphan tmp on subsequent error path. Doc at line 232-234 explicitly documents intent: "尽力清理；失败不致命（文件残留）" — meets §S3 inline justification ritual | N-A | — | — |
| 10 | write.go:237-241 | `if _, err := tmpFile.WriteString(args.Content); err != nil { _ = tmpFile.Close(); cleanup(); return ... }` | A.1 | EDGE | line 238 `_ = tmpFile.Close()` discards Close error in error-path cleanup. Per §S3 spec carve-out for cleanup paths this is acceptable, but missing the inline `// _ = err — <reason>` ritual the spec requires. Same for site #11 / #15 / #18. | LOW | none — Close failure on already-failed write doesn't matter for caller | add inline comment: `_ = tmpFile.Close() // close-after-write-fail; cleanup is best-effort` for ritual | — |
| 11 | write.go:242-245 | `if err := tmpFile.Close(); err != nil { cleanup(); return ... friendly, nil }` | A.1 | OK | Close error properly captured (this is the success-path Close) and converted to friendly | N-A | — | — |
| 12 | write.go:253-260 | `mode := defaultFileMode; if exists { mode = existingInfo.Mode().Perm() }; if err := os.Chmod(tmpPath, mode); err != nil { cleanup(); return ... }` | A.1 | OK | preserves existing file mode on overwrite to prevent CreateTemp's 0600 from silently shrinking permissions. Doc at lines 247-252 documents the rationale | N-A | — | — |
| 13 | write.go:262-265 | `if err := os.Rename(tmpPath, cleaned); err != nil { cleanup(); return ... friendly, nil }` | A.1 | OK | atomic rename; failure surfaces as friendly LLM string after cleanup of orphan tmp | N-A | — | — |
| 14 | write.go:272-274 | `if state, ok := reqctxpkg.GetAgentState(ctx); ok { state.MarkRead(cleaned, int64(len(args.Content))) }` | A.1 | EDGE | Same documented graceful-degrade pattern as read.go markSeen — AgentState miss leaves file unmarked; future Edit will re-ask Read. Same LOW concern about silent server-wiring bug | LOW | wiring bug invisible | optional Warn log — same trade-off as read.go site #14 | — |
| 15 | write.go:281 | `var _ toolapp.Tool = (*Write)(nil)` | — | OK | compile-time check | N-A | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site #6 is a **positive example** — Write properly refuses on AgentState miss rather than silently allowing the must-Read-first bypass; site #10 has a `_ = tmpFile.Close()` cleanup-after-write-fail missing inline ritual comment (LOW); site #14 same graceful-degrade as read.go site #14

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: file write itself is "terminal-on-disk" but not a DB write
  - 各自 ctx 来源: N/A — uses ctx only for AgentState access via reqctx
  - violations: N/A — package doesn't do DB terminal writes; file writes are atomic via tmp+rename which inherently handles cancel mid-write (orphan tmp gets cleaned up)

A.3 §S15 ID 生成:
  - ID generation calls: none — uses `os.CreateTemp(parent, ".forgify-write-*")` which generates a temp name via stdlib, not a business ID
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: site #1 LOW — `errors.New("content field is required ...")` at line 135 has no sentinel and no pkg.method prefix; works as innermost-layer bare error but inconsistent with sibling sentinels
  - sites verified: #2 (json.Unmarshal Errorf canonical)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: file uses ErrEmptyFilePath / ErrPathNotAbsolute (reused from read.go) — no new sentinels declared in write.go itself
  - 已登记 errmap: N/A
  - missing: N/A — same tool-validation framework intercept as read.go (§S18 Tool framework converts ValidateInput errors to friendly tool_result strings)
