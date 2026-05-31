# Execution Logs — Per-Entity Tables with Shared Schema Template

**关联**:
- [`00-overview.md`](./00-overview.md) — D22 决策
- [`02-function.md`](./02-function.md) §5.3 / [`03-handler.md`](./03-handler.md) §7.3 — per-entity 表引用本文件
- [`05-execution-plane.md`](./05-execution-plane.md) — flowrun_nodes 表从原 §5.2 迁到本文件 §4.5

**定位**:每类 capability(function / handler / mcp / skill / workflow 节点)每次执行都落一行 execution log,production debug / cost analysis / audit / replay 用。

---

## 1. 决策 D22 摘要

| 选择 | 取舍 |
|---|---|
| **5 张 per-entity 表**(非 unified) | reviewer 看一张表 self-documenting,胜过 unified 表 + JSON attrs implicit |
| **共享 schema 模板** | 字段命名 + 类型在 5 张表完全一致;只 kind-specific 字段各自加 |
| **写入是各 service 责任** | function service Run 后写 function_executions;handler service Call 后写 handler_calls;等 |
| **跨实体查询经 conversation_id / flowrun_id 索引** | 后端 LLM 工具按 kind dispatch 到对应表;不需 SQL VIEW |

跟 D5 (代码不复用) 哲学一致 —— **schema SHAPE 统一,table 各自独立**。

---

## 2. 共享 schema 模板(5 表通用字段)

每张 execution log 表都必须有以下 **16 个通用字段**(命名 + 类型完全一致;GORM `CreatedAt` 自动维护不计入):

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | TEXT PK | per-entity 各自前缀(详 §4 各表) |
| `user_id` | TEXT 索引 | local-user |
| `status` | TEXT CHECK | `ok / failed / cancelled / timeout`(4 值) |
| `triggered_by` | TEXT CHECK | `chat / workflow / http / test`(4 值) |
| `input` | TEXT (JSON) | 调用参数;sensitive 字段经 service 截 / mask |
| `output` | TEXT (JSON, NULL) | ok 时填;failed/cancelled/timeout 时可空 |
| `error_code` | TEXT NULL | failed 时填 sentinel-mapped code(如 FUNCTION_RUN_FAILED) |
| `error_message` | TEXT NULL | failed 时人类可读 |
| `elapsed_ms` | INT | started_at 到 ended_at 毫秒 |
| `started_at` | DATETIME 索引 DESC | 执行开始 |
| `ended_at` | DATETIME | 执行结束(timeout / cancelled 时仍填) |
| `conversation_id` | TEXT NULL 索引 | chat triggered 时填(便利 chat 调用追溯) |
| `message_id` | TEXT NULL | 同上,消息粒度 |
| `tool_call_id` | TEXT NULL | 同上,LLM tool_call 粒度 |
| `flowrun_id` | TEXT NULL 索引 | workflow triggered 时填(便利 run 整体追溯) |
| `flowrun_node_id` | TEXT NULL | 同上,workflow 内节点粒度 |

(+ GORM 自动:`CreatedAt`,所有表都有,概念上不算 schema 决策)

**索引约定**(每表必备):
- `(entity_id, started_at DESC)` — 单实体最近历史(主路径)
- `(conversation_id, message_id)` — chat 追溯
- `(flowrun_id, started_at)` — workflow 追溯
- `(status, started_at DESC)` — failure dashboard

---

## 3. 各 service 写入责任

| Service | 写入触发点 | 写入表 |
|---|---|---|
| `app/function/Service.Run` | sandbox 跑完(或 timeout / error) | `function_executions` |
| `app/handler/Service.Call` | method RPC 完成(或 timeout / instance crash) | `handler_calls` |
| `app/mcp/Service.CallTool` | MCP 子进程 RPC 完成 | `mcp_calls` |
| `app/skill/Service.Execute` | Skill body 执行完(LLM 调用结束) | `skill_executions` |
| `app/scheduler.dispatchNode` | 每节点 dispatcher 完成 | `flowrun_nodes`(本表迁自 spec 05 §5.2) |

**写入时机**:**终态写一次**(对齐 §S9 detached context 模式 — 终态写必须能完成,即使 ctx 被取消)。retry 多次只算一行 execution,`attempts` 字段(仅 flowrun_nodes 有,见 §4.5)记重试次数。

