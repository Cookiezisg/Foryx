# Forgify — Claude 工作守则

> Claude Code 进入本项目自动加载本文件。**本文件是项目工程纪律的唯一事实源**。
> 项目愿景 / 架构 / Phase 路线 / Verification 见 [`documents/version-1.2/backend-design.md`](documents/version-1.2/backend-design.md)。

---

## 项目一句话

- **本地优先 Agentic Workflow Platform**，目标 **Wails 桌面 app**（不做 SaaS）
- **单人项目**（既是产品也是工程师）
- 当前阶段：**V1.2 后端**，Phase 0-3 完成 + 优化轮中；Phase 4（工作流）/ Phase 5（智能化）未启动
- 架构：**4 层 clean arch**，依赖方向 `transport → app → (domain ∪ infra/store) → infra/db`

## 文档地图

| 用途 | 路径 |
|---|---|
| 项目愿景 / 架构 / Phase 路线 / Verification | `documents/version-1.2/backend-design.md` |
| 当前进展 / 决策日志 | `documents/version-1.2/progress-record.md` |
| 桌面端分发方向 | `documents/version-1.2/desktop-packaging-notes.md` |
| 各 domain 详设计 | `documents/version-1.2/service-design-documents/<domain>.md` |
| 契约索引（API / DB / Error / Events） | `documents/version-1.2/service-contract-documents/` |
| 调试控制台 | `documents/version-1.2/testend-design.md` |
| Claude Code 内部机制调研 | `documents/version-1.2/claude-code-research-documents/` |

## 改代码前必做（每次都要）

1. **改 backend/internal/<domain>/ 之前** 先读对应 `service-design-documents/<domain>.md`
2. **改完** 跑 `cd backend && go build ./... && go test -count=1 ./... && staticcheck ./...`
3. **同步联动文档**（S14 / 设计原则 #7）

---

# 设计原则（7 条，#7 最高优先级）

1. **每个 Phase 都能独立交付价值** — 不会出现"做了 80% 但啥都用不了"
2. **依赖严格自下而上** — 每个 Phase 只依赖前面已完成的 Phase
3. **复杂度阶梯式增长** — 基础 CRUD → 复杂 CRUD → 编排 → 智能
4. **V1.2 后端阶段不动前端** — 本轮专注后端契约与架构清理；前端在 Phase 5 完成后整体迁移到 Wails 桌面 app 形态。开发期用 curl + testend 控制台验证
5. **端到端推演先行** — 每个 domain 开工前**必须**先走一遍"用户一个请求从 HTTP 到最终调用"的完整数据流，列出所有跨 domain 依赖。避免设计看起来完整、实现时才发现"缺一个 domain"
6. **反校验剧场** — Forgify 是**本地 Wails + 单用户 + 同人写前后端**；backend 只保留真正有价值的校验（JSON 畸形、必填字段非空、path 白名单、NotFound 404、DB CHECK/UNIQUE），跳过"前端 dropdown 已筛 + 下游自然报错"式的重复校验。加校验前问自己："前端能不能防住？下游会不会自然炸？"两个都是，就不加
7. **📌 文档与代码同步（最高优先级）** — 每个代码改动必须伴随对应文档的同步更新；每个 domain 完成/推进时必须回头更新**全部** 4 处文档：
   - `service-design-documents/<domain>.md`
   - `service-contract-documents/{api,database,error,events}.md`
   - `progress-record.md`
   - `backend-design.md`（如有新原则/规范变动；大部分时候不改）

   **文档落后于代码 = bug**。详细执行规则见 §S14。

## 端到端推演模板

每个 domain 开工前必填一段"完整调用链"到 `service-design-documents/<domain>.md`：

```
触发源（HTTP/定时/事件）
  → transport 层：哪个 handler
    → app 层：哪个 service 方法
        → 调谁：model / apikey / 其他 domain，每一次 cross-domain 调用都要列
        → 用什么：从 ctx 读什么、从哪个 repo 读什么
      → infra 层：最终落到哪里（DB / 外部 API / 沙箱）
  → 响应路径：成功 / 失败分别怎么返
```

**不走一遍这个推演，不开工**。

---

# Standards — 契约宪法 + S/T 系列

## HTTP API（N 系列）

1. **N1 统一 envelope**：成功 `{"data": ...}`；失败 `{"error": {"code", "message", "details"}}`
2. **N2 状态码严格语义**：200 读/更新 / 201 创建 / 204 删除 / 400 参数错 / 404 不存在 / 409 冲突 / 410 历史已淘汰 / 422 业务拒绝 / 500 内部错
3. **N3 字段 camelCase**：API 请求/响应一律 camelCase；DB 列 snake_case，repo 层转换
4. **N4 列表强制分页**：`?cursor=xxx&limit=50` → `{data, nextCursor, hasMore}`
5. **N5 RESTful 严格化**：资源用名词；状态改动走 `PATCH` + 状态字段；动词用 `:action` 后缀（`POST /tools/{id}:duplicate`）
6. **N6 PUT 幂等返 200**：upsert 类 PUT 端点（如 `PUT /model-configs/{scenario}`）无论新建或更新一律返 200——客户端不需要区分。区别 create/update 时才用 POST 返 201
7. **N7 SSE wire format**（事件日志协议专用）：
    - 每事件按 SSE 标准发送：`event: <type>` + `id: <seq>` + `data: <event payload JSON>` + 空行
    - payload 不重复 `type` / `seq` 字段（已在 SSE header 行）
    - 客户端断线重连用 `Last-Event-ID: <seq>` header；服务端从 `seq+1` 起 replay buffer 内事件
    - 超出 buffer 返 `410 Gone` + `code=SEQ_TOO_OLD`，客户端经 `GET /api/v1/conversations/{id}/eventlog?from=<seq>` refetch 全态
    - 详 [`event-log-protocol.md` §3](documents/version-1.2/event-log-protocol.md)

## 数据库（D 系列）

