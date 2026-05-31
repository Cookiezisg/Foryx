# 09 — Agent Domain

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 00-overview)。

依赖纲领:[`00-overview.md`](./00-overview.md) 的"能力源自 forge"原则。

---

## Agent 升级为 first-class forge 实体

之前 agent 是 workflow 节点的内嵌配置,这跟 function / handler 在 trinity 里的地位**不对称**。修正:

**agent 是 forge 出来的第 4 类实体**,跟 function / handler 平级。**trinity → quadrinity**。

| 现状(改前) | 修正 |
|---|---|
| agent 4 类挂载塞在 workflow 节点 config 里 | agent 是独立 entity,版本管理 + AI 锻造工具 + 试跑 + 历史调用 |
| 每个 workflow agent 节点都从零配 prompt/skill/knowledge/tool | agent 在 entity 上配一次,被多处引用 |
| ❌ AI 没有 create_agent / edit_agent 这种工具 | ✅ 跟 function / handler 同样一套 11 个锻造工具 |

---

## Agent Entity Schema

```go
type Agent struct {
    ID               string        // ag_<16hex>
    UserID           string
    Name             string
    Description      string
    Tags             []string
    ActiveVersionID  string
    NeedsAttention   bool
    AttentionReason  string
    CreatedAt, UpdatedAt time.Time
    // computed(API 返,server-side)
    Pending          *AgentVersion
}

type AgentVersion struct {
    ID                       string   // agv_<16hex>
    AgentID                  string
    Status                   "pending" | "accepted" | "rejected"
    Version                  *int     // 翻 accepted 才有

    // ─── 4 类挂载(从原 02 doc 迁过来) ───
    Prompt          string                       // 整段不拆 system / user,可模板插值
    Skill           *string                      // 1 个 skill name(可空)
    Knowledge       []DocumentRef                // 单挂多个 doc(无 RAG,直接注入)
    Tools           []CallableRef                // forge callable 列表
    OutputSchema    *OutputSchema                // enum / json_schema / free_text
    Model           *ModelRef                    // (apikey, modelId)(可空走 fallback)

    ChangeReason             string
    ForgedInConversationID   string
    CreatedAt, UpdatedAt     time.Time
}
```

**ID 前缀** `ag_` / `agv_` / `agx_`(agent execution)— 对齐 §S15 ID 形状 + trinity 前缀风格(fn_ / fnv_ / fne_,hd_ / hdv_ / hcl_)。

---

## 4 类挂载(全部在 agent entity 上,不在 workflow 节点 config)

跟之前定的语义一致,**只是承载位置从节点 config 改到 agent entity**:

### 1. prompt
一段整体指令(不拆 system / user),可 `{{ payload.* }}` / `{{ ctx.* }}` 模板插值。跑时 payload 来自调用方(workflow agent 节点的输入 / chat / 试跑接口)。

### 2. skill
1 个 skill name(可空)。**预激活**,LLM 不调用 search/activate。skill 作为方法论注入,prompt 仍必填(写"本次任务")。

### 3. knowledge
单挂多个 document(无 RAG,直接注入 context)。size 不预先 cap,运行时暴露(挂的模型不同 context 不同)。

### 4. tools
只挂 **function / handler / mcp** — callable ref 列表(`fn_xxx` / `hd_xxx.method` / `mcp:server/tool`)。
- **不含 `ag_`**:**agent 不能调用 agent**(员工不指挥员工;要别的 agent 是 workflow 编排者的事)。
- **不挂平台内置**(fs / shell / web / ask / memory / todo / subagent)。需要联网 / 操作文件 → forge 出具体 function。
- knowledge(文档)走 `agent.knowledge`、skill 走 `agent.skill`,不在 tools 里。

---

## OutputSchema

| outputSchema | 用途 |
|---|---|
| `enum` | 分类 / 路由(跟 case 节点天然咬合) |
| `json_schema` | 结构化提取 |
| `free_text` | 自由文本 |

无平台默认值(跟 07 doc 的 Mechanism vs Policy 原则一致),AI 编排时按业务拍。

---

## outputSchema 运行时强制

forged agent 跑时,平台**强制**其声明的 outputSchema(声明即 schema-pin,见 13 的 G10)。两层:

- **第一层(可选 / 机会主义)**:provider 原生结构化输出 —— OpenAI `response_format` / Anthropic 强制 tool-use / Gemini `responseSchema` / DeepSeek `strict:true`。**能用就用,但替代不了第二层**。
- **第二层(必建,研究全程靠它)**:agent-run 那薄层做四步 —— JSON-repair(G1)→ 按 outputSchema 校验 → 不合规则回喂带 `next_step` 的结构化错误(G7)→ 重试 ~2 轮(G8:17 → 71 → 88 的 plateau)→ 用尽 = 该 activity 失败(走 07 的失败语义)。

**校验范围**:只对 `enum` / `json_schema` 校验;`free_text` 不校。

**作用层**:只在 **forged-agent run 那层** —— workflow agent 节点 / `run_agent` 试跑 / tool 节点调 agent。通用 `loop.Run` 引擎与 chat 老板**不碰**这套强制。

**默认参数**:agent run 默认 `max_tokens >= 16000`(G2,防截断被误判为不合规)、`thinking` 开(G3,复杂结构化任务别关 thinking)。

**为什么不可怕**:`enum` 输出本是模型强项(~100%),最常见的 **agent → case** 路本就基本可靠;这是接进已验证的第二层 + schema-pin 声明(G10),**不是造大型原生结构化系统**。运行时残留由 N1(agent 真吐合规值)+ case guard 的 fail-to-false(G9)兜。

> 交叉引用 [`13-llm-facing-implementation-guide.md`](./13-llm-facing-implementation-guide.md) / [`14-llm-validation-research-record.md`](./14-llm-validation-research-record.md) 的 G1 / G2 / G3 / G7 / G8 / G10。

---

## Model 配置

`(apikey, modelId)` 二元组。Fallback 链:

```
agent entity.model → workflow 级默认 → 全局 chat scenario 默认
```

(原 02 doc 写"节点级",升级后改成"agent entity 级"——workflow 节点只是引用 agent,model 在 entity 上配)。

---

## AI 锻造工具(11 个,对齐 function / handler)

| 工具 | 用途 |
|---|---|
| `search_agents` | 找名字 / tags / description 含 X 的 agent |
| `get_agent` | 看 ag_xxx 详情(active version 的 prompt / skill / knowledge / tools / model / outputSchema) |
| `get_agent_versions` | 看历史版本 |
| `create_agent` | 造新 agent(初始 ops) |
| `edit_agent` | 改 ag_xxx 的任何字段(ops-based,对齐 edit_workflow 模式) |
| `revert_agent` | 退回前一版 |
| `delete_agent` | 删 ag_xxx |
| `accept_pending_agent` | pending 翻 active |
| `run_agent` | 试跑一次(给 input 看输出 + tokens / 耗时) — 编排前验证 |
| `search_agent_executions` | 看 ag_xxx 历史调用记录 |
| `get_agent_execution` | 看某次调用详情(prompt / 工具调用链 / 输出) |

11 个工具,跟 function 9 / handler 10 完全对称。

---

## Agent 也是 callable

跟 function / handler / mcp 完全对称——**agent 是 callable 的第 4 类**。

tool 节点 callable ref 语法扩展(永远 active version,无 pin):

| Callable | ref 形式 |
|---|---|
| function | `fn_xxx` |
| handler 方法 | `hd_xxx.methodName` |
| mcp 工具 | `mcp:server/tool` |
| **agent** | **`ag_xxx`** ← 新加 |

意味着 workflow 中 **tool 节点也可以直接调 agent**(不必走 agent 节点)。详 [`03-tool-node.md`](./03-tool-node.md)。

⚠️ **重要区分**:这里是 **workflow 编排者(图)** 通过 tool 节点调 agent —— 合法(boss 调 worker)。**但 agent 自己的 `agent.tools` 不能含 `ag_`** —— agent(worker)不能调用另一个 agent(员工不指挥员工)。两者方向相反,别混。

跟 00 总纲 3 "永远 prod" 一致——改 / revert agent,所有引用 workflow 自动跟新 / 跟着回滚。

---

## Workflow 编排时的新流程(AI 视角)