---

## 4. 5 张表 schema 细节

### 4.1 `function_executions`

| 字段 | 类型 | kind-specific 说明 |
|---|---|---|
| (通用 16 字段)| 见 §2 | id 前缀 `fne_<16hex>` |
| `function_id` | TEXT 索引 | FK → functions.id |
| `version_id` | TEXT | 锁哪个 FunctionVersion 跑的 |
| `python_version` | TEXT | 实际 sandbox 用的 Python 版本(env_id 间接对应) |

### 4.2 `handler_calls`

| 字段 | 类型 | 说明 |
|---|---|---|
| (通用 16 字段)| 见 §2 | id 前缀 `hcl_<16hex>` |
| `handler_id` | TEXT 索引 | FK → handlers.id |
| `version_id` | TEXT | 锁版本 |
| `method` | TEXT | 调了哪个 method |
| `instance_id` | TEXT NULL | in-memory instance ID(hdi_xxx);仅运行时有意义 |
| `owner_kind` | TEXT | conversation / flowrun / test / session(caller-context 区分) |
| `owner_id` | TEXT | conversation_id / flowrun_id 等的反射;便利 reviewer 不用 join 看 |

### 4.3 `mcp_calls`

| 字段 | 类型 | 说明 |
|---|---|---|
| (通用 16 字段)| 见 §2 | id 前缀 `mcl_<16hex>` |
| `server_name` | TEXT 索引 | mcp.json 中 server 名 |
| `tool_name` | TEXT | server 暴露的 tool |
| `server_version` | TEXT NULL | server 报告的版本(若有) |

### 4.4 `skill_executions`

| 字段 | 类型 | 说明 |
|---|---|---|
| (通用 16 字段)| 见 §2 | id 前缀 `ske_<16hex>` |
| `skill_name` | TEXT 索引 | skills/<name> |
| `skill_version` | TEXT | SHA256 hash of SKILL.md(变化检测) |
| `fork_depth` | INT | 0 = inline,≥1 = fork mode 嵌套深度 |
| `substitutions` | TEXT (JSON) | `$1 / $ARGUMENTS / $X` 替换的实际值 |

### 4.5 `flowrun_nodes`(从原 05 §5.2 迁入,字段补齐)

| 字段 | 类型 | 说明 |
|---|---|---|
| (通用 16 字段)| 见 §2 | id 前缀 `frn_<16hex>` |
| `flowrun_id` | TEXT 索引 NOT NULL | FK → flowruns.id |
| `node_id` | TEXT | graph 内的 node id(如 "filter_cond")|
| `node_type` | TEXT | function / handler / mcp / skill / llm / condition / loop / parallel / approval / wait / variable |
| `attempts` | INT default 1 | retry 次数(per-node retry policy) |

**索引**:`(flowrun_id, started_at)` 已在通用约定。

**重要**:节点跑 capability 时(function / handler / mcp / skill),**同时写两条**:
- 一条到 `flowrun_nodes`(workflow 视角)
- 一条到对应 entity 表(function_executions / handler_calls / etc.,entity 视角)
- 两条之间通过 `flowrun_node_id` 字段交叉引用(entity 表的 flowrun_node_id 指向 flowrun_nodes 行;flowrun_nodes 不指 entity 表 — entity 视角是 derived)

非 capability 节点(condition / loop / parallel / approval / wait / variable)只写 flowrun_nodes(没对应 entity execution)。

---

## 5. 保留策略

V1 默认每实体保留最近 **200 条**(对齐 flowruns retention §6.7):

| 表 | 保留键 | 保留数 |
|---|---|---|
| function_executions | per function_id | 200 |
| handler_calls | per handler_id | 200 |
| mcp_calls | per (server_name, tool_name) | 200 |
| skill_executions | per skill_name | 200 |
| flowrun_nodes | per flowrun_id | 全保留(随 flowruns retention 自动级联软删) |

后台异步 prune(每实体 finalize 后检查;超过 hard delete 最旧)。

---

## 6. HTTP API

### 6.1 Per-entity 端点(便利查 entity 历史)

```
GET /api/v1/functions/{id}/executions?cursor=&limit=&status=&triggeredBy=&since=
GET /api/v1/handlers/{id}/calls?cursor=&limit=&status=&triggeredBy=&since=
GET /api/v1/mcp-servers/{name}/calls?toolName=&cursor=&limit=...
GET /api/v1/skills/{name}/executions?cursor=&limit=...
GET /api/v1/flowruns/{id}/nodes?cursor=&limit=  (per spec 05 §7,已有)
```