7. **D1 软删除统一**：所有表用 `deleted_at DATETIME`（NULL = 未删除），废弃 `status='deleted'` 风格
8. **D2 时间戳统一**：每表必有 `created_at` / `updated_at`，GORM 自动维护
9. **D3 枚举 CHECK 约束**：稳定白名单（如 `role`、`content_type`、`message_blocks_v2.type`、`message_blocks_v2.status`）在 DB 层做 CHECK；会随 Phase 扩张的白名单（如 `scenario`）在 app 层校验。`message_blocks_v2` 枚举值固定 6/4 种（详 §E1）
10. **D4 PRAGMA `foreign_keys=ON` 开启约束**（V1.2 单用户本地暂未在 GORM tag 声明 `foreignKey`；跨表引用由 app 层 service 管 lifecycle；显式 FK 声明是 aspirational，等真撞跨层完整性 bug 时再加）
11. **D5 业务唯一性用 UNIQUE 约束**：`tools.name`、`(tool_id, version)`、`(user_id, scenario)` 等
12. **D6 schema_extras 模式**：AutoMigrate 表达不了的 SQL（partial 索引 / 触发器 / FTS5 虚拟表 / 复杂 CHECK）走 `infra/db/schema_extras.go`。每条语句必须**幂等**（`CREATE … IF NOT EXISTS`）+ 按 table 分组 + 入口先 `db.Migrator().HasTable()` 守卫
13. **D7 索引归属判断**：**普通**（含复合、UNIQUE）索引一律走 GORM tag（`index:idx_x,priority:N` 或 `uniqueIndex`）；只有带 `WHERE` 子句的 partial 索引才进 schema_extras。例：`UNIQUE(user_id, name) WHERE deleted_at IS NULL` 必须 schema_extras；`(message_id, seq)` 复合索引应放 GORM tag

## SSE（E 系列）

14. **E1 三协议**:后端只有三条 SSE 流,**全部按 user_id 订阅**(后端 Bridge 从 ctx 抽 user_id,无 query 参数),client 按 payload 字段 demux/filter
    - **事件日志** `GET /api/v1/eventlog` —— 5 events × 6 block types,**chat 内流式内容**(per-user 订;payload 带 `conversationId`,client demux 到对应 panel)
      - 5 事件:`message_start` / `message_stop` / `block_start` / `block_delta` / `block_stop`
      - 6 block 类型:`text` / `reasoning` / `tool_call` / `tool_result` / `progress` / `message`
      - 每事件带 `conversationId` + `seq`;block 事件带 `parentId`
    - **通知** `GET /api/v1/notifications` —— 1 通用 envelope,**entity 状态变更**(per-user 订)
      - Envelope: `{type: string, id: string, data: SlimPayload, conversationId?: string}`
      - **data 字段瘦身**:只送 `{action, versionId?, versionNumber?, ...}` 这种轻字段,**禁止塞完整 entity** — UI 拿通知 → 主动 GET 详情
      - 现期 type: `conversation` / `todo` / `mcp_server` / `skill` / `catalog` / `sandbox_env` / `function` / `handler`(开放词表,未来加新 type 字符串即可)
    - **锻造流** `GET /api/v1/forge` —— 4 events × 3 kinds,**trinity entity 锻造进度**(per-user 订)
      - 4 事件:`forge_started` / `forge_op_applied` / `forge_env_attempt` / `forge_completed`(封闭枚举)
      - 3 kinds:`function` / `handler` / `workflow`(封闭枚举)
      - payload 嵌套 `scope: {kind, id}` struct(复用 `domain/eventlog.Scope`),不平铺
      - 给 trinity create / edit / revert / delete 操作 + env-fix loop 推流;LLM tool 双写(forge bus + chat eventlog progress block)
    - 三流共用 Bridge pattern:per-user seq + replay buffer + Last-Event-ID 重连
    - **SSE 上限三条**,**永远不再加**(D-redo-5)。所有未来需求走 forge 流 + filter 或 Wails native event(打包阶段)
    - 详见 [`event-log-protocol.md`](documents/version-1.2/event-log-protocol.md) + [`service-contract-documents/events-design.md`](documents/version-1.2/service-contract-documents/events-design.md) + [`adhoc-topic-documents/forge_redesign/07-notifications-and-eventlog.md`](documents/version-1.2/adhoc-topic-documents/forge_redesign/07-notifications-and-eventlog.md)
15. **E2 协议演进规则**
    - 事件日志:新事件类型 / block 类型必须先改 `event-log-protocol.md` 再加 code(封闭枚举)
    - 锻造流:新 event 类型 / 新 scope.kind / 新 operation 必须先改 `07-notifications-and-eventlog.md` 再加 code(封闭枚举)
    - 通知:新 entity type 加字符串即可(开放词表);**所有 type 的 data 字段必须瘦身**(只 ID + 小字段,完整 entity 走 GET)
    - 路由 / demux:事件日志按 `parentId` 递归 + client 按 `payload.conversationId` 分派;通知 + 锻造流广播给同 user 所有订阅者,client 按 type / scope 过滤

## 代码规范（S 系列）

