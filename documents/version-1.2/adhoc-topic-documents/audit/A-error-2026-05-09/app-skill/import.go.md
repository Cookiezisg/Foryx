# audit: backend/internal/app/skill/import.go

LOC: 149
Read: full file (lines 1-149)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | import.go:84-89 | `if err := validateName(f.Name); err != nil { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: err.Error()}); continue }` | A.1/A.4 | OK | per-file errors are accumulated into result.Errors and surfaced to UI as inline-error rows (per file header §S3 design rationale). NOT a §S3 silent path — every error gets an explicit row. The `err.Error()` consumed as Reason is the canonical UX path (UI displays the message); not propagating Go error chain because this is per-row UX, not error-chain-up. | N-A | — | — | — |
| 2 | import.go:90-96 | `if len(f.RawSkillMD) > skilldomain.MaxBodyBytes { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: fmt.Sprintf("body %d bytes exceeds %d cap", ...)}); continue }` | A.1/A.4 | OK | same per-file UX pattern as #1. Reason is user-facing message, not propagated error. | N-A | — | — | — |
| 3 | import.go:97-103 | `yamlPart, body, err := splitFrontmatter(f.RawSkillMD); if err != nil { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: "split frontmatter: " + err.Error()}); continue }` | A.1/A.4 | OK | per-file UX pattern. Reason concatenated for display only. | N-A | — | — | — |
| 4 | import.go:104-110 | `var fm skilldomain.Frontmatter; if err := yaml.Unmarshal(yamlPart, &fm); err != nil { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: "yaml parse: " + err.Error()}); continue }` | A.1/A.4 | OK | same as #3. | N-A | — | — | — |
| 5 | import.go:111-116 | `if err := validateFrontmatter(fm); err != nil { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: err.Error()}); continue }` | A.1/A.4 | OK | same UX pattern; validateFrontmatter wraps with sentinel + context for diagnostic, str representation is what UI shows. | N-A | — | — | — |
| 6 | import.go:118-127 | `if _, err := os.Stat(dir); err == nil { exists = true } else if !errors.Is(err, fs.ErrNotExist) { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: "stat: " + err.Error()}); continue }` | A.1 | OK | ENOENT silently treated as "not yet existing" (correct); other errors caught + surfaced as per-row error. | N-A | — | — | — |
| 7 | import.go:128-131 | `if exists && !overwrite { res.Conflicts = append(res.Conflicts, f.Name); continue }` | A.1 | OK | skip-on-conflict is documented behavior; UI sees Conflicts list and prompts user. NOT silent. | N-A | — | — | — |
| 8 | import.go:133-138 | `if err := writeSkillDir(dir, fm, string(body)); err != nil { res.Errors = append(res.Errors, ImportError{Name: f.Name, Reason: "write: " + err.Error()}); continue }` | A.1/A.4 | OK | same per-row error UX. | N-A | — | — | — |
| 9 | import.go:142-146 | `if len(res.Imported) > 0 { if err := s.Scan(ctx); err != nil { return res, fmt.Errorf("skillapp.Import: post-batch rescan: %w", err) } }` | A.4 | OK | §S16 canonical. Note: returns res WITH the error — caller can still see per-file outcomes even if rescan fails. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (every per-file error is captured in result.Errors with explicit reason; not silent)
  - **architectural observation**: this file is the model `§S3 + per-row UX` example for the package — every err goes into ImportResult.Errors with the file's name, NEVER silently dropped. UI gets per-row outcome rendering for free.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site #9 (post-batch Scan)
  - 各自 ctx 来源: request ctx
  - violations: N/A — same reasoning as mutate.go: disk file is source of truth, cache rebuild is signal to in-memory index. Cancel after writes complete = files persist + 1s poll picks up.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — skill names supplied by user

A.4 §S16 错误 wrap 格式:
  - violations: not present (per-file errors are UX-text not propagated; the one Go-error return at site #9 is canonical wrap)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none
  - 已登记 errmap (consumed indirectly via validateName / validateFrontmatter / Scan): all skilldomain sentinels registered ✓
  - missing: N/A — file defines no sentinels
