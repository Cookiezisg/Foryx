# audit: backend/internal/app/tool/shell/output.go

LOC: 181
Read: full file (lines 1-181)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | output.go:27 | `ErrEmptyBashID = errors.New("bash_id is required")` | A.5 | OK | ValidateInput sentinel; consumed by Tool framework (§S18) — converted to tool_result string before reaching handler/errmap. | N-A | — | — | — |
| 2 | output.go:81-83 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("BashOutput.ValidateInput: %w", err) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w | N-A | — | — | — |
| 3 | output.go:84-86 | `if strings.TrimSpace(a.BashID) == "" { return ErrEmptyBashID }` | A.4 | OK | direct sentinel return | N-A | — | — | — |
| 4 | output.go:87-91 | `if a.Filter != "" { if _, err := regexp.Compile(a.Filter); err != nil { return fmt.Errorf("BashOutput.ValidateInput: filter regex: %w", err) } }` | A.4 | OK | §S16 canonical with sub-tag | N-A | — | — | — |
| 5 | output.go:108-110 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("BashOutput.Execute: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 6 | output.go:112-115 | `proc, err := t.mgr.Get(args.BashID); if err != nil { return fmt.Sprintf("Background shell process not found: %s", args.BashID), nil }` | A.1/A.4 | OK | ErrProcessNotFound → friendly tool_result string for LLM. §S18 carve-out: failures convert to tool_result. The string is constructed to match the user-facing pattern documented in outputDescription line 39. | N-A | — | — | — |
| 7 | output.go:120-125 | `if args.Filter != "" { re := regexp.MustCompile(args.Filter); body = filterLines(body, re) }` | A.1 | OK | MustCompile is safe per inline comment lines 121-122: "Validated in ValidateInput; safe to MustCompile here." | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - ctx 来源: site #5 takes context.Context but doesn't use it (Execute doesn't propagate ctx — Get is in-memory)
  - violations: N/A — no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none in output.go (manager.go owns ID generation)
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: not present

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyBashID (line 27)
  - 已登记 errmap: N/A
  - missing: N/A — ValidateInput sentinel per §S18, never reaches errmap