- **S3 错误不吞**：`_` 忽略必须带注释说明原因；**严禁用"静默跳过"掩盖失败**——若某功能在当前环境不可用，必须让调用者看到错误或在文档/启动日志里明确说明，而不是用 `_ = err` 藏起来。隐藏的错误会在意想不到的地方爆发（例：FTS5 虚拟表没建成但触发器建成了，INSERT 时才炸）
- **S5 长度参考线（仅为提示，非强制）**：~250-500 行单文件、~60 行单函数作为"该不该回头看一眼是否还内聚"的烟雾报警。**可读性、可扩展性、人的理解优先于行数**——`main.go` 的 DI 装配、SSE 状态机、单 domain 的 Service、AST 解析这种本就该长，强行拆分反而损害理解。**仅当超长伴随"职责模糊 / 显然拆得开 / 子段一段段都能起出独立名字"时才该拆**
- **S6 handler 薄度参考**：handler 只做"解 JSON → 调 service → 写 envelope"是目标形态；~20 行是提示线。SSE 协议设置 / multipart 解析 / dev 端点等天然偏长，不强制硬切。**看的是"做没做业务逻辑"，不是行数**——业务校验 / SQL 拼接 / 状态判断出现在 handler 才是真违规
- **S8 SQL 只在 `infra/store/` 和 `infra/db/`**：其他层出现 SQL 都是违规
- **S9 context 传播 + detached context 模式**：每个跨层调用传 `ctx`。**例外**：终态写入（必须落库的最后一步，例如取消流后 `writeDB(fatal=true)` 写 assistant 消息）必须用 `reqctxpkg.SetUserID(context.Background(), uid)` 创建 detached context——否则上游 cancel 会让终态写失败，消息丢失
- **S10 结构化日志**：用 **zap**（dev 彩色 / prod JSON）。**同步原语不自己打 log**（store / tester 等由调用者决定），**异步或 fire-and-forget 必须打**（events bridge、recover middleware）
- **S11 注释规范** — 见 §S11 详节
- **S12 包结构** — 见 §S12 详节
- **S13 包命名** — 见 §S13 详节
- **S14 📌 文档同步纪律** — 见 §S14 详节（**最高优先级**）
- **S15 ID 生成统一**：业务 ID 一律 `<prefix>_<16hex>` 格式（前缀按 domain 取，如 `aki_` apikey / `mc_` model config / `cv_` conversation / `msg_` message / `att_` attachment / `blk_` block / `f_` forge / `fv_` forge version / `tc_` test case / `fe_` forge execution（Phase 5 起，统一替代旧 `frh_`/`fth_` 两表）/ `b_` forge test 批跑 batch / `td_` todo（对话级 TODO，2026-05-05 由原 `tk_` task 改名）/ `bsh_` Bash 后台 shell 进程 / `sar_` subagent run / `smm_` subagent message / `fnenv_` function venv / `hdenv_` handler venv（每个 sandbox 消费者用各自前缀给 sandbox 当不透明 string，per D-redo-8）)；8 字节从 `crypto/rand` 取，**`rand.Read` 失败必须 panic**——熵源损坏继续会生成碰撞 ID。所有 `newID()` 函数遵守此格式（实现统一在 `pkg/idgen.New(prefix)`）
- **S16 错误包装格式**：上抛错误用 `fmt.Errorf("<pkg>.<Method>: %w", err)`，sentinel 在最里层。例：`apikeystore.List: missing user id in context`。**禁止**裸 `errors.New` 套娃丢失原 sentinel；**禁止**自创新前缀代替 `%w` 包装。`errors.Is` 必须能从最外层 unwrap 到 sentinel
- **S17 errmap 单一事实源**：每个会到达 handler 的 sentinel 必须登记到 `transport/httpapi/response/errmap.go::errTable`——**包括** `pkg/` 和 `infra/` 中跨层使用的（如 `reqctxpkg.ErrMissingUserID` / `cryptoinfra.ErrUnsupportedVersion`）。未登记的 sentinel 会触发"unmapped domain error" ERROR 日志，污染烟雾报警
- **S18 Tool 接口规约** — 见 §S18 详节
- **S21 事件流契约 invariants**（事件日志协议必守）：
  - `block_start.parentId` 必须先于本事件出现过——可以是 message ID（顶层 block 路径）或更早的 block ID（嵌套）；frontend 路由按"先查 blocks Map 再查 messages Map"。任何 dangling parentId 都是 producer bug
  - `block.status` 流转单向：`streaming` → 终态 (`completed` | `error` | `cancelled`)，不可回退。重复 stop 或 stop 后 delta 都是 producer bug
  - `message.status` 同上 4 值流转
  - 同 conversation 内 `seq` 严格全局单调递增（DB UNIQUE(conversation_id, seq) 强制）；frontend 按 seq 顺序消费保证 wire 顺序还原
  - 同 block 的 deltas 按 seq 顺序 append-only——前端永不重写、永不重排
  - tool_call block 的 ID 复用 LLM 自带的 tool-call ID（如 `tc_xxx`）——不走 §S15 prefix 约定（LLM 不知道我们的 ID 体系）；其他 block 用 `idgenpkg.New("blk")`
- **S20 📌 禁止"留下次"无理由（最高优先级）**：发现 bug / 缺陷 / 风险时**默认必须当场修**。声明"留下次"必须**同时满足**两条：(a) **结构性硬约束**——例如必须先扩公共 struct / 必须先改未上线模块 / 必须等外部依赖 / commit 已经超大需要分包；(b) **当场清晰说明**——commit message 或 progress-record dev log 里写明"为什么不能现在修"+"修复需要的前置条件"+"用户场景下会怎么爆"。**禁止理由**：(1)"想分 commit 别堆太多"——commit 大小不是借口，宁可一个大 commit 也不漏 bug；(2)"目前没人撞到"——只要可触发就该修；(3)"我懒"——别说，去修。**审查机制**：每次说出"留下次 / TODO / 改天 / 之后修"的瞬间，必须自检 (a) + (b)，**不通过就立刻动手**。**触发场景**：风险表里出现"⏳ 待修"那一栏；任何"以后再说"措辞；任何"知道在哪知道怎么修就行"的自我安慰式收尾。

- **S19 Dev log 节制**：`progress-record.md` 每条 dev log 1-2 句、~30-100 汉字，跟 Phase 0-2 早期条目同密度。保留：日期标签（`[refactor]` / `[fix]` / `[doc]` 等）/ 模块 / 关键数字（测试数 / 文件数 / 端点数）/ 一句结论。砍：实现细节、设计权衡 why/how、踩坑过程、命名漂移记录——这些归 git log / commit message / 设计文档。**Dev log 是日期索引，不是工程档案**——长篇是噪音，密度过高反而难找历史。
  - **超 100 字回头砍**——多半是把"做了什么"写成了"为什么这么做"，把"为什么"删了通常立刻就回到 100 字以内
  - **超 200 字硬上限**——压不下去就**裂成多条**（一条做多件事时 split 成多行表格），每行专注一个 [tag]
  - **设计文档内容禁入 dev log**——若新建/重写了 `documents/` 下某个 .md，dev log 只写 `[doc]` + 文件路径 + 一句话功能（"完整方案见该文件"），具体决策 / phase 划分 / 风险缓解全留在那个文档里
  - **文档同步是默认要求，不必每条 dev log 都列**——只在"这条改动联动了 N 处文档"这种异常情况才提；常规 S14 联动属于工程纪律，不是新闻

## 测试（T 系列）

