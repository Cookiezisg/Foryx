# Testend Rewrite — Backend Issues Log

> 2026-05-14 起,testend 全量重写过程中发现的后端问题 + 顺手修的记录。
> 原则:不影响产品核心思想的,直接修;影响的,在此记录但不动,留给独立 plan。

## 索引

(以下随发现逐条 append;每条结构:**发现** / **判断** / **行动** / **commit**)

---

## #1 `/dev/` 入口硬编码 `tester.html` — 与 Vite SPA 输出 `index.html` 不兼容

- **发现**:`backend/internal/transport/httpapi/handlers/dev.go::ServeIndex` 只读 `tester.html`。v2 testend 用 Vite,build 产物入口是 `index.html`,导致 `GET /dev/` 直接 404,即便 `/dev/static/assets/*` 静态文件 200 也无法引导前端启动。
- **判断**:小改、不影响产品核心思想;v1 tester 还存活一段时间(`backend/integration/tester.html`)需要兼容,故保留 fallback。
- **行动**:`ServeIndex` 改为遍历 `["index.html", "tester.html"]`,第一个能读到就用;两个都没就 404 + 提示 `make build-testend`。
- **commit**:待 U1 收尾时一起提。

## #2 in-memory DB 多连接 → 各连接拿一个空 schema → 并发 goroutine 看不到 migration 结果

- **发现**:`go run ./cmd/server --dev`(无 `--data-dir`)启动后,scheduler `RehydrateOnBoot` 报 `no such table: flowruns`,而 migration 阶段没报错。
- **判断**:`glebarez/sqlite` 用 `:memory:` DSN 时**每条 Go SQL connection 拿一个独立的内存 DB**。AutoMigrate 在 connection A 跑完后,scheduler `Start`、mcp `Start`、skill `Scan` 等并发 goroutine 从 pool 拿到 connection B/C/...,看到的全是空 schema。pipeline 测试不爆是因为 t.New 后基本是串行调用,大概率复用同一 conn。属于真后端 bug,影响 in-memory 模式下任何带后台 goroutine 的真 server boot。
- **行动**:`internal/infra/db/db.go::Open` 在 `cfg.DataDir == ""` 时显式 `SetMaxOpenConns(1)`,把内存模式锁到单连接。文件 DSN 不受影响(文件本身是共享 state)。
- **commit**:待 U1 收尾时一起提。

## #3 `dev_routes.go` 手维护清单 stale ≈ 1 整 phase (Plan 04-06 全部漏更新)

- **发现**:`/dev/routes` 仍列 `/api/v1/forges/...` 等 v1 forge 路径,而后端实际注册的是 trinity `/api/v1/functions`、`/handlers`、`/workflows` + Plan 05 `/flowruns`。`GET /api/v1/forge` (SSE 锻造流) 也漏。`{idAction}` dispatch 风格的端点(`:run` / `:revert` / `:call` / `:trigger` / `:test` / `:invoke` / `:install` / `:health-check`)很多漏标。
- **判断**:违反 §S14 文档同步纪律最严的一次——手维护清单天然会 drift。CLAUDE.md §S14 已要求每加 endpoint 同步 contract docs;`dev_routes.go` 也是事实源之一。
- **行动**:`internal/transport/httpapi/handlers/dev_routes.go::devRoutes` slice 整段重写:
  - 删全部 `forge.*` (v1 entity routes 已废)
  - 加 functions / handlers / workflows / flowruns / function-executions / handler-calls / `:trigger` / `:revert` / forge SSE `/api/v1/forge` / providers / catalog
  - 注释顶部加 grep 命令做"自检"
  - 一律标 `{idAction} dispatch → :xxx`,让阅读人知道是 dispatcher 而非独立 mux 行
- **后续注意**:每次往 `internal/transport/httpapi/handlers/*.go` 加/删 `mux.HandleFunc`,**同时** 更新这里。违反 = 文档 bug。CLAUDE.md §S14 表格已记。
- **commit**:待 U1 收尾时一起提。

## #4 `chat.Block.Attrs` REST 序列化为 JSON 字符串、SSE 序列化为对象 — 双形态不一致

