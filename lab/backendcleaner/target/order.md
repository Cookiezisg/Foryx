# backend-new 重写顺序（依赖拓扑驱动）

> 本文是"基础 → 复杂"的执行路径，确保**不重不漏**。
> 顺序的依据是 `go list` 实测的 import 依赖图（2026-06-03 扫描），不是拍脑袋。
> 进度状态在 `STATE.md`（单一事实源），本文只定"顺序 + 模块边界 + 旗标"。

---

## 0. 已验证的依赖事实

- **domain 层干净**：28 个 domain 包里只有 4 条 domain→domain 依赖（`agent→model`、`conversation→document,model`、`forge→eventlog`、`workflow→model`），**没有任何 domain 反向依赖 app/infra**。地基层架构是对的，重写风险低。
- **app 层是 DAG，无强连通环**：`scheduler`/`trigger` 通过依赖倒置（接口注入，不 import 对方包）连接；`tool` 子包用 DIP（service 依赖 `tool` 基础接口、`tool/<x>` 适配器依赖 service）避环。**所以"一个一个、基础→复杂"可行，不会卡环。**
- 重写单元 = **垂直切片**：一个业务模块 = `domain/<m>` + `infra/store/<m>` + `app/<m>` + `app/tool/<m>`（若是工具）+ `transport/.../handlers/<m>.go`（若有 HTTP）。粒度见 `module-template.md`（按需取层）。

---

## 1. 重写波次（每波次内可并行，跨波次严格串行）

> 规则：写模块 X 时，它依赖的所有模块必须已在 `backend-new/` 就绪。
> 每波次收尾：`cd backend-new && go build ./...` + smoke（启动 / health / 用户初始化）必须绿。

### 波次 0 — 地基（无业务逻辑，所有上层的根）

| 编号 | 模块 | 说明 | 旗标 |
|---|---|---|---|
| M0.1 | `pkg/*` 纯工具 | idgen, pagination, reqctx, tokencount, pathguard, userpath, jsonrepair, wikilink, limits | ⚠️ `modelcaps`（modelcatalog 已取代，疑残留）、`agentstate`/`envfix`/`installprogress`/`llmclient`/`llmcost`/`llmparse` 逐个判定去留；`forge`/`notifications`/`eventlog`（producer 辅助层，非残留）→ 统一 `pkg/streamemit`，随三流 M0.4/M0.5 |
| M0.2 | `pkg/orm` + `infra/db` | **去 GORM**：自研链式 ORM（R0008 ✅）+ `infra/db` 用 database/sql + glebarez/go-sqlite；**R0018 补 `ErrConflict`**（UNIQUE 冲突翻译，对称 NotFound） | 边界：schema 可激进重定；手写 DDL 对齐 database.md（取代 AutoMigrate）；domain 全部去 GORM 化 |
| M0.3 | `infra/logger` `infra/crypto` | zap + AES-GCM | |
| M0.4 | `domain/errors` · `domain/stream`(协议核心) · `domain/messages` `domain/entities` `domain/notifications` | 横切契约；SSE 三流**统一流式树协议**：stream 定 Envelope/Frame/Node/Bridge，三流各挂 Node 词表（见 `stream-protocol.md`） | eventlog→messages · forge→entities |
| M0.5 | `infra/stream`(单一 `Bus`) | SSE 三流底座：单一 `Bus`(seq + frame 分级 buffer + fanout)实例化三次 = messages/entities/notifications。`infra/chat`(extractor) 依赖 chat domain → **移交 M5.2** | frame 分级：delta/tick=ephemeral 不入 buffer；close 带快照；D2=workspace 全量推 |
| M0.6 | `infra/llm` | 自有 provider 客户端 + factory。**R0015 框架+openai · R0016 其余 10 provider** | 边界：wire 格式冻结；**每家完整自包含、不共享 wire 基座**（duplication < wrong abstraction）；mock 留测试；trace 推迟 M5.2/M7 |
| M0.7 ✅ R0017 | transport 框架：`response`(N1+errmap 塌缩 statusForKind+SSE marshal+pagination Parse) · `middleware`(workspace+5 标准件) · `router`(recorder+chain) | — | 零业务依赖框架；**完整 New(装配所有 handler)+Deps 容器+health → cmd/server M7** |