- **T1 测试命名**：`Test<Function>_<Scenario>` 格式。`<Scenario>` 描述被测条件而非动作（`TestListProviders_Contains11` ✅ / `TestListProviders_ShouldWork` ❌）
- **T2 数据库测试用 in-memory SQLite**：`dbinfra.Open(dbinfra.Config{DataDir: ""})` 建内存 DB，每个 test 独立，无需清理。避免依赖文件系统 / 外部进程
- **T3 外部依赖测试用环境变量门控**：调真实 LLM API / 网络等的集成测试，必须用 `os.Getenv("DEEPSEEK_API_KEY")` 等检查 + `t.Skip` 跳过，**不允许默认跑**——否则离线 / CI 必红
- **T4 删导出符号必搜测试引用**：`deadcode` 工具默认不扫测试代码，所以"看起来死的"导出函数可能正被契约测试用。删之前 grep 一遍 `_test.go` 确认。**反之**，如果导出符号确实只用于测试，加注释说明"生产不调用，仅测试契约用"避免再被误判（例：`ListProviders` / `ListScenarios`）
- **T5 大功能模块完工必加 pipeline 测试 + 必须幂等**：每个**大功能模块**——一个 domain 的 CRUD 闭环 / 一个跨域流程 / 一个 SSE 事件家族 / 一个新 system tool 家族——完工时必须在 `backend/test/<domain>_pipeline_test.go` 追加端到端测试，用 `harness.New(t)` 真起 in-process backend（真 Bridge / 真 LLM / 真 sandbox / 真 SQLite），HTTP 驱动 + SSE 观测对真实 wire 行为做断言。运行入口 `make test-pipeline`（自动 source `.env` + `-tags=pipeline`），单测套件不进。**幂等是硬要求**——同一个 test 任意次数重跑结果必须一致：(a) 默认 `harness.New(t)` 每次拿全新内存 SQLite，天然隔离；(b) 若 test 故意复用 harness 跑多步骤，每步开头自己用 `h.DB.Exec("DELETE FROM ... WHERE ...")` 显式清理；(c) 涉及外部状态（文件系统 / 长生效的 LLM 上下文 / 外部 API 副作用）的，test 末尾或 `t.Cleanup` 兜底回滚；(d) **任何 test 不得依赖前一次运行残留**——这条破了 CI 红一片你都查不到原因。**触发场景**：完成 Phase X / 大重构 / 新 endpoint family 后，"端到端跑通一遍"是 acceptance criteria 的一部分，不是 nice-to-have
- **T6 Pipeline 默认 fake LLM；依赖项缺失时优雅 skip**：`backend/test/` 下的 pipeline 测试默认用 `FakeLLMServer`（不依赖外网）。**需要真实 LLM** 的测试：(a) 函数名加 `Live_` 前缀（如 `TestChat_Live_ReasoningModel_BlocksSeparate`）；(b) 开头调 `RequireDeepSeekKey(t)` gate——`DEEPSEEK_API_KEY` 未设时 skip（`make test-pipeline` source `.env`，有 key 则自动跑）。**需要真实 sandbox v2** 的测试（D2 后）：要求 `backend/internal/infra/sandbox/mise/<goos>-<goarch>/mise` 存在；缺则 skip，跑 `make resources` 拉一份。fake LLM 注入点：`harness.New(t, WithFakeLLMBaseURL(fake.URL()))` + `SeedDeepSeek(t, "fake-key")`

---

# §S11 注释规范（双语 + 节制）

所有 `backend/` 代码注释必须遵守。**核心原则：每个导出符号一行说明 OK；展开成段 + 心路历程不行。**

## 1. 双语格式

- **包/类型/函数** 的 godoc 必须**英文在前、空行、中文在后**
- 简单符号（≤ 2 行 godoc）允许 1+1 直译——成本低、grep 命中率高
- 多行段（≥ 4 行）的中文段必须有信息增量，不能是英文段的字面翻译；写不出新信息就删掉中文段，单留英文

```go
// InjectUserID stamps DefaultLocalUserID into ctx (Phase 2 simplified auth).
//
// InjectUserID 给 ctx 塞入 DefaultLocalUserID（Phase 2 简化 auth）。
func InjectUserID(next http.Handler) http.Handler { ... }
```

## 2. 必须写

- ✅ **Package doc**（≤ 4 行）：一句话讲清包职责
- ✅ **导出符号 godoc**：类型 / 函数 / 常量 / 变量——**默认 1+1 行**
- ✅ **Non-obvious 的 WHY**：陷阱、跨字段不变量、并发约束、踩过的坑
- ✅ **行为契约**：错误语义、生命周期、零值含义

## 3. 禁止写

- ❌ **设计意图段落** ("Design intent / Architecture / Lifecycle")：搬去 `documents/version-1.2/service-design-documents/<domain>.md`，package doc 留一句 "See: <path>" 即可
- ❌ **决策叙事**："we deliberately did NOT…"、"This is intentional because…"、"(decision D11)"、"Phase 3 currently only wires X…"——只留结论，过程去 progress-record / git log
- ❌ **历史/重构来源**："2026-04-27 重构后..."、"以前是 X，现在改成 Y"——git log 已经记了
- ❌ **"为什么不选 A" 对比说明**：去设计文档
- ❌ **章节横幅** `// ── X ──`：靠 `type/var` 分组 + 空行；要靠横幅切才能读 = 该拆文件
- ❌ **架构哲学 / 团队约定 / 规范解释**：搬到本文件
- ❌ **机械复述**：`// Set name sets the name`
- ❌ **跑题猜测**："未来可能..."（除非真是 TODO）

## 4. 长度硬上限

超出回头砍，砍不下去就拆文件 / 搬设计文档。

| 注释类型 | 上限 | 触发拆分 |
|---|---|---|
| Package doc 段（中英合计） | ≤ 4 行 | > 4 行 → 搬 design doc |
| 简单导出符号 godoc | 1+1 行（中英各一） | — |
| 复杂契约 godoc（多返回值 / 错误 / 并发 / 生命周期） | 中英各 ≤ 4 行 | 中英合计 > 8 行 → 红线 |
| 内联注释 | 1 行 | 非平凡业务规则 ≤ 3 行 |

**红线**：任何注释段超过 8 行 = 红线，要么拆代码、要么搬 documents/。

## 5. 字段 godoc 节制

- **不写**：自明字段。`Log *zap.Logger` / `DB *gorm.DB` / `Mu sync.Mutex` / `Port int`
- **1+1 行**：字段名有歧义、零值有陷阱、有跨字段不变量、或字段角色与类型不一一对应（例：`APIKeyService *apikeyapp.Service` 这种"实现 X 接口给 Y 用"的角色信息）
- **多行**：极少数"字段本身就是契约"的情况（例：`EventsBridge` 解释发布订阅契约）

## 6. 测试文件

