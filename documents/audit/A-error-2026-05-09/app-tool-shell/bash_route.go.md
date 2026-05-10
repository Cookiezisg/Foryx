# audit: backend/internal/app/tool/shell/bash_route.go

LOC: 382
Read: full file (lines 1-382)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | bash_route.go:88-91 | `file, err := syntax.NewParser().Parse(strings.NewReader(command), ""); if err != nil { return detectRuntimeFirstToken(command) }` | A.1 | OK | Parser failure → fallback to first-token regex. Documented intent at lines 78-82 explicitly: "parser 拒绝输入（罕见）时 fallback 到 first-token regex". This is a defensive degradation not silence — output is still produced and the auto-route path remains correct. §S3 carve-out for "documented soft-degrade with rationale". | N-A | — | — | — |
| 2 | bash_route.go:93-107 | `syntax.Walk(file, func(node syntax.Node) bool { ... })` | A.1 | OK | Walk callback returns bool indicating descent — no err handling in this callback shape. Walk itself doesn't return error. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (file is pure-function classifier — no error returns from any function. detectRuntime returns string-or-empty, classifyCallExpr returns string, all helpers are string→string)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - ctx 来源: N/A
  - violations: N/A — pure parsing logic, no DB / fs / ctx involvement

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — pure function package, no ID generation

A.4 §S16 错误 wrap 格式:
  - violations: not present — file has zero `fmt.Errorf` / `errors.New` calls. All public functions return string-or-empty rather than error.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (file is pure pattern-matching logic with no sentinels)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