```
用户:"造一个邮件总结 workflow,每天 9 点跑"

AI [决定:需要一个 agent + cron trigger + Slack 推送 tool]

AI 调 create_agent:
  prompt:  "你是邮件总结助手,简洁摘要..."
  skill:   "summarization"
  tools:   [fn_read_inbox, mcp:gmail/list]
  model:   (claude-key, claude-3-5-sonnet)
  → 返 ag_xxx pending v1

AI 调 run_agent(ag_xxx, mockInput) → 试跑,效果 OK

AI 调 accept_pending_agent(ag_xxx) → v1 active

AI 调 create_workflow:
  - trigger 节点(cron "0 9 * * *")
  - agent 节点(agentRef=ag_xxx)            ← 引用刚造的 agent
  - tool 节点(callable=mcp:slack/post)

AI 调 accept_pending workflow → v1 active
AI 调 activate_workflow → 上线

AI:"造好了,每天 9 点会跑。看一下画布?"
```

agent 跟 function / handler 完全等地位,锻造流程一致。

---

## Quadrinity — 整个 forge 体系

| 维度 | function | handler | **agent** | workflow |
|---|---|---|---|---|
| 性质 | 纯函数 | stateful class | **LLM ReAct loop 配置** | 编排 |
| 来源 | 用户/AI forge | 用户/AI forge | **用户/AI forge** | 用户/AI 编排 |
| 版本管理 | ✅ | ✅ | ✅ | ✅ |
| pending → accept | ✅ | ✅ | ✅ | ✅ |
| AI 锻造工具 | 9 个 | 10 个 | **11 个** | 9 个 |
| 试跑 | `:run` | `:call` | `:run` | `:trigger`(指定 triggerNode + payload) |
| 可被 workflow tool 节点引用 | ✅ | ✅ | ✅(新加) | ❌(员工思维,不能调其他 workflow) |
| ID 前缀 | fn_ / fnv_ / fne_ | hd_ / hdv_ / hcl_ | **ag_ / agv_ / agx_** | wf_ / wfv_ / fr_ |

**mcp** 是从 marketplace 装的不算 forge — Forgify 的 forge 体系是**四元**:function / handler / agent / workflow。

> 区分两个 axis(详 [`12`](./12-deep-dive-findings.md) §S2 / B5):这里"**四元**"是**锻造实体分类**(有版本 / pending / accept 的能力实体)。而 **forge SSE 流式通道**另外覆盖 document / skill,共 **6 kind** —— 那是驱动前端 subpage 右栏实时呈现编辑的 **UI 通道**,document/skill 在通道里但**不是**锻造实体。**6 kind(通道)≠ 四元(分类)**,两 axis 不冲突。

---

## 不变的:员工思维 + 禁令

agent entity 的设计约束跟之前 02 doc 一致(只是承载位置变了):

| 禁制 | 体现 |
|---|---|
| 不能 spawn subagent | agent.tools 不含 subagent 工具 |
| 不能调其他 workflow | agent.tools 不含 trigger_workflow 工具 |
| skill 编排时配死 | agent.skill 是 entity 字段,不让 LLM 临场 search/activate |
| tools 不挂平台黑盒 | agent.tools 只允许 **fn / hd / mcp**(forge callable + marketplace mcp)|
| **agent 不能调用 agent** | **agent.tools 不含 `ag_`** —— 员工不指挥员工。要别的 agent 帮忙是 workflow 编排者(boss)的事,不是 agent(worker)自己的事 |
| 不挂 ask / memory / todo / subagent | 同上 |
| knowledge / skill | 通过 agent.knowledge(挂文档)+ agent.skill(挂 1 个 skill)承载,不在 tools 里 |

---

## chat agent vs workflow agent(entity)的产品对照

| | chat agent | workflow agent (entity) |
|---|---|---|
| 角色 | **老板** | **员工** |
| 任务来源 | 用户对话 / 探索 | 程序走到 agent 节点喂给它的输入 / 试跑接口 |
| skill | 自己 search + activate | entity 上配死 |
| tools | 自己挑 + 临场 forge | entity 上配死 |
| subagent | 可 spawn | 不能 |
| 是 forge 实体? | ❌ 主对话直接跑 | ✅ entity 化 |

Forgify narrative:**chat 老板帮你造 / 改 agent → agent entity → workflow 员工反复用**。