- 双语规则一致；长度上限放宽到 1.5×
- "为什么测这个 corner case" 非显然时写一行，显然时省略
- 测试名 `TestX_Scenario` 自解释 = 不写 godoc

## 7. 迁移姿态

- 本规矩生效后，老文件按 v1 写的"长篇 + 心路历程"视作历史债务
- 渐进清理：跟代码改动顺手清理；常规情况不专门刷新整个 backend
- 例外：本轮（2026-05）做一次专项清理，按 `pkg/ → domain/ → infra/ → app/ → transport/ → test/` 顺序逐域走

---

# §S12 包结构（domain 平铺，按概念拆文件）

每个 domain 的代码**平铺到包根目录**，**禁止子目录**。文件按"概念 / feature"拆分，**禁止**按"种类"拆分。

## 1. 拆错 vs 拆对

```
❌ 错误：按 "kind of thing" 拆
domain/chat/
├── types.go        (全部 struct)
├── errors.go       (全部错误)
├── constants.go    (全部常量)
└── interfaces.go   (全部接口)

✅ 正确：按 "concept / feature" 拆
domain/chat/
├── chat.go         Conversation 核心 + godoc
├── message.go      Message struct + 相关常量/错误
├── stream.go       流式输出契约
└── repository.go   存储接口
```

每个文件还是混合 types + 常量 + errors + 小 interface——只要它们围绕**同一个子概念**。
对照 stdlib：`net/http/request.go` 同时定义 `Request` 类型、它的方法、相关常量、相关错误。

## 2. 主文件命名

主文件用**包名**（如 `apikey.go`、`chat.go`）。包级 godoc **只写在主文件顶部**；其他文件的文件头注释要和 `package X` 之间留空行。**禁止**单独建 `doc.go`。

**三层统一**：domain / app / infra/store 全部三层都遵守。

```
domain/apikey/apikey.go       ← 主文件
app/apikey/apikey.go          ← 主文件（不叫 service.go）
infra/store/apikey/apikey.go  ← 主文件（不叫 store.go）
```

例外：有独立接口 + 独立具体类型 + 独立测试的子组件可以单独一个文件（如 `tester.go`）。
仅"Service 实现某接口"或"小工具函数"这类合并进主文件，不单独建文件。

## 3. 文件长度

- < 500 行 舒服
- 500-1000 行 可接受（只要概念内聚）
- 1000+ 行 该拆，但拆**文件**不拆包

## 4. 何时拆子包

两个硬条件**同时满足**才拆：
1. 有独立的**词汇体系**
2. 至少 **10+ 个文件** 围绕这个子词汇

### 例外：tool framework meta-namespace

`internal/app/tool/` 是 tool 框架的 meta-namespace（不是业务 domain），允许按 tool 家族嵌套子包：

```
app/tool/
├── tool.go          ← Tool 接口、ToolEvent、ctx helpers
├── forge/           ← user-forged-tool 系统工具（search/get/create/edit/run）
├── filesystem/      ← Read/Write/Edit/Glob/Grep/LS
├── shell/           ← Bash
└── web/             ← WebSearch/Fetch
```

理由：每个家族有独立词汇体系（forge ≠ filesystem ≠ shell），对外靠相同的 `Tool` 接口契约统一。仅本目录例外，其他 domain 仍遵守"平铺 + 拆文件不拆包"。

## 5. 共享纯工具

跨 domain 用的纯函数（无业务、无 infra 依赖）放 `internal/pkg/<name>/`（如 `pkg/reqctx/`、`pkg/pagination/`）。

## 6. 辅助注册表归属

`providers.go`（provider 注册表）这类"纯配置 + 查询函数"的文件，放在**消费它的层**，而非 domain 层。判断标准：**domain 层自身使用 → 放 domain；仅 app 层消费 → 放 app**。

例：`apikey/providers.go` 的所有消费者都在 app 层，故放 `app/apikey/providers.go`。

---

# §S13 包命名（三层同名 + 调用方别名）

**核心规则**：**所有**从 `internal/` 导入的包**必须使用别名**，格式为 `<name><role>`。无别名视为违规。

## 1. 别名后缀规则（按目录）

| 目录 | 后缀 | 示例 |
|---|---|---|
| `internal/app/<name>/` | `app` | `apikeyapp`, `chatapp`, `convapp` |
| `internal/domain/<name>/` | `domain` | `apikeydomain`, `chatdomain`, `errorsdomain` |
| `internal/infra/<name>/`（非 store）| `infra` | `llminfra`, `dbinfra`, `loggerinfra`, `cryptoinfra`, `memoryinfra`, `sandboxinfra`, `chatinfra` |
| `internal/infra/store/<name>/` | `store` | `apikeystore`, `chatstore`, `convstore` |
| `internal/pkg/<name>/` | `pkg` | `reqctxpkg`, `paginationpkg` |
| `internal/transport/httpapi/<name>/` | `httpapi` | `responsehttpapi`, `middlewarehttpapi`, `handlershttpapi`, `routerhttpapi` |
| `internal/app/tool/<sub>/`（嵌套子包，§S12 例外位置）| `tool`（用 `<sub>tool` 形式）| `forgetool`, `fstool`, `shelltool`, `webtool` |

> `<name>` 取包路径最后一段（允许约定缩写，如 `conversation` → `conv`）。
> 嵌套子包别名 = `<子名><父名>`，与父级 `toolapp` 区分。例：`tool/forge/` → `forgetool`（系统 tool 那侧）vs `app/forge/` → `forgeapp`（user-forge service）一眼分辨。

## 2. 包内统一名

每个 domain 在 **domain / app / infra/store** 三层的包名都用 domain 单名（如 `apikey`）：

| 目录 | 包声明 |
|---|---|
| `internal/domain/apikey/` | `package apikey` |
| `internal/app/apikey/` | `package apikey` |
| `internal/infra/store/apikey/` | `package apikey` |

## 3. 调用方按角色起别名

```go
import (
    apikeydomain  "github.com/sunweilin/forgify/backend/internal/domain/apikey"
    apikeyapp     "github.com/sunweilin/forgify/backend/internal/app/apikey"
    apikeystore   "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
    dbinfra       "github.com/sunweilin/forgify/backend/internal/infra/db"
    reqctxpkg     "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
    paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
)
```

## 4. 接口定义位置

