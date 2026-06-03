# Round 0006 — pkg/jsonrepair（波次 0 · M0.1 续）

类型 / 目标：迁移 `jsonrepair`（LLM 畸形 JSON 的 best-effort 修复）—— 实现原样保留 + **补全缺失的测试**。

依赖扫描：
- 上游：`bytes`/`encoding/json`/`strings`（stdlib）。零上层依赖。
- 下游：`function`/`handler`/`workflow` 的 `apply.go`（M3.1/M3.2/M4.1，AI 生成实体定义 JSON）+ `tool/tool.go`（M2.1，工具调用参数）。

旧实现历史包袱：**无**。实现质量高（带 deepseek 失败率实证作设计动机，符合 S11 Why）。唯一缺陷：**整个包没有 `_test.go`**。

修改后完整逻辑（给人看的）：
- `Repair(s)` 3-pass 漏斗：① `json.Valid` 已合法 → 原样；② `escapeControlChars` 转义字符串内裸控制字符；③ `balanceBrackets` 补缺失闭合括号；④ 两者组合作用于原串；都失败 → 返原串让调用方报真错。
- `escapeControlChars`：逐字节状态机（inString/escaped），字符串内 `0x00-0x1F` → `\n`/`\r`/`\t`/`\u00XX`；字符串外不动。
- `balanceBrackets`：栈数非字符串内 `{[`，末尾逆序补 `}]`。
- `RepairBytes`：[]byte 变体。
- 克制：不是完整 JSON 解析器，只修实测两类模式。

删除 / 移出：无（零移出）。

契约变更：无对外 API。`Repair`/`RepairBytes` 是内部契约（4 下游），签名不动。

新测试（补 10 个 unit）：空串 / 已合法(fast path) / 字符串内裸换行转义 / tab+CR 转义 / 缺 `}` 补齐 / 缺 `]` 补齐 / 控制字符+缺括号组合 / 不可修复(`{"a":}`)原串退回 / 已转义 `\t` 不重复转义(fast path) / `RepairBytes`。

验证：`gofmt -w`（把 doc-comment 列表缩进 3→2 空格归一，原 backend 未过最新 gofmt）；`go build -o /dev/null ./...` OK；`go vet` OK；`go test -v` 10/10 PASS。

是否更干净：实现原样（高质量无须动）+ 补齐测试覆盖 + gofmt 归一。**"保留 + 补测试"范本**（对照 tokencount 保留+自带测试、pathguard 搬+清、userpath 判删）。

覆盖状态：jsonrepair 标 cleaned（含测试）。

下一步：`limits` 考古（M0.1 最后一个纯工具）→ 判定 `modelcaps`/`modelcatalog` 残留 → M0.2 `infra/db`。
