# 00 — Hard-Code 限制优化(总览 + 已定原则 + 分类)

> **定位**:全后端 hardcode 上下限/超时/截断的一次系统优化。本篇是"为什么 + 原则 + 分类总览",一屏看完。
> 逐项裁决与业界依据见 [`01-optimize-decisions.md`](./01-optimize-decisions.md);前端"高级能力"设置区设计见 [`02-advanced-settings-ui.md`](./02-advanced-settings-ui.md);分阶段实施清单见 [`03-implementation-plan.md`](./03-implementation-plan.md)。
> **缘起**:对 436 个非测试 `.go` 文件逐文件通读(非 grep)审出 ~150 处 hardcode;用户拍板优化方向。**调研日期 2026-05-30**。
> **状态**:📐 设计定稿,未动工。

---

## §0 一句话 + 已定原则

**净结论**:这些"离谱"的限制根因只有三类(见 §1)——大多不是"数字选错",是更深的设计问题(谎报成功 / 把 SaaS 紧箍咒套在本地单用户 / 有真实信号不用)。**Forgify 是本地单用户桌面 app:用户自付 token、用自己的 key、能随时按 Esc**——这是判定"该不该留"的核心尺子。

**已拍板的 5 条原则(2026-05-30):**

1. **统一原则**:`高 ceiling + 诚实失败态 + 用户可中断 + 真实信号驱动`。撞限不许谎报成功;能驱动行为的优雅杠杆(预算/idle 信号/真实窗口)都便宜。
2. **唯一例外**:**无人值守的 workflow agent 节点**保留真预算(turn / token / cost 上限)——因为没有人能按 Esc 救它。这是唯一一处 SaaS 式 cap 仍然 load-bearing。
3. **配置化落点**:可调项**存** `settings.json`(后端),但**调节入口是前端 Settings 界面底部新增的「高级能力」区**——不暴露裸文件给用户编辑。详 [`02`](./02-advanced-settings-ui.md)。
4. **范围**:全部做完(P0 → P3)。
5. **流程**:先落成本系列正式文档,再动工。

> **超时哲学(讨论后细化)**:超时**不用于限"健康的慢活"**——in-progress 工作一律靠 **ctx 端到端 + 用户 stop** 中止;只保留 ① LLM idle **死连接探测网** ② 探针 fail-fast ③ 把控制权还给 agent 的工具超时(Bash / mcp call,可配高默认)。成本护栏 = agent `maxTurns`(无人值守 workflow 节点保留)。

---

## §1 三个根因(给所有"该优化"项一个统一解释)

| 根因 | 本质 | 典型 |
|---|---|---|
| **A. 不诚实的静默终止** | 撞到上限时**假装成功 / 静默截断**,不大声暴露、不给"取回/继续"的手柄 | ReAct 撞顶写 `completed`+`max_tokens`(在撒谎);模型自己 hit max_tokens 不提示;tool 结果被砍且取不回 |
| **B. 本地单用户套 SaaS 紧箍咒** | 紧 cap 是为多租户控成本/防滥用/护共享资源——本地单用户全部失效 | `maxSteps=20`、`max_tokens` 砍小、紧超时、`search` 只回 3/5 |
| **C. 有真实信号不用,拍脑袋写死** | 明明有真实可用信号(modelcaps 真窗口/真输出、token 预算、idle 信号、各家 live capability API),却 hardcode 兜底 | 兜底窗口 `32768`、输出 `8192`、历史 `200` 条、LLM `120s` 墙钟 |

**业界(2024–2026)与仓库自有文档高度印证**(详见 [`01`](./01-optimize-decisions.md) 的引用):
- Claude Agent SDK / Claude Code 本身**无 maxSteps 硬顶**,靠 token 预算 + 输出 cap 升级重试 + 用户中断收敛;撞限返回**独立错误态**(`error_max_turns`),从不返回"成功"。LangGraph(`GraphRecursionError`)、OpenAI Agents SDK(`MaxTurnsExceeded`)同理。
- 官方 SDK 的 LLM 超时是 **600s**,Anthropic 还**按 `max_tokens` 缩放、设 TCP keepalive**,不用固定墙钟。Go `http.Client.Timeout` 覆盖 body 读取,对流式是**公认反模式**。
- 仓库自有 `claude-code-research-documents/01-agent-loop.md §6` 与 `final_sweep/final-sweep.md §1` **早就把 `maxSteps=20` / `maxHistoryMessages=200` 标为"该被 token 预算 + compaction 取代"的设计债**——只是没做完。

---

## §2 三桶分类总览

判据:**产品理念 → 留;技术/安全必需 → 留;其余(为省事/抄 SaaS/拍脑袋写死的上下限)→ 优化**。下面两桶("留")在此**完整登记备查**;优化桶的逐项裁决在 [`01`](./01-optimize-decisions.md)。

### 🟢 桶 1 — 保留:产品理念 / 语义