- **接口定义**在 domain 层（如 `apikeydomain.Repository`、`apikeydomain.KeyProvider`）
- **store 实现** Repository（被 service 消费）
- **app/Service 实现** KeyProvider（被其他 domain 消费）
- **跨 domain 消费**只通过 port 接口，**禁止暴露 entity**

## 5. 为什么这样定

- **强制别名**：`reqctx` / `db` / `logger` 等包名太通用，无别名时与变量名冲突
- **统一后缀**：grep 一个符号名，别名立刻告诉你它在哪一层
- **包内统一名**：包内部代码读起来"就是 apikey"，不用长前缀

---

# §S14 📌 文档同步纪律（最高优先级）

对应设计原则 #7。**全项目最硬的纪律**，比代码风格更严。理由：文档是新决策的唯一参考；**滞后的文档 = 集体性质的 bug**。

## 1. 三处联动

代码改动涉及以下任何一条，**三处都要动**：

| 代码变动类型 | 联动位置 |
|---|---|
| 新 entity / 表 / struct 字段 / 约束 | ① `service-design-documents/<domain>.md` 领域模型 + 数据库表<br>② `service-contract-documents/database-design.md` 索引行<br>③ `progress-record.md` dev log |
| 新 sentinel / errmap 行 | ① `service-design-documents/<domain>.md` 错误码<br>② `service-contract-documents/error-codes.md` 表格行<br>③ `progress-record.md` dev log |
| 新 endpoint / req-resp 形状 / path 变 | ① `service-design-documents/<domain>.md` HTTP API 详细<br>② `service-contract-documents/api-design.md` 端点表<br>③ `progress-record.md` dev log |
| 新事件 / struct 改 / 过滤 key 改 | ① `service-design-documents/<domain>.md` 事件<br>② `service-contract-documents/events-design.md` 表格行<br>③ `progress-record.md` dev log |
| 方法签名改 / 新方法 / 接口变 | ① `service-design-documents/<domain>.md`<br>②（仅当影响对外入口才动 contract 文档） |
| 新/变跨 domain 依赖 | ① `service-design-documents/<domain>.md` 对外 API + 消费方 + 协作图<br>② 受影响的 `<other-domain>.md` 也要改 |

## 2. 子任务推进 checklist

每个子任务做完：

- [ ] `service-design-documents/<domain>.md` 实现清单勾 ✅
- [ ] 改了 API/schema/error → contract 文档对应表格行更新 ✅
- [ ] `progress-record.md` 加 dev log（**含具体做了什么 + 测试数 + 新规范/决策**）
- [ ] 新原则/规范变动 → 加到本文件相应章节

## 3. domain 完工 checklist

- [ ] `service-design-documents/<domain>.md` 整体过一遍逐字段匹配代码
- [ ] `service-contract-documents/*.md` 该 domain 行从 ⬜ 改 ✅ / 🔄
- [ ] `progress-record.md` 更新 Phase 子任务表 + 完工日志
- [ ] 引入新跨域模式 → 更新 `backend-design.md` Architecture 树

## 4. 发现文档与代码不符

- **立刻停下手里的事修文档**（哪怕正在写新 domain）
- 修完记 dev log，类别标 `[doc-fix]`
- 反思缺了什么 checklist 入口

## 5. 审查文档套路

开始新 domain 前以"我要从文档里找指南"视角读：
- 读本文件找规范
- 读对应 `<domain>.md` 详设计
- 读 `service-contract-documents/*` 确认索引一致
- 读 `progress-record.md` 找"刚刚别的 domain 用了什么套路"

发现"和我脑子里的不一致"或"少了一块"，**立刻修文档再继续**。

## 6. 为什么最高优先级

- 单次漏改成本小（几行字），积累成本巨大（后续 domain 建立在错误信息上）
- 项目边做边讨论，规范随项目演化；文档是**持久保存演化结果** 的唯一地方
- 代码告诉"是什么"，文档告诉"为什么 / 怎么连"——后者失真整个协作就失血
- 单人项目，**对未来的自己诚实 = 给未来的自己减负**

---

# §S18 Tool 接口规约

LLM 调用的 system tool 实现 `app/tool.Tool` 接口。**9 个方法全必填，无 BaseTool 嵌入**——每个 tool 的元数据全部显式声明，可 grep 可读。

## 1. 接口结构

```go
type Tool interface {
    // ── Identity（3 个）──
    Name() string                              // LLM 看到的工具名（如 "search_forges"）
    Description() string                       // 说明工具用途
    Parameters() json.RawMessage               // 输入 JSON Schema（不含 summary / destructive / execution_group）

    // ── 静态元数据（3 个，固有属性）──
    IsReadOnly() bool                          // 仅文档/语义参考；不再驱动并发调度
    NeedsReadFirst() bool                      // 操作的文件是否必须 session 内 Read 过（Phase 5 Edit/Write）
    RequiresWorkspace() bool                   // 路径是否需经 PathGuard 黑名单守卫（Phase 5）

    // ── args-dependent 钩子（2 个）──
    ValidateInput(args json.RawMessage) error                                          // 进 Execute 前校验
    CheckPermissions(args json.RawMessage, mode PermissionMode) PermissionResult       // Allow / Deny / Ask

    // ── 主入口（1 个）──
    Execute(ctx context.Context, argsJSON string) (string, error)                      // argsJSON 已剥除三个标准字段
}
```

> ⚠️ **没有 `IsConcurrencySafe(args)` 方法**——并行调度由 LLM 自报的 `execution_group` 标准字段驱动（见 §2 + §4），tool 不再参与判断。

## 2. 标准注入字段

框架在每个 tool 的 Parameters schema 自动注入**三个** LLM-facing 字段：

| 字段 | 类型 | 必填 | 用途 |
|---|---|---|---|
| `summary` | string | ✅ 必填 | LLM 一句话描述本次调用在干啥（"Searching forges for csv parsing"）|
| `destructive` | bool | 可选默认 false | LLM 自报本次调用是否可能不可逆破坏；UI 据此显示警示徽章 |
| `execution_group` | int (≥1) | 可选 | LLM 自报并行 batch 提示——同 group = 并行；不同 group = 升序串行；缺失 = 框架自动分配唯一 group 排在所有显式 group 之后（独自串行） |

三者由 framework 在传给 `Execute` 前剥除（`StripStandardFields` 返回 `StandardFields` struct），存进 `chatdomain.ToolCallData` 的一等字段（`Summary` / `Destructive` / `ExecutionGroup`）。**tool 实现的 Parameters() 不得包含这三个字段名**——冲突时 framework panic。

