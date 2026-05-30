# Recommendations — 设计级建议(给人,不自动改)

_空。每条:date · 面/工具 · 建议 · 证据 · 状态_

- **[2026-05-30] run_model / 后端 exec** — 执行/accept function 前对 code 做 unescape(`\"`→`"`),或强制 run_function 兜——over-escape 是 JSON 合法的,G1 括号修复治不了它  _(证据: round 0001: 2/13 exec SyntaxError at `\"\"\"`; 状态: open)_