### 波次 1 — 叶子业务域（只依赖地基）

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M1.1 ✅ R0018 | `workspace`（原 `user` 正名） | — | 隔离标识=workspace_id；**多 workspace 数据隔离 + 资源不分桶**；Name 自由名 UNIQUE（去 slug/GetByUsername/EnsureExists）；`Validate` 实现 WorkspaceResolver；boot 默认 ws + 注入 + 共享资源布局 → M7 |
| M1.2 ✅ R0019 | `apikey` | domain/crypto | **收窄** = 加密保险箱 + 哑探针 + 按 id 发钥匙；选 key / 解析 / 模型理解全下放（model / 搜索配置）；首个吃 orm 自动隔离的表；modelcatalog/capabilities → M1.3 |
| M1.3 ✅ R0020 | `model` | domain/apikey | modelcapoverride/modelcaps 确认无残留 R0020（空目录/不存在，backend-new 从零未迁，旧旗标作废） |
| M1.4 ✅ R0021 | `relation` | — | 横切（实体关系图）；4 动词边(create/edit/equip/link)、8 节点、`KindForID` 8 条(补 agent + 定 sk_/mcp_ 规矩)、读时内存 hydrate name(无 reader port)、override 式弱引用无删除保护 |
| M1.5 ✅ R0022 | `catalog` | — | 能力概览「实体名录」：只报名字+描述按类型分组；砍调用工具/Generator/Granularity 等预留；两段式概览→搜索(id 不进菜单/name 不唯一)；无 store 派生 |
| M1.6 ✅ R0023 | `mention` | — | @ 引用快照纯 domain 契约：5 种可 @ 类型(四件套+document) + Resolver 接口 + IsValidMentionType；Freeze-on-Send；resolver 波次 3 / chat 渲染+错误 波次 5 |
| ✅ R0024 | `notification` | domain/stream | 通知中心实体(DB 持久 + SSE durable signal)；scope=notification:noti_x、workspace 是 Bus 分流轴非 scope；Emitter 端口；memory 等发通知的前置依赖；连带 stream 清理 + R0018 分桶翻转 |
| M1.7 ✅ R0025 | `memory` | domain/notification | 文件式按 workspace(~/.forgify/workspaces/<wsID>/memories/*.md)；两段式注入(pinned 全文+目录按需读)；无 mem_ id/无 SQLite；发通知用 notification.Emitter；首个文件式 store skills 复用；工具/chat 注入留波次 2/3/5 |
| M1.8 ✅ R0026 | `sandbox` | infra/sandbox | 三 runtime(Python+Node+Docker，registry 调研 92%+7 docker 缺口)；image=docker runtime/容器=env 统一双接口、`ResolveExec`；两表系统级不分桶(orm meta.ws==nil)；去 GORM+硬删；docker 探测+pull+run(不代装)；Emitter；路由 RESTful+N5；**docker 精细化(stop/孤儿/stdio e2e)留 M3.6、注册+base+fetch-mise 留 M7** |
| M1.9 ⏭️ R0027 | ~~`permissions` / `hooks`~~ **判定解散** | — | hooks 砍（Claude Code 花活）、危险控制别处管（不做中央门控）、limits 用 `pkg/limits` 默认、settings.json 砍；permissions domain + app/hooks + infra/settings 全不迁。连带 M5.4 permissionsgate 解散、M5.2 chat 去 hooks 依赖 |
| M1.10 ✅ R0028 | `document` | catalog, relation, mention | Notion 树(树 CRUD/path 级联/防环/软删/COALESCE UNIQUE)+ 显式挂载注入(**无 RAG/砍子树**)；去 GORM+workspace；**4 适配器对齐前三模块新地基**(catalog 去 Granularity/Category、relations wikilink→KindForID→link 边、mention、Namer)；注入留 M7、attach 消费波次4/5、:iterate 波次6 |
| M1.11 ✅ R0029 | `todo` | — | TodoWrite 式重铸（1 工具整列替换 / `scope_id` 多态键 / 双层注入 / messages live）；工具波次 2-3、注入 M2.2、bridge M7、前端真看板覆盖后 |

### 波次 2 — tool 基础 + 执行原语

| 编号 | 模块 | 依赖 | 旗标 |
|---|---|---|---|
| M2.1 ✅ R0030 | `tool`（基础接口） | infra/llm | **S18 9→5 方法**（删权限模式机制）；framework 注入 summary/`danger`(三级)/execution_group；Toolset 懒加载保留（与 catalog 正交）；danger 确认流/并行批 → loop M2.2 |
| M2.2 ✅ R0031 | `loop`（ReAct 引擎）+ `domain/messages` | tool, messages | 共享 ReAct 引擎接 stream(eventlog→messages 流 open/delta/close、close 带快照)、danger 纯标记、删 interceptor(M1.9)、todo 注入走 `ReminderProvider` 钩子；**建 messages domain**(Block/ToolCallData/词表无家可归——修正 loop 依赖 chat 耦合反向)；executeTool 极简(删权限/sanitize/enrich)；agentstate 零依赖(随各工具消费者重建)；message_blocks 表/落盘/History 留 M5.2 |
| M2.3#1 ✅ R0032 | `tool/filesystem` | tool, pkg/agentstate(新建,SeenFiles 渐进) | Read/Write/Edit 三件套:9→5 方法机械跟进;Read 用 `Allow`、Write/Edit 升级用 `AllowWrite`(.git/.env/node_modules 物理拦截);写前必读 fail-closed;Edit size 漂移;原子写 mode 保留;danger 不静态(M2.1 纯信任);agentstate activeSkill/activatedGroups 留 skill/toolset 按需追加(**cwd R0033 废弃**) |
| M2.3#2 ✅ R0033 | `tool/search`(LS/Glob/Grep) | tool, pkg/fspath(新建) | 三件套;**LS 新增**列目录;**无 cwd 全绝对路径 + `~` 展开**(fspath,六文件工具共用);Grep 双后端(rg 优先/stdlib 兜底,不代装);path 必填;danger LLM 自报工具不碰;**回溯改 filesystem 补 `~` + cwd 全局废弃** |
| 前置 ✅ R0034 | `domain/websearch`(新) + workspace `default_search_key_id` | workspace | 搜索配置(web 前置):独立 `domain/websearch` 包(Provider 词表 + `SearchKeyPicker`、无 store、对齐 domain/model)+ workspace 加列(选 key 显式、防乱烧钱、provider 由 key 隐含);存储借 workspace、不建 app/websearch |
| M2.3#3 | `tool/web`(WebFetch + WebSearch) | tool, websearch, apikey, model, llm | WebFetch 摘要链对齐新地基 + WebSearch 消费 `SearchKeyPicker`(BYOK)+ MCP 接口(注入)+ SSRF 守卫 |
| M2.3#4 | `tool/toolset` | tool | 剩余叶子工具适配器 |

### 波次 3 — Quadrinity 执行体

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M3.1 | `function` | sandbox | |
| M3.2 | `handler` | sandbox | |
| M3.3 | `subagent` | loop, tool | |
| M3.4 | `agent` | loop, tool | 🔧 in-flight：execution 面对齐 function/skill（当前未提交改动）|
| M3.5 | `skill` | subagent | |
| M3.6 | `mcp` | infra/mcp | ⚠️ `infra/store/mcpcalls`+`mcphealth` 判定 |
| M3.7 | tool 适配器 | `tool/function` `tool/handler` `tool/agent` `tool/skill` `tool/subagent` `tool/mcp` `tool/memory` `tool/document` `tool/shell` | `domain/forge` 在此被工具适配器依赖（forge 角色已定：M0.4 SSE 三流之一，非旧实体） |

### 波次 4 — 编排核心（最复杂，重灾区）

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M4.1 | `workflow` | document, function, handler, mcp, skill | 5-node DAG 规格 + CEL；**13→5 节点收敛在此定型** |
| M4.2 | `flowrun`（domain+store: flowrun/flowrunevent/approval） | — | Journal 真相模型 |
| M4.3 | **`scheduler`** | agent, function, handler, loop, mcp, skill, tool, workflow | 🔴 **最大重灾区**：删 topo-walk 旧链（state/pause/subdag/retry 旧半）；14 dispatcher 收 5；`LoopDispatcher` 删→结构化 loop 取代；只保留 durable interpreter |
| M4.4 | `trigger`（+ infra/trigger: cron/fsnotify/webhook/polling） | workflow | inbox 单事务 claim；⚠️ polling listener 是否真需要 |
| M4.5 | `tool/workflow` | scheduler, workflow | |

### 波次 5 — 对话与上下文

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M5.1 | `conversation` | — | |
| M5.2 | `chat` | document, loop, tool | runner 庞大，重点审；hooks/permissionsgate 已解散（M1.9 R0027），危险控制下放工具 |
| M5.3 | `contextmgr` | — | compaction |
| ⏭️ R0027 | ~~`tool/permissionsgate`~~ **随 M1.9 解散** | — | 中央门控取消；危险控制由工具自管（别处） |

### 波次 6 — 顶层智能编排

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M6.1 | `askai` | agent, chat, conversation, document, function, handler, workflow | :iterate / :triage（N5） |
| M6.2 | `ask` + `tool/ask` | — | ⚠️ **强残留嫌疑**：疑似 askai 旧版；判定后大概率删 |

### 波次 7 — wiring（transport 框架已上移波次 0；handler 随各业务模块）

> `response`/`middleware`/`router` 在 M0.7；`handlers/<m>.go` 是各业务模块垂直切片的一层（user 模块含 users.go），不在此集中。`dev_*` 及 `answers·scenarios·prompts·capabilities·context_stats·metrics·usage` 等信息端点随其对应模块 handler 逐个判定去留。

| 编号 | 模块 | 说明 | 旗标 |
|---|---|---|---|
| M7.1 | `cmd/server`（DI 装配） | main.go：装配所有模块（import 全部，天然最后）；只注册 5-node | |
| M7.2 | `cmd/desktop`（wails）`cmd/resources` `cmd/doc-*` `cmd/lintprompts` `cmd/coverage-matrix` | 入口 + 工具 | embed = generated |

---

## 2. 覆盖阶段（所有波次完成后）

1. `backend-new` 自证完整：`go build ./...` 全平台 + 全部新测试绿 + 能力对账表（`capability-ledger.md`）全勾。
2. 覆盖：`rm -rf backend && mv backend-new backend`（module path 已是最终值，import / Makefile 零改动）。
3. 前端 / testend 兼容：按 `contract-changes.md` 逐条施工。
4. 全链路 e2e + 全量 verification。

---

## 3. 跨模块依赖问题：就地登记

重写某模块时若发现"它依赖的下游模块设计有问题、需要等那一轮调整"，**不要当场跨界改**，在该下游模块的 `target/rounds/` 记录或 `contracts/<下游>.md` 顶部"待调整"区登记，留给那一轮处理。已知大项先列在此：

- `scheduler`（M4.3）：topo-walk 整条旧链删除、14→5 dispatcher 收敛——见独立审计（已深挖）。
- `ask`/`askai`（M6）：双实现合并决策。
- ~~`forge`（domain/infra/pkg 三处）~~：✅ 已判定为 SSE 三流之一（E1），保留；domain→M0.4 / infra→M0.5 / pkg 随附。