## 3. 推流约定（事件日志协议 / Phase 2+ 推荐）

Tool 实现要推 SSE 时，**优先用 `pkg/eventlog` 的 Emitter**——从 ctx 拿，自动继承 conversationId + parentBlockId（runOneTool 已用 `WithParentBlockID(tc.ID)` 包了 ctx，工具内 emit 自动挂 tool_call 父下）：

```go
import (
    eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
    eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

func (t *MyTool) Execute(ctx context.Context, args string) (string, error) {
    em := eventlogpkg.From(ctx) // never nil — falls back to no-op
    progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{"stage": "downloading"})
    em.DeltaBlock(ctx, progID, "234 MB / 1.2 GB\n")
    em.DeltaBlock(ctx, progID, "612 MB / 1.2 GB\n")
    em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)
    return "done", nil
}
```

**legacy `bridge.Publish(ctx, convID, eventsdomain.X{...})` 仍可用**——dual-write 期共存，新代码优先走 Emitter。

**不引入更高的 emit 抽象**——心智统一优先；所有推流走 Emitter 6 方法或 Bridge 的直接 Publish，不再抽象第三层。

## 4. runTools 分批语义（按 LLM 自报的 execution_group）

`chat/tools.go::runTools` 调 `partitionByExecutionGroup(calls)` 分批，纯按 LLM 在每个 tool call 的 `execution_group` 字段决定：

- **同 group = 并行 batch**：LLM 给多个调用相同 group 号 = 它担保它们之间无依赖、无共享可变状态
- **不同 group = 串行**：按 group 号升序执行，前一个 group 全跑完才进下一个
- **缺省（≤0）= 自动分配唯一 group ≥1000**：每个未标 group 的 call 各自独占一个 batch（独自串行，且**排在所有显式 group 之后**）。这是 fail-safe 默认——LLM 不主动声明并行就不并行

例：LLM 同 turn 发 `[A:1, B:1, C:0, D:2, E:0]`：
→ 分配后 `[A:1, B:1, C:1000, D:2, E:1001]` （`maxExplicit=2`，autoBase=max(3, 1000)=1000）
→ 排序 `[1, 2, 1000, 1001]`
→ 4 个 batches: `[A, B 并行] [D 单跑] [C 单跑] [E 单跑]`

## 5. 进 Execute 前的钩子链

每次 tool 调用顺序：

1. `ValidateInput(args)` — 失败转失败 tool_result，不进后续
2. `CheckPermissions(args, mode)` — Deny 转失败 tool_result；Ask 当前阶段当 Allow
3. `Execute(ctx, args)` — 主体；返 error 转失败 tool_result（LLM 看到错误文本）

## 6. 子包结构

`tool/` 是 framework meta-namespace，**§S12 例外允许嵌套子包**（按 tool 家族）：

```
internal/app/tool/
├── tool.go         ← Tool 接口、PermissionMode/Result、injectStandardFields/StripStandardFields/ToLLMDef
├── forge/          ← user-forged-tool 系统工具（5 个）
│   ├── forge.go    ← ForgeTools() 工厂 + buildClient + extractJSON / extractCode / streamCode / resolveAttachments
│   ├── search.go   ← SearchForge
│   ├── get.go      ← GetForge
│   ├── create.go   ← CreateForge
│   ├── edit.go     ← EditForge
│   └── run.go      ← RunForge
├── filesystem/     ← Read/Write/Edit/Glob/Grep/LS（Phase 5）
├── shell/          ← Bash（Phase 5）
└── web/            ← WebSearch/Fetch（Phase 5）
```

调用方按 §S13 嵌套子包别名规则导入：`forgetool` / `fstool` / `shelltool` / `webtool`。

## 7. 例：Search 实现 9 方法（最简单 readOnly tool）

```go
type SearchForge struct{ ... }

// Identity
func (t *SearchForge) Name() string                  { return "search_forges" }
func (t *SearchForge) Description() string           { return "Search the user's forge library..." }
func (t *SearchForge) Parameters() json.RawMessage   { return json.RawMessage(`{...}`) }

// Static metadata
func (t *SearchForge) IsReadOnly() bool        { return true }
func (t *SearchForge) NeedsReadFirst() bool    { return false }
func (t *SearchForge) RequiresWorkspace() bool { return false }

// Args-dependent hooks
func (t *SearchForge) ValidateInput(json.RawMessage) error    { return nil }
func (t *SearchForge) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
    return toolapp.PermissionAllow
}

// Main
func (t *SearchForge) Execute(ctx context.Context, args string) (string, error) { ... }
```

## 8. 静态元数据 3 字段对照表（实战示例）

> **重要**：`NeedsReadFirst` / `RequiresWorkspace` 当前是**文档性元数据**——框架不强制，靠每个 tool 在 Execute 内部自查（must-Read-first 守卫读 `AgentState.SeenFiles`，PathGuard 守卫调 `pathguard.Allow`）。新加 tool 时如果声明 true 但忘了在 Execute 内 check，元数据就是撒谎，框架不会发现——务必自查。`IsReadOnly` 当前纯文档（execution_group 接管了并发调度），仅用于人读 / 未来 scheduler 提示。

| Tool | IsReadOnly | NeedsReadFirst | RequiresWorkspace | 备注 |
|---|---|---|---|---|
| `Read` | ✅ | ❌ | ✅ | 走 PathGuard；Read 自身是产生 SeenFiles 标记的源头，所以自己不需要 must-Read-first |
| `Write` | ❌ | ✅ | ✅ | 覆写时强制 must-Read-first；新建文件路径无需 |
| `Edit` | ❌ | ✅ | ✅ | 字面量替换；must-Read-first + size 失配检测外部修改 |
| `Grep` / `Glob` | ✅ | ❌ | ✅ | 走 PathGuard；只读 |
| `Bash` / `BashOutput` / `KillShell` | ❌（KillShell）/ ✅（BashOutput） / ❌（Bash） | ❌ | **❌** | **故意不走 PathGuard**——单用户本地场景下 Bash 是用户日常命令的代理，banned-list 没价值；`RequiresWorkspace=false` 是设计决策（详 `02-tools-deep/03-shell.md` 决策 D5，安全 trade-off）|
| `WebFetch` / `WebSearch` | ✅ | ❌ | ❌ | 网络 tool 不碰文件系统；SSRF 守卫在 Execute 内部（不通过 RequiresWorkspace）|
| `TaskCreate` / `TaskUpdate` | ❌ | ❌ | ❌ | 改对话级 to-do；纯 DB 操作 |
| `TaskList` / `TaskGet` / `AskUserQuestion` | ✅ | ❌ | ❌ | 只读 / 等待答案 |
| `create_forge` / `edit_forge` / `run_forge` | ❌ | ❌ | ❌ | mutate 用户 forge 库；走 sandbox |
| `search_forges` / `get_forge` | ✅ | ❌ | ❌ | 只读 |

