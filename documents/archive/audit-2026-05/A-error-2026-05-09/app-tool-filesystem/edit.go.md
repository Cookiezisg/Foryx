# audit: backend/internal/app/tool/filesystem/edit.go

LOC: 338
Read: full file (lines 1-338)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix |
|---|---|---|---|---|---|---|---|---|
| 1 | edit.go:42-54 | `var ErrEmptyOldString = errors.New("old_string is required and must be non-empty"); ErrEditNoOp = errors.New("old_string and new_string must be different")` | A.5 | OK | 2 tool-validation sentinels. Same §S18 framework intercept pattern — errmap N/A | N-A | — | — |
| 2 | edit.go:144-146 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("Edit.ValidateInput: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — |
| 3 | edit.go:147-152 | bare-return ErrEmptyFilePath / ErrPathNotAbsolute (reused from read.go) | A.4 | OK | §S16 spec OK at innermost layer | N-A | — | — |
| 4 | edit.go:153-155 | `if a.OldString == nil || *a.OldString == "" { return ErrEmptyOldString }` | A.4 | OK | direct sentinel return | N-A | — | — |
| 5 | edit.go:156-158 | `if a.NewString == nil { return errors.New("new_string field is required (use empty string to delete the matched text)") }` | A.4 | EDGE | Same pattern as write.go #1 — bare `errors.New` at validation layer with no sentinel and no pkg.method prefix. LOW per §S16 (innermost-layer bare error works but is inconsistent with sibling sentinels at file scope) | LOW | identical UX; style inconsistency | introduce `ErrNewStringRequired` sentinel for parity with ErrEmptyOldString | — |
| 6 | edit.go:159-161 | `if *a.OldString == *a.NewString { return ErrEditNoOp }` | A.4 | OK | direct sentinel | N-A | — | — |
| 7 | edit.go:219-221 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("Edit.Execute: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — |
| 8 | edit.go:224-226 | `if ok, reason := t.pathGuard.Allow(args.FilePath); !ok { return reason, nil }` | A.1 | OK | §S18 RequiresWorkspace=true ✓ — Edit self-checks via pathGuard | N-A | — | — |
| 9 | edit.go:231-240 | `info, err := os.Stat(cleaned); err handling: NotExist → friendly with "use Write to create"; other err → friendly access msg; IsDir → friendly` | A.1 | OK | os errors → LLM-friendly strings; NotExist hint includes "use Write to create new ones" — defensive UX | N-A | — | — |
| 10 | edit.go:243-246 | must-Read-first: `state, hasState := reqctxpkg.GetAgentState(ctx); if !hasState { return "Cannot verify Read-first guard: agent state missing. Read the file first.", nil }` | A.1 | **OK (POSITIVE EXAMPLE)** | §S18 NeedsReadFirst=true ✓ — Edit refuses on AgentState miss, same defensive discipline as Write site #6. **Best practice** consistent with §S3 anti-silent-fallback | N-A | — | — |
| 11 | edit.go:247-250 | `seenSize, seen := state.WasRead(cleaned); if !seen { return "File must be read first ...", nil }` | A.1 | OK | must-Read-first refusal | N-A | — | — |
| 12 | edit.go:258-263 | `if info.Size() != seenSize { return "File has been modified since last read (current size %d, expected %d) ...", nil }` | A.1 | **OK (POSITIVE EXAMPLE)** | external-modification detection via size comparison. Doc at lines 252-257 documents the v1 trade-off (size-only is best-effort; same-size content swaps not caught; hash check overkill). **Important defensive layer** that prevents Edit from clobbering changes made outside the conversation | N-A | — | — |
| 13 | edit.go:266-269 | `raw, err := os.ReadFile(cleaned); if err != nil { return ... friendly, nil }` | A.1 | OK | os err → LLM-friendly | N-A | — | — |
| 14 | edit.go:273-282 | `occurrences := strings.Count(content, args.OldString); switch: 0 → "not found"; >1 && !replace_all → "found N matches" hint` | A.1/A.4 | OK | Replacement counting / friendly hints — no error to swallow | N-A | — | — |
| 15 | edit.go:288-295 | `if args.ReplaceAll { newContent = strings.ReplaceAll(...); replaced = occurrences } else { newContent = strings.Replace(..., 1); replaced = 1 }` | — | OK | trusts stdlib replacement semantics (decision D1 documented at lines 5-10) | N-A | — | — |
| 16 | edit.go:298-302 | `tmpFile, err := os.CreateTemp(parent, ".forgify-edit-*"); if err != nil { return ... friendly, nil }` | A.1 | OK | same pattern as Write site #8 | N-A | — | — |
| 17 | edit.go:304 | `cleanup := func() { _ = os.Remove(tmpPath) }` | A.1 | OK | same §S3 carve-out as Write site #9 — best-effort cleanup of orphan tmp | N-A | — | — |
| 18 | edit.go:306-310 | `if _, err := tmpFile.WriteString(newContent); err != nil { _ = tmpFile.Close(); cleanup(); return ... }` | A.1 | EDGE | line 307 `_ = tmpFile.Close()` discards Close err in error-path cleanup. Same LOW as Write site #10 — missing inline ritual comment for the discard | LOW | none — Close-on-failed-write irrelevant to caller | add inline comment for ritual | — |
| 19 | edit.go:311-314 | `if err := tmpFile.Close(); err != nil { cleanup(); return ... friendly, nil }` | A.1 | OK | success-path Close error captured | N-A | — | — |
| 20 | edit.go:315-318 | `if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil { cleanup(); return ... }` | A.1 | OK | preserves source mode (different from Write which has mode-from-existing logic; Edit only overwrites existing so no fresh-create branch) | N-A | — | — |
| 21 | edit.go:319-322 | `if err := os.Rename(tmpPath, cleaned); err != nil { cleanup(); return ... }` | A.1 | OK | atomic rename — same pattern as Write site #13 | N-A | — | — |
| 22 | edit.go:328 | `state.MarkRead(cleaned, int64(len(newContent)))` (state already verified non-nil at site #10) | A.1 | OK | guaranteed non-nil because Guard 3 (line 244) refuses if hasState==false. Different from Read/Write where state miss is a graceful degrade — here it's a hard refusal | N-A | — | — |
| 23 | edit.go:339 | `var _ toolapp.Tool = (*Edit)(nil)` | — | OK | compile-time check | N-A | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site #10 + #12 are **positive examples** of defensive guard discipline — reject on missing state / size mismatch rather than silent fallback; site #18 has `_ = tmpFile.Close()` cleanup-after-failure missing inline ritual comment (LOW)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: file content rewrite is "terminal-on-disk" but not a DB write; AgentState.MarkRead at site #22 is in-memory per-conv (not DB)
  - 各自 ctx 来源: N/A
  - violations: N/A — package doesn't do DB terminal writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: site #5 LOW — bare `errors.New("new_string field is required ...")` no sentinel; same pattern as write.go site #1
  - sites verified: #2 / #7 (json.Unmarshal canonical); #3 / #4 / #6 (bare-sentinel returns at validation deepest layer)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyOldString, ErrEditNoOp (2 in this file)
  - 已登记 errmap: none
  - missing: N/A — tool-validation sentinels caught by §S18 Tool framework before reaching errmap (same as read.go site #1)
