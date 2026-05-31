# Handler — 常驻 Python 服务对象

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景
- [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) — 工具接口形态(本域共用)
- [`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) §D-E — env 模型与 LLM env-fix loop 的当前事实源
- 现状 — 全新 domain,跟现有 mcp / forge 无重叠

---

## 1. 定位 + 与 MCP 边界

Handler 是 Trinity 中**中等粒度**的产物 —— Python 类 + 多 method,Definition + Instance 二层模型。

| 属性 | 值 |
|---|---|
| 状态 | **有**(per-instance 内存对象) |
| 调用频次 | 多 method / 反复调用 |
| Lifetime | caller-owns(详见 §3) |
| 形态 | LLM 写的 Python class |
| 承载 | sandbox v2 Python EnvManager + per-instance long-lived owner |

### 1.1 vs MCP — 决策 D2

| 维度 | MCP | Handler |
|---|---|---|
| 模式 | daemon("装一次跑到删") | instance("起一个用完拆") |
| 来源 | 外部(npm / pypi 包) | 用户 / LLM 锻造 |
| 质量 | 不可控(5000+ 条质量参差) | 我们的 Python 代码规范 |
| 文档 | 各包自己的(参差) | 我们的 description / methodSpec 约束 |
| 协议 | MCP spec 标准 | stdio JSON-RPC(参考 MCP 但代码独立) |
| 失败处理 | health check + auto-restart(daemon 思路) | 销毁 + 下次 call 重 spawn |
| Lifetime 控制 | 装时永久;手动删才停 | caller-owns 自动 |

**协议可参考,代码不复用**。Handler 自己一套 domain / store / Service / LLM tool / HTTP / catalog source。

---

## 2. 二层模型:Definition + Instance

### 2.1 HandlerDefinition(持久化)

锻造的产物。包含:
- `name` / `description` / `tags`
- `code`(Python class 主体)
- `methods`(List[MethodSpec])
- `init_args_schema`(可选,启动时一次性参数)
- `dependencies` / `pythonVersion`

落 DB:`handlers` + `handler_versions` 两表。

### 2.2 HandlerInstance(运行时,不持久化)

一次发起的运行。包含:
- **subprocess**(long-lived `python -u`,Class loaded)— per-instance,**不复用**(state 隔离)
- **sandbox venv 引用** — **per HandlerVersion**(D-redo-8 每 Version 独立 venv,env_id `hdenv_<16hex>` 在 Version 行内独立生成,跟 version_id 1:1 但解耦;销 instance 不影响 venv,销 version 才 destroy env)
- 自有 state(Class 实例的内存属性)
- caller-context(chat conv / FlowRun / test execution)
- in-memory `instanceId`(`hdi_<16hex>`)
- lastUsedAt(idle GC 用)

**不持久化** —— Instance 的存在仅在 Forgify 进程内存里。进程死,所有 Instance 死(对齐 Phase 4 plan paused-state rehydrate;Handler instance 不跨进程重启)。

---

## 3. Caller-owns Lifetime — 决策 D3

详细见 [`00-overview.md`](./00-overview.md) §3 / §决策 D3。

### 3.1 Owner 矩阵

| Caller Context | Owner | Instance Lifetime |
|---|---|---|
| Chat conv | conversation | conv 活着 + 闲置 N 分钟 GC + conv 结束销 |
| FlowRun | run | run 跑多久就活多久 + run 结束(任意终态)即销 |
| Test execution | test | 一次性,跑完即销 |
| 手动 HTTP 调试 | session(power-user) | 显式 acquire / release |

### 3.2 Definition 上不加 lifetime 字段

V1 决定 —— Definition 保持纯净。caller-context 自动判定 lifetime,LLM forge 时心智零负担。详见 D3 trade-off。

### 3.3 In-memory registry 实现

```go
// app/handler/registry.go (草图)
type Owner struct {
    Kind string  // "conversation" | "flowrun" | "test" | "session"
    ID   string
}

type instanceRegistry struct {
    mu        sync.RWMutex
    instances map[Owner]map[string]*Instance  // owner → handlerName → instance
    idleGC    *time.Ticker                    // chat scope only
}

func (r *instanceRegistry) Acquire(owner Owner, def *Definition) (*Instance, error) {
    // 找现有,无则 spawn
}

func (r *instanceRegistry) DestroyAll(owner Owner) {
    // 一个 caller-context 退出时调用
}
```

每 caller-context 维护自己的 in-memory subregistry。Handler Service 提供 hook 给 chat / scheduler / test runner 调:
- chat conv 删 → `registry.DestroyAll(Owner{Kind: "conversation", ID: convID})`
- FlowRun 终态 → `registry.DestroyAll(Owner{Kind: "flowrun", ID: runID})`
- Idle GC tick → 扫所有 chat-scope instance 的 lastUsedAt,超时 destroy
- Process exit → 全部 destroy(cleanup hook)

---

## 4. Op 集合(method-level,**跟 workflow 节点级 ops 一致**)

Handler class 由系统按 ops 拼接生成 — LLM 不写整 class,而是分别提供 imports / init / shutdown / 各 method body。每 op 局部应用,改 1 个 method 不动其他。

| Op | 字段 | 校验 |
|---|---|---|
| `set_meta` | `name?, description?, tags?` | name 非空,partial UNIQUE |
| `set_imports` | `imports: string` | class 顶部 import 语句一段;Python AST 可解析 |
| `set_init` | `init_body: string` | `__init__` body(接 init_args + 初始化状态);AST 可解析,可引用 `self.X` |
| `set_shutdown` | `shutdown_body: string` | `shutdown` body(可选,默认 `pass`);AST 可解析 |
| `set_init_args_schema` | `args: InitArgSpec[]` | 声明启动时一次性参数(每条带 `name / type / required / sensitive / description / default?`)|
| `add_method` | `method: MethodSpec` | name 唯一,**body 必填**,AST 可解析 |
| `update_method` | `name, patch` | method 存在;patch 走 JSON Merge Patch(RFC 7396),可改 args / body / return_schema / streaming / timeout 任意子集 |
| `delete_method` | `name` | method 存在 |
| `set_dependencies` | `deps: string[]` | PEP 508 解析 |
| `set_python_version` | `version: string` | PEP 440 解析 |

**校验时机**:
- 单 op apply 后:本 op 自身合法
- 全部 ops apply 完(final 校验):整 class 拼出来 AST 解析通过 + class 名跟 Handler name 对齐 + 所有 add_method 的 args/return_schema 跟 body 签名对齐(参考 5-A 校验方式)

### 4.1 MethodSpec(含 body)

```json
{
  "name": "query",
  "description": "Execute a SELECT query and return rows",
  "args": [
    {"name": "sql", "type": "string", "required": true, "description": "SQL query"}
  ],
  "returnSchema": {
    "type": "array",
    "items": {"type": "object"}
  },
  "body": "with self.conn.cursor() as cur:\n    cur.execute(sql)\n    return cur.fetchall()",
  "streaming": false,
  "timeout": 30000
}
```

- `body` 是 Python method body 字符串(不含 `def name(...):` 头,系统按 args 自动生成 def 头);**LLM 必须在 add_method 时一并提供 body**
- `streaming: true` 表明 body 内会 `yield`,系统翻译成 progress block delta;`return` 是 final tool_result
- `timeout` 单位 ms,默认 30s
- Private helper(以 `_` 开头的 method)同样走 `add_method`,但**不暴露给 LLM 调用**(call_handler 无法调 `_`-prefix method)

### 4.2 InitArgSpec

```json
{
  "name": "dsn",
  "type": "string",
  "description": "PostgreSQL connection string",
  "required": true,
  "sensitive": true,
  "default": null
}
```

- `sensitive: true` 表明此参数是 secret(API key / password / DSN),走加密存储 + UI 密码框 + 日志过滤(详见 §6.5 Handler Config)

---

## 5. Python class 契约 — 系统按 ops 拼装

LLM **不写整段 class**,而是通过 ops 提供片段。系统拼出最终 class 字符串落 `handler_versions.code` 列。

### 5.1 系统拼装模板

```python
# Auto-assembled by Forgify from ops; do not edit by hand.
{set_imports content}                     # ← op set_imports

class HandlerImpl:
    def __init__(self, **init_args):
        {set_init body}                   # ← op set_init

    def shutdown(self):
        {set_shutdown body or "pass"}     # ← op set_shutdown(可选)

    # ── 以下每个 method 由 add_method op 拼入 ─────
    def query(self, sql):
        """{methodSpec.description}"""
        {methodSpec.body}                  # ← op add_method/update_method 提供

    def insert(self, table, data):
        """..."""
        ...

    def _helper(self, x):                 # ← `_`-prefix private method
        """internal helper, not exposed to LLM"""
        ...
```

### 5.2 LLM 锻造一个 PG-Handler 的 ops 序列示例

```json
[
  {"op":"set_meta", "name":"pg-prod", "description":"Production Postgres connector"},
  {"op":"set_dependencies", "deps":["psycopg2-binary>=2.9"]},
  {"op":"set_python_version", "version":">=3.12"},
  {"op":"set_imports", "imports":"import psycopg2"},
  {"op":"set_init_args_schema", "args":[
    {"name":"dsn","type":"string","required":true,"sensitive":true,"description":"PG connection string"}
  ]},
  {"op":"set_init", "init_body":"self.conn = psycopg2.connect(init_args['dsn'])"},
  {"op":"set_shutdown", "shutdown_body":"self.conn.close()"},
  {"op":"add_method", "method":{
    "name":"query",
    "description":"Execute a SELECT query",
    "args":[{"name":"sql","type":"string","required":true}],
    "returnSchema":{"type":"array","items":{"type":"object"}},
    "body":"with self.conn.cursor() as cur:\n    cur.execute(sql)\n    return cur.fetchall()",
    "streaming":false
  }},
  {"op":"add_method", "method":{
    "name":"insert",
    "description":"Insert one row",
    "args":[
      {"name":"table","type":"string","required":true},
      {"name":"data","type":"object","required":true}
    ],
    "returnSchema":{"type":"object"},
    "body":"...",
    "streaming":false
  }}
]
```

10 个 ops,每个独立可校验、可 streaming emit、可在前端按 op 类型 incremental 渲染(class 主体逐部分长出)。

### 5.3 LLM 改一个 method body 的流程(对比 forge 现状)

```json
edit_handler({
  id: "hd_pg",
  ops: [
    {"op":"update_method","name":"query","patch":{"body":"try:\n    with self.conn.cursor() as cur:\n        cur.execute(sql)\n        return cur.fetchall()\nexcept Exception as e:\n    raise RuntimeError(f'query failed: {e}')"}}
  ],
  changeReason: "add error handling"
})
```

只发 1 个 op,**不动其他 method / imports / init**。Token 节省 + diff 清晰 + 流式呼啦只动那一个 method 卡片。

### 5.4 不允许的代码

- import 任何 Forgify Handler client(D7 — Handler 本身就是 client,Handler 内不再调其他 Handler)
- `__init__` / `shutdown` 之外的 dunder method(`__del__` / `__enter__` 等)— V1 不支持
- 修改 sys.path 越出沙箱
- 阻塞 stdin 之外的 IO 等待 indefinite(挂 instance)

### 5.5 driver 模板(stdio JSON-RPC)

```python
# Auto-generated by Forgify; do not edit.
import sys, json, traceback
sys.path.insert(0, '/sandbox/lib')
from user_handler import HandlerImpl

def respond(payload):
    sys.stdout.write(json.dumps(payload) + "\n")
    sys.stdout.flush()

def main():
    # init
    init_line = sys.stdin.readline()
    init_msg = json.loads(init_line)  # {"type":"init", "args":{...}}
    try:
        handler = HandlerImpl(**init_msg.get("args", {}))
        respond({"type": "ready"})
    except Exception as e:
        respond({"type": "init_error", "error": str(e), "trace": traceback.format_exc()})
        return

    # message loop
    for line in sys.stdin:
        msg = json.loads(line)
        msg_type = msg.get("type")
        if msg_type == "shutdown":
            try: handler.shutdown()
            except Exception: pass
            break
        if msg_type == "call":
            method_name = msg["method"]
            args = msg.get("args", {})
            request_id = msg["id"]
            try:
                method = getattr(handler, method_name)
                result = method(**args)
                # generator 形态 → progress + final
                if hasattr(result, '__iter__') and not isinstance(result, (str, list, dict)):
                    for item in result:
                        if isinstance(item, dict) and "progress" in item:
                            respond({"type":"progress", "id":request_id, "data":item["progress"]})
                        else:
                            final = item
                    respond({"type":"return", "id":request_id, "data":final})
                else:
                    respond({"type":"return", "id":request_id, "data":result})
            except Exception as e:
                respond({"type":"error", "id":request_id, 
                         "error":str(e), "trace":traceback.format_exc()})

if __name__ == "__main__":
    main()
```

---

## 6. 调用流程

```
LLM 调 call_handler({handlerName, method, args})
   ↓
Service.Call(ctx, handlerName, method, args):
   owner := getOwnerFromCtx(ctx)
       // ctx 上的 caller-context kind/id
   instance := registry.Acquire(owner, def)
       // 没有 → spawn:
       //   - EnsureRuntime python (mise install if needed)
       //   - EnsureEnv (per-instance owner = "handler-instance:<owner.kind>:<owner.id>:<handlerName>")
       //   - SpawnLongLived (subprocess, send init message)
       //   - 等待 "ready" 响应
       //   - 注册到 registry
       // emit progress for spawn 阶段
   ↓
   // RPC call
   reqID := newReqID()
   instance.SendStdin({"type":"call", "id":reqID, "method":method, "args":args})
   ↓
   // 接收响应
   loop {
     msg := instance.ReadStdout()
     switch msg.type:
       case "progress": emit progress block delta (parent = tool_call.id)
       case "return": result = msg.data; break loop
       case "error": err = msg.error; break loop
   }
   ↓
   instance.lastUsedAt = now
   return { ok, output, error?, elapsedMs }
```

---

## 6.5 Handler Config — Init Args 运行时机制

Handler `__init__` 经常需要外部参数(DSN / API key / model_path),这些**不是 method 调用时传的**(那是 method args),而是**实例化一次性**用的。本节定义这些参数怎么从用户输入到达 Python `__init__`。

### 6.5.1 用户视角(简单)

每个 Handler 是 dedicated 到一个具体目标的:
- 连 Prod DB → 锻造 `pg-prod` Handler
- 连 Staging DB → 另锻造 `pg-staging`
- 连 Slack → 另锻造 `slack-team-1`

**敏感信息(DSN / API key)用户在 Handler 详情页填一次,后台加密存**;之后 LLM 调用时透明注入,LLM 完全感觉不到 secret 存在。

### 6.5.2 Definition 上声明 schema

LLM 锻造时通过 `set_init_args_schema` op 声明(参见 §4.1):

```json
{
  "init_args_schema": [
    {"name":"dsn", "type":"string", "description":"PG connection string", "required":true, "sensitive":true},
    {"name":"schema", "type":"string", "default":"public", "sensitive":false}
  ]
}
```

### 6.5.3 Handler Config 是 per-user 一份(per-Definition 一份)

DB:`handlers` 表加 `config_encrypted TEXT` 字段(整体 AES-GCM 加密 JSON;复用 `infra/crypto.AESGCMEncryptor` + `v1:` 前缀 + machine fingerprint key derivation,与 apikey domain 同模式)。

```
handlers 行:
  id: hd_pg_prod
  name: pg-prod
  ...
  config_encrypted: <encrypted JSON{"dsn":"postgresql://...","schema":"public"}>
```

### 6.5.4 UI 配置流(用户主动)

进 Handler 详情页 → "Configure" 按钮 → 表单按 init_args_schema 渲染(sensitive=true 走密码框)→ 填好 PATCH 上去 → 后端加密存。

### 6.5.5 LLM-driven 流(用户没提前配的情况)

```
LLM call_handler({pg-prod, query, ...}) ← 第一次
   ↓
System 想 spawn instance → 读 config → 缺 dsn
   ↓
返 HANDLER_CONFIG_INCOMPLETE 422 + body: {missing: ["dsn"]}
   ↓
LLM 收到 → 用 AskUserQuestion 工具问用户:"用 pg-prod 需要 DSN,请提供"
   ↓
用户回答 → LLM 调 update_handler_config({id, partial:{dsn:"..."}})
   ↓
LLM 重试 call_handler → 这次 spawn 成功
```

### 6.5.6 Spawn 时(系统侧)

```go
spawn(handlerID, ownerCtx):
  // 1. 解密 config
  configJSON := decrypt(handlers.config_encrypted)
  config := json.Unmarshal(configJSON)
  
  // 2. 校验
  validateAgainstSchema(config, def.init_args_schema)
  // 缺必填 → return ErrConfigIncomplete + missing 列表
  
  // 3. spawn subprocess + send init
  pythonHandle := SpawnLongLived(...)
  pythonHandle.SendStdin({"type":"init", "args": config})
  
  // 4. 等待 ready 响应
  ...
```

### 6.5.7 HTTP API

| 端点 | 用途 |
|---|---|
| `GET /api/v1/handlers/{id}/config` | 返 schema + `{configured: [...], missing: [...]}`,**不返 secret 值** |
| `PATCH /api/v1/handlers/{id}/config` | 写 / 更新 partial config(后端加密合并) |
| `DELETE /api/v1/handlers/{id}/config` | 清空 config(测试 / 重置用) |

### 6.5.8 LLM 工具

只给写工具,**不给读工具**(LLM 不该看 secret 值):

| 工具 | 用途 |
|---|---|
| `update_handler_config({id, partial})` | 写 / 更新 partial config |

LLM 通过 `get_handler` 间接拿到 config 状态(返回 `configured` / `missing` 字段,不返值)。

### 6.5.9 Workflow 场景

workflow 的 `handler` 节点跑 instance 时,**同样从 user 的 Handler Config 拿 init_args**(workflow 不能 override)。多 DB 场景用户造多个 Handler Definition(`pg-prod` / `pg-staging`),**不要复用一个 Definition 加 override**。

V1.5 看用户反馈是否加 per-workflow init_args override(往 `handler` 节点 config 加 `init_args_override` 字段)。

### 6.5.10 Sensitive 字段处理

| 维度 | 处理 |
|---|---|
| 写时 | UI 密码框 / LLM 工具 args description 标 "sensitive,不要 log" |
| 存时 | 整 config JSON AES-GCM 加密(不区分 sensitive/non-sensitive,简化) |
| 读时 | **永不返明文**(GET config 只返 configured/missing,get_handler 不返 config) |
| 日志 | 整 config JSON 不进日志(zap 字段过滤) |

### 6.5.11 错误码

- `HANDLER_CONFIG_INCOMPLETE` 422 — spawn 时缺必填 init_args,body 含 `missing[]`
- `HANDLER_CONFIG_INVALID` 400 — 类型 / 格式不合 schema
- `HANDLER_CONFIG_DECRYPT_FAILED` 500 — 加密 key 失效(machine fingerprint 变,跨设备拷贝等)

### 6.5.12 失败回路

config 改了但 instance 已经活着 → V1 不动 instance(沿用旧 config 直至 destroy);用户想生效就显式 `DELETE /handlers/{id}/instances/{instanceId}` 销毁 + 下次 call 重 spawn。V1.5 加自动 reload。

---

## 7. 持久化 — 2 张本域表 + 1 张 execution log 表

本域**自身**只 2 表(handlers / handler_versions,以下 §7.1 / §7.2);call 记录(`handler_calls` 表)走**共享 schema 模板**,含 method / instance_id / owner_kind / owner_id 等 kind-specific 字段,详见 [`08-executions.md`](./08-executions.md) §4.2。Service.Call 终态写入。LLM 通过 `query_executions({kind:"handler", entityId:hd_xxx})` 看历史。

### 7.1 `handlers`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | `hd_<16hex>` |
| user_id | TEXT 索引 | local-user |
| name | TEXT | partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL` |
| description | TEXT | — |
| tags | TEXT (JSON) | — |
| active_version_id | TEXT | 指向当前活 HandlerVersion |
| **config_encrypted** | TEXT NULL | AES-GCM 加密的 init args config(JSON,详 §6.5);未配时 NULL |
| 时间戳 + 软删 | — | GORM 标配 |

**计算字段**(`gorm:"-"`):
- `Pending *HandlerVersion`
- `EnvStatus / EnvError / EnvSyncedAt / EnvSyncStage / EnvSyncDetail`(从 active version 拷)
- `LiveInstances int`(可选,从 registry 拿当前 instance 数显示)

### 7.2 `handler_versions`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | `hdv_<16hex>` |
| handler_id | TEXT 索引 | FK → handlers.id |
| status | TEXT CHECK | `pending` / `accepted` / `rejected` |
| version | INT NULL | accepted 递增 |
| code | TEXT | Python class 主体 |
| methods | TEXT (JSON) | List[MethodSpec] |
| init_args_schema | TEXT (JSON) | 可空 |
| dependencies | TEXT (JSON) | List[string] |
| python_version | TEXT | PEP 440 |
| env_id | TEXT 索引 | `hdenv_<16hex>`,每 Version 行独立生成(D-redo-8 与 version_id 解耦) |
| env_status / env_error / env_synced_at / env_sync_stage / env_sync_detail | — | 同 function_versions(sync_stage 加 `fixing` 表示 env-fix loop 中) |
| change_reason | TEXT | — |
| 时间戳 | — | — |

**没有 instances 表** —— Instance 是运行时对象,不持久化。

---

## 8. LLM 工具集 + HTTP API

### 8.1 LLM 工具

```typescript
search_handler({ query?, limit?, cursor? }) → { items, nextCursor?, hasMore }
get_handler({ id }) → { handler, activeVersion?, pending?, configState? }
  // configState: { configured: ["dsn"], missing: [] } — 不返 secret 值

create_handler({ name, description, ops, changeReason })
  → { id, versionId, version, status, envStatus, envError?, attemptsUsed, opsApplied }
  // 流式:每 op emit progress;env 装阶段 emit forge_env_attempt × N;
  // 内部 env-fix loop(D-redo-15/16/17/18):装失败 → 调主 chat scenario LLM
  // 让其只改 deps → 重装 → 最多 3 次。成功后 auto-accept v1。

edit_handler({ id, ops, changeReason })
  → { pendingId, envStatus, envError?, attemptsUsed, opsApplied }
  // pending 已存在时 — 重写同 ID pending 行(D-redo-11),不返 ErrPendingConflict;
  // 旧 env 销 + 新 env 装走 fix loop。
  // ops=[] 显式语义 — 强制重建当前 active version 的 env(D-redo-22)。

revert_handler({ id, targetVersion }) → { activeVersionId }
delete_handler({ id }) → { deleted }
call_handler({ handlerName, method, args }) → { ok, output, error?, elapsedMs }
  // 隐式 acquire by caller-context;LLM 不传 instance_id;
  // 缺必填 config → HANDLER_CONFIG_INCOMPLETE → AskUserQuestion → update_handler_config 重试
update_handler_config({ id, partial }) → { configured: string[] }
  // 写 / 更新 partial config(后端加密合并;LLM 收用户输入后调用此工具回写)

// D22 — execution log 工具(per-entity,handler_calls 表)
search_handler_calls({ handlerId?, method?, ownerKind?, instanceId?, status?, conversationId?, flowrunId?, since?, until?, limit?, cursor? })
  → { count, calls[], nextCursor?, aggregates }
get_handler_call({ id })
  → { ...全字段..., input 截 4KB(sensitive 字段 mask), output 截 4KB, hints }
```

### 8.2 HTTP API(~16 端点)

```
POST   /api/v1/handlers                                      创建
GET    /api/v1/handlers                                      列表
GET    /api/v1/handlers/{id}                                 详情
PATCH  /api/v1/handlers/{id}                                 改 meta
DELETE /api/v1/handlers/{id}                                 软删

GET    /api/v1/handlers/{id}/versions                        版本列表
GET    /api/v1/handlers/{id}/versions/{v}                    单版本
POST   /api/v1/handlers/{id}:revert                          回滚
GET    /api/v1/handlers/{id}/pending                         看 pending
POST   /api/v1/handlers/{id}/pending:accept                  接受(瞬时返,env 不在此装)
POST   /api/v1/handlers/{id}/pending:reject                  拒绝(销 env + 删 pending 行)

GET    /api/v1/handlers/{id}/config                          看 config 状态(不返 secret 值;§6.5.7)
PATCH  /api/v1/handlers/{id}/config                          写 / 更新 partial config(加密)
DELETE /api/v1/handlers/{id}/config                          清空 config

POST   /api/v1/handlers/{id}:call?conversationId=...         手动调用
                                                              (HTTP 必须传 caller-context query 参)
GET    /api/v1/handlers/{id}/calls                           call 历史(D22)
GET    /api/v1/handler-calls/{callId}                        单 call 详情 + hints(D22)
```

**删除的端点**:`:resync` 端点删除(D-redo-14)— "env 装失败后想重装"路径走 LLM tool `edit_handler({id, ops:[]})`(D-redo-22)。`/handlers/{id}/instances` GET + DELETE 端点未在 Plan 02 落地(留作 V1.5 power-user 入口),本节去掉以反映现实。

---

## 9. Instance lifecycle 触发器汇总

| 事件 | 动作 |
|---|---|
| 第一次 `call_handler` 同 caller-context | spawn instance + register |
| 同 caller-context 后续 `call_handler` | reuse instance + update lastUsedAt |
| Idle N 分钟(chat scope only,默认 N=10) | destroy instance |
| Conversation deleted / archived | 该 conv 拥有的所有 instances 全 destroy |
| FlowRun 终态(completed / failed / cancelled) | 该 run 拥有的所有 instances 全 destroy |
| Test execution 结束 | 该 test 拥有的 instances 全 destroy |
| Handler Definition deleted | 全部相关 instance(跨所有 caller-context)destroy |
| Handler Definition active version 翻新(accept pending) | 全部旧 instance 强制 destroy(下次 call 走新 version) |
| Process exit | 全部 instance destroy(cleanup hook) |
| Subprocess crash | 标 instance unhealthy + 下次 call 时 destroy + re-spawn |
| RPC timeout 超阈值 | 标 instance unhealthy + 下次 call 时 destroy + re-spawn |

---

## 10. 错误码

| Code | HTTP | Sentinel | 触发 |
|---|---|---|---|
| `HANDLER_NOT_FOUND` | 404 | `handler.ErrNotFound` | Definition id 查不到 |
| `HANDLER_NAME_DUPLICATE` | 409 | `handler.ErrDuplicateName` | 重名 |
| `HANDLER_METHOD_NOT_FOUND` | 404 | `handler.ErrMethodNotFound` | call 时 method 不存在 |
| `HANDLER_VERSION_NOT_FOUND` | 404 | `handler.ErrVersionNotFound` | revert 不到的 version |
| `HANDLER_PENDING_NOT_FOUND` | 404 | `handler.ErrPendingNotFound` | accept/reject 无 pending |
| `HANDLER_INSTANCE_SPAWN_FAILED` | 422 | `handler.ErrInstanceSpawnFailed` | spawn 时 sandbox / init 失败 |
| `HANDLER_INSTANCE_CRASHED` | 422 | `handler.ErrInstanceCrashed` | RPC 时子进程死 |
| `HANDLER_INSTANCE_RPC_TIMEOUT` | 504 | `handler.ErrInstanceRPCTimeout` | method timeout |
| `HANDLER_NO_ACTIVE_VERSION` | 422 | `handler.ErrNoActiveVersion` | 草稿 + pending 未 accept |
| `HANDLER_ENV_NOT_READY` | 422 | `handler.ErrEnvNotReady` | env 非 ready(syncing / evicted 且 in-flight 也失败)|
| `HANDLER_ENV_FAILED` | 422 | `handler.ErrEnvFailed` | env-fix loop 跑满 maxAttempts 仍失败;envError 含末态摘要 + attemptHistory |
| `HANDLER_SANDBOX_UNAVAILABLE` | 503 | `handler.ErrSandboxUnavailable` | Service.Create / Edit 调 sandbox 前 ping 失败(D-redo-20)— **硬拒,不建 entity** |
| `HANDLER_OP_INVALID` | 400 | `handler.ErrOpInvalid` | ops 应用失败 |
| `HANDLER_INSTANCE_NOT_FOUND` | 404 | `handler.ErrInstanceNotFound` | 手动 destroy 时 instance 不存在 |
| `HANDLER_CONFIG_INCOMPLETE` | 422 | `handler.ErrConfigIncomplete` | spawn 时缺必填 init_args(body 含 `missing[]`)|
| `HANDLER_CONFIG_INVALID` | 400 | `handler.ErrConfigInvalid` | config 类型 / 格式不合 schema |
| `HANDLER_CONFIG_DECRYPT_FAILED` | 500 | `handler.ErrConfigDecryptFailed` | 加密 key 失效(machine fingerprint 变 / 跨设备拷贝)|
| `HANDLER_CALL_NOT_FOUND` | 404 | `handler.ErrCallNotFound` | D22 get_handler_call 找不到 |

**删除**:`HANDLER_PENDING_CONFLICT`(409)— Edit 改"iterate same pending"后,pending 已存在不再返冲突(D-redo-11)。

---

## 11. Sandbox 集成

**关键区分**:env(venv)和 subprocess 是两个独立维度:
- **env**:`env_id = idgenpkg.New("hdenv")`(D-redo-8)— **每 HandlerVersion 独立 venv**,跨 instance 共享同 Version 的 venv,但不跨 Version 共享;env_id 跟 version_id 1:1 但独立生成,sandbox 当不透明 string(其他消费者 mcp/chat tool 等用各自前缀互不冲突)。销 instance 不影响 venv,销 Version 才 destroy env
- **subprocess**:per-instance long-lived;state 在 Python 内存里,跟 venv 文件无关。状态隔离来自 process 隔离

调度链:
1. `EnsureRuntime python <version>`(mise install if needed,共享 runtime)
2. `EnsureEnv(owner={Kind:"handler", ID:"<handlerID>_<envID>"}, deps)` 走 PythonEnvManager — env 维度 owner.ID 拼 handlerID + 各 Version 独立的 env_id(`hdenv_<16hex>`)
3. `SpawnLongLived(owner=<spawnOwner>, opts{...})` — spawn 维度 owner per-instance(active handle 跟踪用,跨 instance 不共享)
4. 注册到 registry,Handle 保留以便后续 RPC

具体 `<spawnOwner>` 的字符串形态由 sandbox v2 现有约定决定(e.g. `handler-instance:<callerKind>:<callerID>:<handlerName>`)。**spec 不锁定 owner 字符串**,实施时按 sandbox 现有 convention 走。

**Env 装配时机** — **同步发生在 `create_handler` / `edit_handler` 工具内部**(D-redo-9):apply ops → 持久化 Version 行 → 调 sandbox sync(blocking)→ 失败进 env-fix loop(maxAttempts=3,主 chat scenario LLM 改 deps)。详见 [`02-function.md`](./02-function.md) §10 — 跟 function domain 同模式。**没有后台 sync 入口**(D-redo-14:删 `SyncEnvForVersion` / Resync 路径)。**Service.Create / Edit 前置 sandbox ping**;失败返 `ErrSandboxUnavailable`(503)硬拒(D-redo-20)。

进程 leak 防御走现有 sandbox v2 三层:
- Layer A:active handle 注册表 + Service.Shutdown 并发 Kill
- Layer B:Env.RunningPID 持久化 + boot 时 cleanup stale
- Layer C:OS-level(Linux PR_SET_PDEATHSIG / Windows Job Object KILL_ON_JOB_CLOSE)

详见 `service-design-documents/sandbox.md`。

---

## 12. RPC 协议设计

### 12.1 Wire format

stdio,每行一个 JSON 消息(LF 分隔),无 size header(简单,Python 内置 json 友好)。

### 12.2 Message types

```typescript
// 系统 → Python
{ type: "init", args: {...} }                      // 启动初始化
{ type: "call", id: "<reqId>", method: "...", args: {...} }
{ type: "shutdown" }

// Python → 系统
{ type: "ready" }                                  // init 完成
{ type: "init_error", error: "...", trace: "..." }
{ type: "progress", id: "<reqId>", data: "..." }   // yield 的内容
{ type: "return", id: "<reqId>", data: <any> }     // method 返回值
{ type: "error", id: "<reqId>", error: "...", trace: "..." }
```

### 12.3 并发 RPC

V1 同 instance 内 method call **串行**(per-instance lock):
- 避免 Python 内部 race(用户写的 class 不一定线程安全)
- RPC 队列 in-memory,先到先服务
- LLM 一个 batch 内多个 `call_handler` 同 instance → 串行执行

V1.5 加 method-level concurrency 配置(若 Python class 内显式声明 thread-safe,可并行调用)。

---

## 13. Catalog source

`app/handler/AsCatalogSource()` Per-item granularity。description 含 methods 概览(前 3 个 method 名 + desc)+ **`configState` 字段**(让 LLM 心智清晰知道哪些可立调):

```
PG-Prod — PostgreSQL connector. Methods: query, insert, migrate.
  configState: ready

PG-Staging — PostgreSQL connector for staging.
  configState: unconfigured (missing: dsn)
```

`configState` 取值:
- `ready` — 全部必填 init_args 都已配
- `partially_configured` — 部分必填 init_args 缺(rare,因 V1 校验 PATCH 时不允许只填一半;V1.5 看)
- `unconfigured` — 完全没配过 / 必填全缺

LLM 看 catalog summary 时一眼分清"可立调 / 需先 setup",`call_handler` 前不必猜。

具体实现参考 [`02-function.md`](./02-function.md) §11 同模式 + 加 `configState` derivation logic。

---

## 14. 测试覆盖(V1 目标)

| 测试套件 | 覆盖点 |
|---|---|
| `app/handler/apply_test.go` | 各 op 应用 + 校验失败 |
| `app/handler/registry_test.go` | caller-owns 销毁级联 / idle GC / 跨 owner 隔离 |
| `app/handler/service_test.go` | spawn / call / cleanup mock |
| `app/handler/rpc_test.go` | stdio JSON-RPC 协议 invariants |
| `test/handler/handler_pipeline_test.go` | E2E:create → call(spawn)→ call(reuse)→ idle GC → call(re-spawn)|
| `test/handler/lifecycle_pipeline_test.go` | conv 删 → instance 销;flowrun 结束 → instance 销;process exit → all clean |
| `test/handler/streaming_test.go` | yield → progress block delta;return → tool_result |

---

## 15. 实现清单

Plan 02 主体已 merge(2026-05-12,11 commits);env 模型重整作为 2026-05-12 redesign 的一部分,后续 commit 改动点(跟 function 同模式):

1. **domain/sandbox_types** — 删 `ComputeEnvID(deps, python)` 哈希函数(若 handler 自有);Version 创建时 `EnvID = idgenpkg.New("hdenv")` 独立生成(C1.1 与 versionID 解耦)
2. **app/handler/apply.go** — Create / Edit 改同步装 env(走 fix loop);AcceptPending 纯指针;RejectPending 销 env + 删行
3. **app/tool/handler/{create.go,edit.go}** — 加内部 env-fix loop(maxAttempts=3,主 chat scenario LLM,只改 deps);edit `ops=[]` 显式语义重建 env
4. **app/handler** — Service.Create/Edit 前置 sandbox ping;失败返 `ErrSandboxUnavailable`
5. **transport/httpapi/handlers/handler.go** — 删 `:resync` 端点(若有);`:pending/accept` 改瞬时返
6. **删 sentinel** `ErrPendingConflict` + 删 errmap 行
7. **文档同步**(per S14):本文件 + service-design-documents/handler.md + database-design.md + error-codes.md + progress-record

---

## 16. 主要风险

| 风险 | 缓解 |
|---|---|
| Instance 进程 leak | sandbox v2 三层防御 + 进程退出 hook |
| RPC 阻塞 / hang | per-method timeout + instance 总超时(5min idle 强制 kill) |
| 用户 Python class 写得有 bug 卡死 | RPC timeout 触发 destroy + 标 instance unhealthy |
| Instance 跨进程死亡(如 OOM Killer) | 系统检测 stdout 关闭 → 标 instance crashed → 下次 call re-spawn(状态丢但不级联崩溃) |
| Handler Definition 改了但旧 instance 还在 | active version 翻新时强制 destroy 旧 instance |
| Caller-context 漏触发清理 hook | Process exit 时强制全清(底兜) |

---

(本文档完)
