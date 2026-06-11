# findings —— 评审中发现的偏差（不合理 / 冗余 / 产品问题）

> docswriter 真正的产出。一条 `F-N`：模块 · 类型 · 描述 · 对照的标准 · 建议修法（**标准化、不打补丁**）· 严重度 · 状态（open / 待裁 / 已修 / 转ADR / wontfix）。
> 流程：我列 → **用户裁决修哪些、怎么修** → 修 + 文档 → 下一模块。修法默认走"统一到标准"，不加特例。

| 类型 | 含义 |
|---|---|
| 不合理 | 设计讲不通 / 海拔错 / 该是 A 却 B |
| 冗余 | 同一概念两处实现 / 重复样板（标准>冗余：统一掉） |
| 产品 | 功能本身建模错 / 缺失 / 不一致 |
| 真bug | 代码确实错 |

---

## F-1 错误构造分裂（std vs errorsdomain）→ 全量统一 ✅ 已修

- **原现状**：errorsdomain.New 只用于"会冒泡 HTTP"的 domain 错误；tool 错误（todo/shell/web/filesystem/search/toolset）+ pkg/infra 原语用 std `errors.New`。"按出口分情况"是认知税（todo 为此写 9 行注释、易踩坑）+ 脆弱（一加写端点就漏成 500）。
- **裁决（用户 2026-06-11）**：真正全量统一。把错误**类型**从 `domain/errors` 移到 `pkg/errors`（纯机制、全层可用、无反向依赖）；**所有命名 sentinel 一律 `errorspkg.New`**——无"是否冒泡 HTTP"之分（出口不同：HTTP 读 Kind/Code、LLM tool 读 Message）。
- **已做**：类型搬迁（39 文件重命名 `errorsdomain`→`errorspkg`）+ 37 个命名 sentinel（tool 22 + pkg/infra 15）全转 `errorspkg.New`；web 的 auth/ratelimit/upstream 配真 HTTP 语义（502/429）。build + 全测试绿。
- **状态**：**已修**（type relocation + 全 sentinel；ADR `decisions/0002`）。

## F-2 websearch 错误无 Kind → 随 F-1 一并修 ✅

- **原现状**：`app/tool/web` 的 `ErrAuthFailed`/`ErrRateLimited`/`ErrUpstreamHTTP` 是 std error，丢了它们本有的 HTTP 语义。
- **已做**：转为 `errorspkg.New`——`WEBSEARCH_AUTH_FAILED`/`WEBSEARCH_UPSTREAM_HTTP`(KindBadGateway 502)、`WEBSEARCH_RATE_LIMITED`(KindRateLimited 429)。语义编码正确、未来若经 HTTP 冒出即对。
- **状态**：**已修**（随 F-1）。

## F-3 内联 validation 错误重复样板（Phase 3，open）

- **现状**：~22 处内联 `return errors.New("x is required")`（memory/ask/document/filesystem/search/shell 等 tool 的 ValidateInput）——非命名 sentinel，是逐工具重复的输入校验样板。
- **已做（7 agent 并行读码理解业务 + 去重，2026-06-11）**：22 处全转 `errorspkg.New`，**先去重不盲配码**——shell/kill 复用 `ErrEmptyBashID`、search 3 处 "limit must be non-negative" → 1 个 `ErrNegativeLimit`、document 4 处 "id is required" → 1 个 `ErrIDRequired`、memory 3 处共享 `ErrEmpty*`；ask 的 malformed-JSON 对齐全库 `fmt.Errorf("…: %w")` 包裹惯例（**不配码**——它是包裹非 sentinel）；顺带把 `shell.ErrInvalidTimeout`（还在 fmt.Errorf）也转了。
- **故意保留**：`web/fetch.go:163` "stopped after 10 redirects" 是 `http.Client.CheckRedirect` 回调的内部控制流、非面向 LLM 的 sentinel → 留 std `errors.New`（全库唯一一处）。
- **状态**：**✅ 已修**（Phase 3，全量统一彻底收尾）
