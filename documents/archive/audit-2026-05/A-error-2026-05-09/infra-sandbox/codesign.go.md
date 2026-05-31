# audit: backend/internal/infra/sandbox/codesign.go

LOC: 101
Read: full file (lines 1-101)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | codesign.go:60-63 | `cmd := exec.CommandContext(ctx, "xattr", ...); if out, err := cmd.CombinedOutput(); err != nil { return fmt.Errorf("xattr -dr: %w (output: %s)", err, out) }` | A.4 | EDGE | §S16: has `%w` ✓ + descriptive prefix `xattr -dr:` (not canonical `<pkg>.<Method>:`); function-name form for an unexported helper is borderline. Sentinel chain (caller's exec.ExitError) preserved | LOW | minor — error reads "xattr -dr: exit status 1 (output: ...)" descriptive but lacks pkg loc | tighten: `fmt.Errorf("sandbox.macCodesign: xattr -dr: %w (output: %s)", err, out)` | FOUND |
| 2 | codesign.go:76-78 | `err := filepath.WalkDir(root, func(path, d, err error) error { if err != nil { return err } ... })` | A.1/A.4 | OK | bare `return err` inside WalkDir callback — propagates filesystem walk error; outer caller line 96-98 re-checks. Error is preserved through callback chain. WalkDir contract expects this pattern. | N-A | — | — | — |
| 3 | codesign.go:82-85 | `info, err := d.Info(); if err != nil { return err }` (in callback) | A.4 | OK | bare return inside WalkDir callback — same pattern as site #2; outer caller wraps via WalkDir contract | N-A | — | — | — |
| 4 | codesign.go:89-92 | `signCmd := exec.CommandContext(ctx, "codesign", ...); if out, signErr := signCmd.CombinedOutput(); signErr != nil { return fmt.Errorf("codesign %s: %w (output: %s)", path, signErr, out) }` | A.4 | EDGE | §S16: has `%w` ✓; prefix is `codesign %s:` (command name + path), descriptive but missing pkg.Method qualifier. Caller can grep "codesign" but loses sandbox.macCodesign loc. | LOW | identical UX (error propagates with output context) | tighten: `fmt.Errorf("sandbox.macCodesign: codesign %s: %w (output: %s)", path, signErr, out)` | FOUND |
| 5 | codesign.go:96-98 | `if err != nil { return err }` (after filepath.WalkDir return) | A.4 | EDGE | bare return — WalkDir's err is the callback's accumulated error, which sites #1/#4 already have wrapped with `xattr -dr:` / `codesign %s:` prefixes. So sentinel preserved + caller has command-name context, just missing pkg.Method. | LOW | minor — caller (bootstrap_mise.go ExtractMiseBinary) wraps further so loc is recovered | wrap: `return fmt.Errorf("sandbox.macCodesign: walk: %w", err)` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (all error returns propagate; WalkDir callbacks correctly bubble)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs filesystem operations + spawns codesign/xattr; no DB writes, no terminal-state operations

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: sites #1, #4, #5 (LOW — descriptive prefixes `xattr -dr:` / `codesign %s:` / bare-after-WalkDir; all preserve sentinel chain via %w but missing canonical `sandbox.macCodesign:` pkg.Method qualifier)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (uses stdlib exec / filesystem errors only)
