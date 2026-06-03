# 目标架构判据（删除 / 保留标准）

> 这是判定"一块代码该不该存在"的**唯一尺子**。
> 判定标准不是"技术上是否可达（有没有人注册/调用）"，而是"**是否属于目标架构**"。
> 不在目标架构里的东西，哪怕现在被注册、被调用，也是死的——它存在只为兼容已不存在的旧设计。
>
> 事实源：`docs/concepts/architecture.md` 核心能力清单 + 项目根 `CLAUDE.md` 设计原则。

---

## 白名单（属于目标架构 = 保留并重写干净）

1. **Quadrinity 四实体**：Function（无状态）/ Handler（有状态）/ Agent（智能体）/ Workflow。
2. **5 核心节点**：Trigger / Agent / Tool / Case / Approval。**只有这 5 种节点类型。**
3. **Durable Execution**：journal（`flowrun_events`）+ 确定性重放 interpreter。崩溃恢复是魂。
4. **无 RAG 的知识库**：document 树 + LLM-ranked attach（XML 注入），充分利用本地大窗口。
5. **MCP 集成**：原生 Model Context Protocol。
6. **自动化调度**：cron / fsnotify / webhook 物理材化触发（polling 待定）。
7. **基础对话**：apikey / model / conversation / chat + loop（共享 ReAct 引擎）。
8. **Memory**：跨对话长期事实库。
9. **意图识别**（Phase 6，规划内但未做）：意图 → 工作流分发。

## 判定规则

- **不在白名单的能力/概念 = 范式残留 = 删**，无论是否被 main.go 注册、是否有调用链。
- 已确认的残留范式（删除，用目标机制重写）：
  - **topo-walk 执行模型** → 被 durable interpreter 取代（`scheduler` 的 state/pause/subdag/retry 旧半 + `LoopDispatcher`）。
  - **独立的 13 节点 dispatcher** → 收敛为 5 节点（function/handler/mcp/skill/llm/http 并入 Tool；loop/parallel/wait/variable 由 interpreter 原生机制取代）。
  - **任何 RAG / vector / embedding / retrieval / 语义检索** → 不存在于图范式。
  - **`pkg/modelcaps` + `infra/store/modelcapoverride`** → 被 `modelcatalog` 取代的旧 model 范式。

## 边界保留（boundary-kept，不是残留，不删）

- **对前端 / testend 的 REST envelope + SSE 事件契约**：可以改（当前有 AI 瞎改的），但**每次改必须 take note 到 `contract-changes.md`**，留给覆盖后做前端兼容。
- **LLM provider wire 格式**（各家 API 请求/响应）：外部现实。
- **generated**：wails embed（`cmd/desktop/embed`）、mise binaries（`infra/sandbox/mise`）、provider mock。
- **安全防护**：permissions / sandbox / SSRF guard / 加密。

## 灰色地带（重写到该模块那一轮再判定，先标记，别提前删）

| 候选 | 嫌疑 | 判定那一轮 |
|---|---|---|
| `app/ask` vs `app/askai` | ask 疑似 askai 旧版残留 | M6 |
| `domain/forge` + `infra/forge` + `pkg/forge` | 是 SSE 锻造基础设施还是旧实体 | M3.7 |
| `domain/todo` + `app/todo` + `tool/todo` | agent TodoWrite 后端，还是旧概念 | M1.11 |
| `infra/store/mcpcalls` + `mcphealth` | mcp 调用记录/健康，5-node 是否需要 | M3.6 |
| `dev_*` handlers（5 个） | dev-only 边界 vs 该删 | M7.2 |
| `answers`/`scenarios`/`prompts`/`capabilities`/`context_stats`/`metrics`/`usage` 端点 | AI 加的信息端点，是否有产品旅程用到 | M7.2 |
| `pkg` 杂项（agentstate/envfix/installprogress/...） | 逐个判用途 | M0.1 |

> 判定结果（保留/删除/合并）写进对应 `contracts/<module>.md`，并在 `STATE.md` 记一笔。