### 6.2 全局详情端点

```
GET /api/v1/executions/{id}?kind=function|handler|mcp|skill|flowrun_node
```

后端按 `kind` query 参 dispatch 到对应表查;返完整 record(input/output 不截)。

### 6.3 跨实体查询(by conversation / flowrun)

```
GET /api/v1/conversations/{convId}/executions   返该 conv 触发的所有 executions(5 表 UNION)
GET /api/v1/flowruns/{runId}/executions         返该 run 内所有 atomic executions(跨 entity)
```

后端 UNION 5 表(每表 SELECT * WHERE conversation_id=? 或 flowrun_id=?),按 started_at 排序。

---

## 7. LLM 诊断工具 — 5 套 per-entity(共 10 个)

跟 D22 5 张表对应,**每域 search + get 各一**。**不走 unified dispatcher** — 每个工具在对应 `app/tool/<kind>/` 实现,跟该域其他工具同包。LLM 拿到 execution id 时已知 kind(从 search 返回),get_<kind>_execution 无需 kind 参数。

### 7.1 通用签名模板(各域各自实例化)

```typescript
// 搜
search_<kind>_executions({
  // 通用 filter(所有 search 工具都有)
  status?: "ok" | "failed" | "cancelled" | "timeout",
  conversationId?: string,
  flowrunId?: string,
  since?: string,           // ISO8601
  until?: string,
  limit?: number = 50,      // max 200
  cursor?: string,
  // kind-specific filter — 见 §7.2 每域差异
  ...
}) → {
  count: number,
  executions: [{
    id, started_at, status, elapsed_ms,
    input_preview,          // 截 200B
    output_preview,         // 截 200B
    error_message?,
    // kind-specific 字段(handler.method, mcp.server_name, etc.)
  }],
  nextCursor?: string,
  aggregates: {
    ok_count, failed_count, cancelled_count, timeout_count,
    avg_elapsed_ms, p95_elapsed_ms,
  }
}

// 看
get_<kind>_execution({ id }) → {
  ...all fields...,
  input,                    // 截 4KB(超长标 input_truncated: true)
  output,                   // 截 4KB
  // sensitive Handler config 字段在 service 写入时已 mask
  hints: {
    output_empty: boolean,
    significantly_slower: boolean,
    duplicates_previous_input?: string,
  }
}
```

### 7.2 各域 kind-specific filter

| 工具 | kind-specific search filter | 实施位置 |
|---|---|---|
| `search_function_executions` | `functionId?`, `versionId?` | `app/tool/function/` |
| `get_function_execution` | — | `app/tool/function/` |
| `search_handler_executions` | `handlerId?`, `method?`, `ownerKind?`, `instanceId?` | `app/tool/handler/` |
| `get_handler_execution` | — | `app/tool/handler/` |
| `search_workflow_executions` | `workflowId?`, `nodeType?`(flowrun_nodes 域)| `app/tool/workflow/` |
| `get_workflow_execution` | — | `app/tool/workflow/` |
| `search_mcp_executions` | `serverName?`, `toolName?` | `app/tool/mcp/` |
| `get_mcp_execution` | — | `app/tool/mcp/` |
| `search_skill_executions` | `skillName?`, `forkDepth?` | `app/tool/skill/` |
| `get_skill_execution` | — | `app/tool/skill/` |

**共 10 个 LLM 工具**(5 search + 5 get),分散到 5 个域的 tool 包,跟该域其他 LLM 工具放一起(per spec D5 — 不抽共享 helper)。

### 7.3 `hints` 字段

后端机械检查给 LLM 的快速 signal(详 §8 诊断 use case):

| Hint | 含义 |
|---|---|
| `output_empty: bool` | output 是 null / [] / "" / {} |
| `significantly_slower: bool` | elapsed_ms > 3× entity p50 |
| `duplicates_previous_input: string?` | 同 entity 上次同 input 的 execution id(可对比) |

---

## 8. 诊断 use case 示例

**用户**:"为啥昨天那个 to-pdf function 跑出来 PDF 是空的"

**LLM 流**:

```
1. query_executions({kind:"function", entityId:"fn_to_pdf", 
                     since:"2026-05-10", limit:20})
   → aggregates: ok_count=15, failed_count=0
   → 全部 status=ok 但 output_preview 显示 page_count=0  ←异常!
2. get_execution({id: "fne_xxx", kind:"function"})
   → hints: { output_empty: false, duplicates_previous_input: "fne_yyy" }
   → 看 output 详:`page_count=0`
3. get_execution({id: "fne_yyy", kind:"function"})  ← 之前同 input
   → 也是 page_count=0
4. LLM 回答用户:
   "Function 一直返 page_count=0,问题在代码不在 input。
    建议 edit_function 检查 weasyprint 用法。"
```

**关键**:status=ok 但 output 语义不对的场景**全靠 LLM 看 output_preview / get_execution 自己诊断**(不靠 status flag)。

---

## 9. 敏感字段处理

| 维度 | 处理 |
|---|---|
| 写入时(各 service Run/Call 内) | Handler 调用如经过 init_args sensitive 字段:从 input JSON 剥除 / 替换为 `***`;Function 不接 sensitive,正常写 |
| 读出时(API/LLM 工具) | 不再加额外 mask(写入时已处理) |
| 日志 | zap 字段过滤(不打整 input/output 进日志,只 input_preview 200B) |

---

## 10. Catalog 集成(可选 V1)

Function / Handler catalog source 在生成 catalog item 时,可加 hint 字段:

```
catalog item function "to-pdf":
  description: "Convert markdown to PDF"
  lastExecutionAt: 2026-05-10T14:32:00Z   ← from MAX(started_at) WHERE function_id=...
  recentSuccessRate: 0.93                 ← 最近 200 条 ok/total
```

让 LLM 看 catalog summary 时知道**哪些 capability 活跃 / 哪些没人用 / 哪些最近失败多**,给优先级建议。

**V1 不强求**(catalog 主功能是教 LLM 类目能力,不是分析);加进去成本低(SQL count + max);**先标 V1.5 候选**,看实际用户反馈再决。

---

## 11. ID 前缀清单(对齐 §S15)

新加 4 个前缀:

| 实体 | 前缀 |
|---|---|
| FunctionExecution | `fne_<16hex>` |
| HandlerCall | `hcl_<16hex>` |
| MCPCall | `mcl_<16hex>` |
| SkillExecution | `ske_<16hex>` |
| FlowRunNode(已有,无变)| `frn_<16hex>` |

---

## 12. 错误码

跟现有 errmap 协同 — execution row 的 `error_code` 字段填**已注册的 sentinel 对应 wire code**(如 `FUNCTION_RUN_FAILED` / `HANDLER_INSTANCE_RPC_TIMEOUT` 等),不发明新错误码。

LLM 工具 `query_executions` / `get_execution` 自身错误:

| Code | HTTP | 触发 |
|---|---|---|
| `EXECUTION_NOT_FOUND` | 404 | id 查不到 |
| `EXECUTION_KIND_INVALID` | 400 | kind 不在 4 值 |
| `EXECUTION_QUERY_INVALID` | 400 | 不指定 kind 也不指定 conv/run(防全表扫) |

---

## 13. 实施工作量(per Plan 影响)

| Plan | 加 task | LOC |
|---|---|---|
| Plan 01 (function) | +4 task(domain Execution writer / store 表 / Service.Run write / HTTP) | ~250 |
| Plan 02 (handler) | +4 task(同上,handler_calls) | ~280 |
| Plan 05 (execution plane) | flowrun_nodes 加字段 + 加 mcp_calls / skill_executions writes + delete StatusTimeout bug fix | ~350 |
| Plan 06 (e2e) | +1 task(query_executions / get_execution LLM 工具实施 + e2e 诊断场景) | ~200 |

**总 ~1080 LOC + 13 task**。V1 实施加 ~3 工作日,8 周 → 8.4 周。

---

## 14. V1.5 候补

- **`semantic_status` 字段**(用户 / LLM 标 wrong / correct,training data)
- **compare_executions 工具**(两 execution diff 输入输出)
- **Anomaly detection**(elapsed_ms 突增 / output schema 突变 → notification alert)
- **Catalog `lastExecutionAt` / `recentSuccessRate` 字段**(§10 V1.5)

---

(本文档完)
