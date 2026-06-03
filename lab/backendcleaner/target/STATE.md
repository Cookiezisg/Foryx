# backendcleaner — STATE（单一状态源）

> 进度的**唯一**事实源。原 CONCLUSIONS.md / backlog.json 已删；结论并入 SPEC/criteria，轮次索引在 ROUNDS.md，跨模块待办在 deps-todo.md。

## 当前

- **阶段**：Phase 2 逐模块 — 波次 0 进行中。
- **分支**：`main`（backend-new 平行重写不需要分支）。
- **策略**：`backend-new/` 平行重建 → 覆盖回 `backend/` → 调前端/testend 兼容。

## 已定的关键决策

- 全量重写，**无任何保留**（含本地 SQLite 数据 → schema 可激进重定）。
- **全局命名 `user_id` → `workspace_id`**（本地单机隔离单元=工作区；ctx/middleware/物理列/实体一律 workspace）。从波次 0 reqctx 起生效。
- 契约**可改**：每改对外 API/SSE/error 都 take note 到 `contract-changes.md`；前端/testend 也是 AI 写的，覆盖后一并兼容。
- 架构按 `module-template.md` 统一、**按需取层**；`go.mod` 空起按需生长、版本对齐现有。
- 重写单元 = 垂直切片；顺序见 `order.md`，判据见 `criteria.md`。
- **去 GORM**：自研 `pkg/orm`（链式、类型安全、自动 workspace 双向隔离 + 软删 + 时间戳）+ `glebarez/go-sqlite`（database/sql driver）。R0008 ✅。
- **domain 去 GORM 化（贯穿所有业务模块）**：domain 实体剥 `import gorm` + gorm tag + `TableName` + `gorm.DeletedAt` → 纯 struct + 轻量 `db:"col,..."` tag（无 import）；store 基于 `pkg/orm` 重写。
- **错误体系强化（贯穿所有模块）**：domain 错误升级为结构化 `Error{Kind,Code,Message,Details,cause}`（Is by Code）；错误码契约内聚 domain；各 domain `errors.New(msg)`→`New(kind,code,msg)`；transport errmap 塌缩成 `statusForKind`（M0.7，零 domain import）。R0012 ✅。
- **SSE 三流统一协议（流式树）**：`eventlog/forge/notifications` → `messages`/`entities`(全实体流式总线)/`notifications`；统一信封 `Envelope{seq,scope,id,frame}` + 四动词 frame(open/delta/close/signal) + **通用 Node{Type,Content}**（词表下放各业务、domain 不持）；**id 升信封层**；frame 按可丢性分级(delta/tick=ephemeral 不入 buffer，open/close/signal=durable，close 带快照)；infra **单一 `Bus`×3 实例**。设计蓝本 = `stream-protocol.md`（已拍板 2026-06-03）。

## 模块进度（编号见 order.md）

状态：⬜ pending ｜ 🔧 doing ｜ ✅ done ｜ ⏭️ 判定删除/合并

- **Phase 1 骨架** ✅：`backend-new/` + 空 go.mod + health server + smoke。
- **波次0 地基**：M0.1 pkg ✅（**reqctx/idgen/pagination ✅** R0001；**tokencount ✅** R0002；**pathguard ✅** R0003；**userpath ⏭️删** R0004；**wikilink ✅** R0005；**jsonrepair ✅** R0006；**limits ✅** R0007；modelcaps/modelcatalog 移交 M1.3）· M0.2 数据库层 ✅（**pkg/orm R0008 · db 网关 R0009**；业务表 DDL 分散各模块）· M0.3 ✅（**logger R0010 · crypto R0011**）· M0.4 ✅：**errors R0012** · **stream 统一协议 R0013**（单一 domain/stream：信封+四动词Frame+通用 Node{Type,Content}+Bridge/ListReader；词表下放业务）· M0.5 ✅ infra **stream bus（单一 Bus）R0014**（实例化三次=三流；frame 分级；D2 全量推；infra/chat extractor 移交 M5.2）· M0.6 llm ⬜ · M0.7 transport 框架 ⬜
- **波次1 叶子域**：M1.1 workspace(原 user) ⬜ · M1.2 apikey ⬜ · M1.3 model ⬜ · M1.4 relation ⬜ · M1.5 catalog ⬜ · M1.6 mention ⬜ · M1.7 memory ⬜ · M1.8 sandbox ⬜ · M1.9 permissions/hooks ⬜ · M1.10 document ⬜ · M1.11 todo ⬜(待判定)
- **波次2 tool+原语**：tool ⬜ · loop ⬜ · tool/filesystem·search·web·toolset ⬜
- **波次3 Quadrinity**：function·handler·subagent·agent·skill·mcp + tool 适配器组 ⬜
- **波次4 编排核心**：workflow ⬜ · flowrun ⬜ · scheduler 🔴⬜ · trigger ⬜ · tool/workflow ⬜
- **波次5 对话**：conversation ⬜ · chat ⬜ · contextmgr ⬜ · tool/permissionsgate ⬜
- **波次6 顶层编排**：askai ⬜ · ask+tool/ask ⬜(强残留嫌疑)
- **波次7 wiring**：cmd/server 装配 ⬜ · cmd/desktop+工具 ⬜

## 下一步

- **M0.6（下一轮）**：`infra/llm` 自有 provider 客户端（18 文件）+ factory（provider wire 格式冻结；`mock.go` 留测试）。`modelcaps`/`modelcatalog` 移交 M1.3。