- **发现**:`GET /api/v1/conversations/{id}/messages` 返的每个 Block 里 `attrs` 字段是 **JSON 字符串**(如 `"{\"toolCallId\":\"tc_...\"}"`,DB 列就是 text);而 SSE `block_start` 事件 payload 的 `attrs` 是**对象**(如 `{"toolCallId":"tc_..."}`,因 BlockStart Go struct 用 `map[string]any`)。同一 entity 同一字段,两条传输路径两种形态。
- **判断**:真后端 bug。前端要分两条路径解析,不科学。N3 字段 camelCase 已统一,attrs 的内部结构也应统一为对象。
- **影响**:testend `BlockView.vue` 渲染时读 `block.attrs?.toolName` / `toolCallId` / `summary` 等,字符串形态全 undefined,工具调用渲染会丢失元数据(显示 "(unknown tool)" 之类)。
- **行动**:**短期**:testend `stores/chat.ts::loadMessages` 加 `parseMaybeJSON()` 工具,把 REST 拿到的 string `attrs` 解析成对象,后端继续按现状。**长期**:`infra/store/chat` repo 层 GetMessages / ListMessages 出口处把 `Block.Attrs` 解析成 `map[string]any`,Block struct 改 `Attrs map[string]any \`json:"attrs"\`` ,GORM `serializer:json` 处理列读写。等做 U10 收尾或后续 chat domain 重写时一起改。
- **commit**:短期前端修复待 U1 收尾时一起提;后端长期方案另开 issue。

---

## 收尾总结(2026-05-14)

V2 testend 完工。U1-U10 全部 done。33 个 view 路由、6 个 nav section、4 列固定布局、3 SSE 流共享订阅、典型 chat 路径已经过 DeepSeek 实测(create conv → POST messages → SSE 消息+块流式 → 全部状态 completed)。`npm run typecheck` 干净通过,`npm run build` 干净,产物 `dist/index-*.js`(151 KB,gzip 55 KB)+ `dist/WorkflowDetail-*.js`(450 KB,含 cytoscape)。

**后端共改动 3 个文件 + 1 个新文档**(全部在不影响产品核心思想的前提下):
1. `backend/internal/transport/httpapi/handlers/dev.go::ServeIndex` — index.html / tester.html 双 fallback(issue #1)
2. `backend/internal/infra/db/db.go::Open` — `:memory:` DSN 锁 MaxOpenConns(1)(issue #2)
3. `backend/internal/transport/httpapi/handlers/dev_routes.go::devRoutes` — 整段重写为 trinity + Plan 05 现状(issue #3)
4. 新增本文档 `documents/version-1.2/testend-rewrite-backend-issues.md` 记 4 个 issue

**待办(留给后端 chat domain 重写时一起做)**:Issue #4 —— 把 `chat.Block.Attrs` 在 REST 出口处也 deserialize 成对象,统一两条传输路径。当前 testend 用 `parseMaybeJSON` 客户端兜底,够用但不优雅。

---

## V3 (2026-05-27 — React rewrite) — issue log

### #5 sse.ts comment "5 × 6 block types" stale

- **发现**: V2 sse.ts JSDoc 注释说 5 events × 6 block types,实际 5/14 后 compaction block 加入是 7。
- **判断**: 注释 drift。
- **行动**: V3 sse.ts 注释直接说 7 block types;V2 注释 drift 不再修。

### #6 V2 IDPrefix 联合类型不全 + 含已废条目

- **发现**: V2 types/api.ts::IDPrefix 缺多个 + 有已废条目。
- **判断**: V3 不再维护自己的 IDPrefix 列表——id 前缀语义嵌在 frontend entity 类型每个 id 字段。
- **行动**: V3 testend 通过 vite alias deep-import frontend entity types,自动跟随 backend §S15 定义。

### #7 dev_routes.go 手维护清单 drift(根治版)

- **发现**: V2 testend-rewrite issue #3 已记录,5/14 后又 drift。手维护清单天然会 drift。
- **判断**: 需要根本性改造。
- **行动**: P0 引入 `router.Recorder` 包装 *http.ServeMux;HandleFunc/Handle 调底层 + 记录 Route。/dev/routes 读 Recorder.List()。**根治**。
- **commits**: bdf9dd3 (Recorder), d56a6eb (Registrar widening 29 handlers), f96685d (/dev/routes wires to Recorder).

### Status of #4 (Block.Attrs 双形态)

V2 issue #4 (REST attrs JSON 字符串 vs SSE 对象) 仍未在后端根治。V3 testend 客户端 workaround:hook `useNormalizedBlock(block)` 在 BlockView 渲染前 parseMaybeJSON。长期后端 fix 留独立 plan(chat domain 重写时一起做)。

---

### V3 收尾总结(2026-05-27)

P0 backend cleanup(10 commits)+ P1 scaffold(1)+ P2 infrastructure(3)+ P3 views(6,44 views)= 20 commits on main。后端净改:删 dev_routes.go,加 recorder.go + handlers/registrar.go,删 3 dev handler + 2 字段 + 1 flag,rename 1 flag,大约 35 文件触动。前端净改:加 errorCodes.ts + errorMap.ts 重构。testend 完全重写:Vue 拆除,React 重建,~50 新 TS/TSX 文件。