| 项 | 位置 | 为什么留 |
|---|---|---|
| Subagent 不能嵌套(`depth ≥1` 禁止)+ 子 agent 剥离 workflow/subagent 工具 | `subagent/agent.go:109`、`subagent/subagent.go:65` | 员工不指挥员工,防递归 |
| SSE 只有 3 条流 | — | 架构铁律(E1) |
| 全部封闭枚举:block/message/relation(8/7)/memory(4/2)/scenario(3)/mcp/mention 等 | 各 `domain/*` | 协议/产品语义,扩张先改协议文档(E2) |
| workflow 串行并发=1、pseudo-terminal/self-loop/dup 节点拒绝、approval 决策枚举 | `scheduler/scheduler.go:129`、`workflow/validate.go` | DAG 正确性 + 产品语义 |
| SSRF scheme/host/IP 拦截、pathguard ~30 deny、private `_` 方法不可调、用户名正则、last-user 删除守卫 | `scheduler/dispatch_http.go`、`pkg/pathguard`、`user/user.go` | 安全 / 产品规则 |
| loop 默认并发 `5`、容器嵌套 `≤3` | `scheduler/dispatch_loop_parallel.go:24`、`workflow/validate.go:57` | 产品安全默认(防"千次调用爆炸");用户可覆盖 |

### 🔧 桶 2 — 保留:技术 / 安全必需

| 项 | 位置 | 为什么留 |
|---|---|---|
| crypto:AES key 32B、nonce 12B、`v1:` 前缀、salt | `infra/crypto/aesgcm.go` | AES-256 硬性要求 |
| idgen 8 字节(16 hex) | `pkg/idgen/idgen.go:20` | 碰撞概率工程取舍 |
| SQLite pragmas:busy_timeout 5000 / WAL / FK on / in-mem `MaxOpenConns=1` | `infra/db/db.go` | 驱动/并发必需 |
| 外部/导入入口 DoS·OOM 防护:webhook 10MB、skill·mcp 导入 1MB、HTTP 节点响应 10MB、附件 50MB、resources 下载 100MB | 各 handler / `infra/trigger/webhook` | 机制必需(**值可在「高级能力」放开**) |
| file perms 0644/0755、anthropic-version header、document 祖先链 walk 10000 / 重名重试 100 | 多处 | 标准 / runaway 防护 |
| 探针类短超时:mcp init 30s / AddServer 3min / HealthCheck 10s、apikey test 10s、HTTP server Read 15s / Idle 60s / WriteTimeout 0(为 SSE 关) | `app/mcp/mcp.go`、`apikey/tester.go`、`cmd/server/main.go:650` | 探针就该 fail-fast;`WriteTimeout=0` 为流式正确 |
| python/sandbox 执行超时范式(`opts.Timeout` 可配,`0`=无) | `app/sandbox/spawn.go:29` | **已是优雅范式,其它该向它看齐** |

### 🔴 桶 3 — 优化(详 [`01`](./01-optimize-decisions.md))

5 个主题:**① 循环边界 · ② 输出 token · ③ context/历史 · ④ 工具结果截断 · ⑤ 超时**。外加 5 个顺带挖出的**真 bug**(无论怎么决策都该修):
1. 循环撞顶谎报 `completed`(`loop/loop.go:182`)
2. `truncateJSON` 吐非法 JSON(7 处调用)
3. SSE 单行 `64KB` → 整条流 abort(`infra/llm/transport.go:34`)
4. Opus 4.7/4.8 手填 thinking budget 会 400(`infra/llm/anthropic.go`)
5. 压缩 nil-resolver 时大模型被按 32K 压缩(`contextmgr/estimate.go:16`)

---

## §3 本系列文档地图

| # | 文档 | 内容 |
|---|---|---|
| 00 | 本篇 | 原则 + 三根因 + 三桶分类总览 + 文档地图 |
| 01 | [`01-optimize-decisions.md`](./01-optimize-decisions.md) | 优化桶**逐项裁决**:现状值/位置 → 裁决 → 目标做法/值 → 业界依据(引用) |
| 02 | [`02-advanced-settings-ui.md`](./02-advanced-settings-ui.md) | 前端「高级能力」设置区设计(settings.json backing + UI + 注入路径) |
| 03 | [`03-implementation-plan.md`](./03-implementation-plan.md) | 分阶段 P0–P3 实施清单(file-level + verification + §S14/§F1 doc-sync) |

---

## §4 状态

- 2026-05-30:审计 + 调研 + 分类 + 原则拍板,本系列 4 篇落稿。
- **2026-05-31:P0–P3 全部实现并提交**（见 `progress-record.md` 2026-05-30/31 `[opt]` 段，commits `52095f6`→`b863935`）。每阶段 `go build`+`staticcheck`+`make mock`（16 包绿）；前端 `make lint`+`make web` 绿。
  - ✅ P0 止血 / P1 诚实失败态 / P2 换机制（idle 超时 + 删节点墙钟 + history 投影）/ 上限抬高+接通 `limits.Current()` / P3 settings.json 配置化 + `GET/PUT /settings/limits` 端点 + 前端「高级能力」区。
  - ⏸ **延后**：Anthropic 4.7/4.8 effort-thinking（需 live key 验 wire format）；live capability overlay（P3.4，独立子工程）；MessageView 徽章美化（诚实态已可见）；前端 contract 细节文档（cross-cutting/fsd-layers/feature-settings）。