简单 tool 的 5 个 boilerplate 方法每个 1 行。并行决策由 LLM 自报的 `execution_group` 完成，tool 不再做 `IsConcurrencySafe` 判断。

---

# 开发期工具纪律

不属于 S 系列（不是代码规范），是**操作教训**。每条都是踩过坑后立的。

- **`staticcheck ./...` 是提交前必跑项**——比 `go vet` 严格得多，能捞出 SA1029（ctx key 用 string）/ S1016（结构体可类型转换）/ U1000（未使用符号）等真问题
- **`deadcode ./cmd/server` 默认不扫测试**——会把"仅测试用的导出符号"误判为死代码（曾因此误删 `ListProviders` / `ListScenarios` 触发编译失败）。跑时带 `-test=true`
- **staticcheck 不认 `//nolint`**——要用 `//lint:ignore <code> <reason>` 指令才能正确抑制
- **项目内禁用 sed 批量改 import / 函数名**——BSD sed `\b` word boundary 不识别，`sed -i '' 's/\bX/Y/g' file.go` 会清空整个文件。已踩 2 次坑。批量改用 Edit 工具或具体上下文匹配
- **跨平台编译当 PR 阶段烟雾测试**：`GOOS=windows/linux/darwin go build ./...` 任一平台编不过都该立刻拦下。modernc.org/sqlite 之后这是 1 行命令的事

## 测试命令的正确选择（**不要直接 `go test ./...`**）

| 命令 | 用途 | 是否 source `.env` | 范围 |
|---|---|---|---|
| **`make test-unit`** | **改完代码默认跑这个**——纯函数 / in-memory SQLite 套件 | ❌ 不 source | `go test -count=1 ./... -skip TestIntegration_`（明确跳过所有 `TestIntegration_*` 集成测试） |
| **`make test-pipeline`** | 端到端套件（Phase 边界 / 大重构 / 新 endpoint family 后必跑） | ✅ source `.env` | `go test -count=1 -tags=pipeline ./test/...`；DEEPSEEK key 在则跑 Live_ 测试，否则 t.Skip；mise embed 在则跑真 sandbox |
| **`make test-console`** | 起 dev 服务器（不是测试，是手动验证用的入口） | ✅ source `.env` | `go run ./cmd/server --dev` + 自动开浏览器 testend 控制台 |

**为何不直接 `go test ./...`**：会跑到 `TestIntegration_*` 这种调真实 LLM API 的集成测试。`go test` 不自动 load `.env`，env 缺 `DEEPSEEK_API_KEY` 时这些测试 fail——但这是 **noise，不是真 regression**。`make test-unit` 用 `-skip TestIntegration_` 一刀切掉，测试输出干净。

**集成测试的规矩（§T3 的具体落地）**：所有 `TestIntegration_*` 必须用 `os.Getenv("DEEPSEEK_API_KEY")` 等 + `t.Skip` 门控，**禁止 hardcoded fallback key**——历史曾用 `"shabi"` 当兜底（自嘲 placeholder），导致 noise 失败被当成"基线"忽视，掩盖真实问题。Pattern：

```go
func requireKey(t *testing.T) string {
    t.Helper()
    k := os.Getenv("DEEPSEEK_API_KEY")
    if k == "" {
        t.Skip("DEEPSEEK_API_KEY not set; use `make test-pipeline` to load .env")
    }
    return k
}
```

---

# 项目特殊性（防止用通用 Go 习惯做错事）

- **单用户本地 + 同人写前后端** → 校验少、便利优先（见设计原则 #6）
- **已摆脱 Eino**（2026-04-27 重构），自有 LLM 客户端 `infra/llm`（OpenAI-compat + Anthropic 原生）
- **modernc.org/sqlite**（纯 Go），跨平台 build 一行命令；DSN 用 `_pragma=...` 语法
- **桌面端集成方式**：Wails 当窗口外壳 + 复用 httpapi（**不走** Wails native binding，详见 `desktop-packaging-notes.md`）
- **chat Block 模型** = 事件日志协议同 shape（messages 元数据 / message_blocks 流式内容；Block.Content 是裸文本 / Block.Attrs JSON 元数据）
- **双 SSE 协议**：`domain/eventlog` (per-conversation 5 events × 6 block types) + `domain/notifications` (global entity updates)；分别经 `/api/v1/eventlog` / `/api/v1/notifications` 暴露。Legacy `domain/events` 已删
- **subagent 数据**：sub-run 是统一 `messages` 行（attrs.kind=subagent_run + parent_block_id 指向占位 message-block）+ 该 message 在 `message_blocks` 的 blocks。无独立 `subagent_runs` / `subagent_messages` 表
- **测试基线**：~170 单测全绿；5 个 LLM 集成测试因 `DEEPSEEK_API_KEY` 环境失效，与基线一致，不算回归
- **`infra/sandbox` v2 捆绑 mise**（~25 MB binary），lazy install 各语言 runtime（python/node/rust/...）+ per-plugin 隔离 env（forge / mcp / skill / conversation 4 类 owner）。SQLite 双表 manifest（sandbox_runtimes + sandbox_envs）。dev 与 prod 都走 `go:embed` 把 mise binary 编进 binary——`make resources` 把 mise 拉到 `backend/internal/infra/sandbox/mise/<goos>-<goarch>/` 给 embed 用（默认仅当前平台，加 `ALL=1` 拉全 5 平台供 release pipeline）。devbox bootstrap 自动跑 `cd backend && go run ./cmd/resources`。**v1 dev resources** (`~/.forgify-dev-resources/` 的 uv + python-build-standalone) 已废弃；forge sandbox v1 在 D2-5 切到 v2 service 前会返 `ErrSandboxUnavailable`
