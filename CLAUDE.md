# Forgify — Claude 工作守则

> Claude Code 进入本项目自动加载本文件。**本文件是项目工程纪律的唯一事实源**。
> 项目愿景 / 架构 / 实体地图 / 引擎见 [`docs/concepts/architecture.md`](docs/concepts/architecture.md)；文档规范见 [`docs/GOVERNANCE.md`](docs/GOVERNANCE.md)。
> 旧版（覆盖回 `backend/` 之前的快照）在 `version-0.2` git 分支——参考旧版 checkout 它即可，不在当前文档维护任何历史。

---

## 项目一句话

- **本地优先 Agentic Workflow Platform**，目标 **Wails 桌面 app**、**单进程单用户**、SQLite 落盘（**不做 SaaS**）。
- **核心心智**：**Quadrinity（四项全能）** 实体（Function/Handler/Agent/Workflow）+ **Durable Execution**（节点结果记忆化 + 解释器幂等重走）。
- **架构**：4 层 Clean Architecture，依赖单向 `transport → app → (domain ∪ infra/store) → infra/db`。地基自研：`pkg/orm`（去 GORM）+ `glebarez/go-sqlite`（纯 Go、无 CGO）。
- **当前状态**：后端 clean-room 重写完成（`backend-new/`，全实体 + durable 引擎，编译/装配/启动全通）；**下一步**：覆盖回 `backend/` + 前端按 FSD 重建。

## 文档地图

> **V0.2 → V-next 重置**：docs 内容已清空、目录骨架按 `GOVERNANCE.md` 重建为空占位（每文件夹一个 `.gitkeep` 说明职责，待按新结构填充）。仅 `concepts/architecture.md` + `GOVERNANCE.md` + `INDEX.md` 留内容；前版完整文档在 `version-0.2` 分支。

| 用途 | 路径 |
|---|---|
| 愿景 / 架构 / 实体 / 引擎 / 路线 | `docs/concepts/architecture.md` |
| 文档规范（类型 / 同步 / 执行） | `docs/GOVERNANCE.md` |
| 文档索引（入口 + 结构） | `docs/INDEX.md` |

---

# 设计原则（9 条，#9 最高优先级）

1. **Quadrinity 实体化**：任何能力必须归属于 Function / Handler / Agent / Workflow 之一。
2. **Durable 为魂**：工作流执行基于**节点结果记忆化**（`flowrun_nodes` 行表 + record-once）+ **解释器幂等重走**实现崩溃恢复与确定性重放——**非**事件日志（Temporal 式 journal 已否决）。
3. **依赖自下而上**：`domain` 层**严禁 import 任何外部包**（含 ORM / cel-go）；`app` 层协调 domain 与 infra；跨实体协作走 DIP 端口、不硬依赖具体实现。
4. **后端契约是事实源**：`reference` 文档 = 代码的精确投影；前端按 FSD 架构对接已定型的后端契约（前端重建中）。
5. **端到端推演先行**：开工前必走完整数据流 + 列出跨域依赖（relation 边）。
6. **反校验剧场**：只保留有物理价值的校验（JSON、必填、CHECK/UNIQUE）；不加多余 null-check。
7. **零历史包袱**：项目未上线，禁止维护任何兼容性、禁止任何历史演化描述。修改一步到位、只留当前物理事实；历史从 git 取。
8. **复用优先、不造轮子**：动工前先盘点 `pkg/*` 与 `infra/*` 既有能力——能复用就复用；业务层将手搓的样板本应由地基提供时（如 orm 补 UNIQUE 冲突翻译），**强化地基**而非模块内重抄。错误抽象与重复样板比多写一行更糟。
9. **📌 文档与代码物理同步（最高优先级）**：每个代码改动必须在**同一提交**伴随对应文档的 1:1 更新——**文档落后于代码 = 严重 Bug，与编译失败同级**。完整执行规则见本文件末「**文档纪律（强制）**」节 + [`docs/GOVERNANCE.md`](docs/GOVERNANCE.md)。

---

# Standards — 契约宪法

## HTTP API（N 系列）

- **N1 统一 Envelope**：成功 `{"data": ...}`；失败 `{"error": {"code", "message", "details"}}`。
- **N2 状态码**：202 Accepted（异步流）/ 204 No Content / 410 Gone（SSE 淘汰）。
- **N3 命名规约**：API 线缆 camelCase；数据库物理列 snake_case。
- **N4 强制分页**：所有 List 接口必须支持 `?cursor=...&limit=...`。
- **N5 动作后缀**：非 CRUD 逻辑用 `:action`。
    - **`:run`**(fn) / **`:call`**(hd) / **`:invoke`**(ag) / **`:trigger`**(wf) 为标准执行动词。
    - **`:iterate`**（AI 编辑实体）/ **`:triage`**（AI 诊断执行）统一返回 `conversationId` 开启对话。

## 数据库（D 系列）

- **D1 软删除**：业务表用 `deleted_at DATETIME`；**Log 表**（`flowrun_nodes` / trigger 的 firing·activation / messages 块 等内容/执行日志）**严禁物理或逻辑删除**。
- **D2 物理隔离**：所有表（除全局配置外）必须持 **`workspace_id`** 物理列；`pkg/orm` 据 ctx 自动双向隔离。
- **D3 唯一性铁律**：`idx_frn_once`（flowrun 记忆化 `UNIQUE(flowrun_id,node_id,iteration)`）与 `idx_trf_dedup`（trigger firing 去重）必须保证幂等。

## SSE 协议（E 系列）

- **E1 三条流限制**：全系统仅 `messages` / `entities` / `notifications` 三条 SSE，**永不再加**。前端启动即常驻全连；三流 **workspace 级、后端不过滤**（发完整 delta、前端自滤）；订阅统一在 `StreamHandler`（`GET /api/v1/{messages,entities,notifications}/stream`）。
- **E2 Ephemeral 帧**：delta / tick（如 flowrun 节点推进）标 `seq=0`，**不入 buffer、不产生背压**；open/close/signal 为 durable（close 带快照供 replay）。
- **E3 嵌套递归**：messages 流支持 `parentBlockId` 嵌套，前端据此渲染 subagent 树。

---

# 代码规范（S 系列）

- **S5 物理文件对齐**：handler 文件名对应 API 资源域；domain 文件名对应 Repository 接口。
- **S9 确定性上下文**：每个跨层调用强制传 `ctx`；异步 Finalize 必须用 **Detached Context**（保留 workspace 种子、脱离请求取消）。
- **S11 注释双语化**：`// English \n\n // 中文`。**只写 Why、不写 What**。
- **S13 导入别名**：所有 `internal/` 包导入带 `<name><role>` 别名（如 `apikeydomain`、`chatapp`、`workflowstore`）。
- **S15 ID 宪法**：`<prefix>_<16hex>`。前缀全集必须在 `references/backend/database.md` 登记（infra 侧 ID 用自己的前缀，不从消费实体 ID 派生）。
- **S18 Tool 规范**：Tool 实现 **5 方法接口**（`Name`/`Description`/`Parameters`/`ValidateInput`/`Execute`）；`summary` / `danger`（三级 safe/cautious/dangerous，LLM 逐次自报）/ `execution_group` 三字段由 Framework 强制注入 schema 并从 args 剥离。**无中央权限门控**：危险靠 LLM 自报 + 逐次内存阻塞确认（active skill 的 `allowed-tools` 预授权可免确认）。
- **S20 错误构造**：会冒泡到 HTTP 的 domain 错误一律 `errorsdomain.New(kind, code, msg)`（带 Kind→HTTP status + 稳定 wire code）；**禁止**用标准库 `errors.New` 造命名错误。`errors.Is`/`errors.As` 仍用标准库。

---

# 测试与门禁（T 系列）

- **T5 Pipeline 优先**：大功能必须提供 `test/<axis>/` 下的集成测试（smoke / api / cross / sse / lifecycle / errcodes）。
- **T6 Fake LLM**：默认测试用 `fake_llm`，0 Token 消耗。
- **每次改动必过**：后端 `go build ./...`（5 平台）+ `go vet` + `gofmt` 净 + 相关包单测全绿。并发/取消相关测试带 `-race`。
- **文档门禁**：`lint-docs`（GOVERNANCE §11——frontmatter / reference-sync / ADR 不可变 / 孤儿链接 / INDEX≤50 行）；规格已定、待构建体系恢复后落地。在此之前以「文档纪律」收尾清单人肉把关。
- 前端门禁（TS / ESLint / FSD 架构检查）随前端重建接入。

---

# 前端开发守则（重建中，按本节）

- **FSD 架构**：严格遵守 `app → pages → widgets → features → entities → shared` 依赖链。
- **DIP 注入**：`shared` 层不准依赖上层；**workspace 注入** + 401 拦截由 `app` 层经 Provider 注入。
- **视觉灵魂**：明亮、通透、轻盈。`--row-h: 32px` 紧凑布局；`tool_call` 与 `reasoning` 默认折叠。
- **i18n**：严禁在 TSX 硬编码中英文；文案走 `t("key")`、登记在 `locales/` 下。
- 对接 backend-new 契约（三条 SSE 流常驻订阅；camelCase 线缆；统一 Envelope）。

---

# 文档纪律（强制 —— 完整规范见 [`docs/GOVERNANCE.md`](docs/GOVERNANCE.md)）

> 本节是文档规范的**常驻执行层**：CLAUDE.md 每次会话自动加载，故下列规则你**每次都已读到、无「不知道」借口**。详尽规则（6 类型 / frontmatter / 生命周期 / 命名 / 质量门禁）在 `GOVERNANCE.md`——它是 binding。**本节与 GOVERNANCE §0/§7/§12 必须一致**（改一处即同步另一处）。

## 三条铁律（违反 = 严重 Bug，与编译失败同级）

1. **同步**：改代码 → **同一提交**改对应文档。文档落后于代码 = 这次改动**未完成**。
2. **触发即停**：发现文档与代码不符 → 立刻停下修文档（记 `[doc-fix]` dev log），再续原任务。
3. **存疑即查**：不确定 → 查 `GOVERNANCE.md`；它没覆盖 → 按设计原则推导 + 回头补一条进 GOVERNANCE。

## 同步触发表（改左列代码 → 同一提交改右列文档）

| 代码改动 | 必须同步 |
|---|---|
| 新增/改 API 端点 | `references/backend/api.md` + 对应 `domains/<域>.md` |
| 新增/改 DB 表/列 | `references/backend/database.md` + 对应 `domains/<域>.md` |
| 新增/改 error code | `references/backend/error-codes.md` + 对应 `domains/<域>.md` |
| 新增/改 SSE 事件 | `references/backend/events.md` + 对应 `domains/<域>.md` |
| 架构决策（选型/取舍） | `decisions/` 新建一篇 ADR |
| 完成里程碑 | `concepts/architecture.md` 路线表 + changelog |
| 前端实体类型 / FSD 规则变更 | `references/frontend/{entity-types,fsd-layers}.md` |

非穷举——判据始终是「`reference` 文档必须 = 代码」。

## 收尾清单（声明任何代码改动「完成」前逐条勾，任一未过 = 未完成）

1. ☐ 碰了上表的东西？→ 对应文档**同提交**更新了？
2. ☐ 改的 `reference` 文档与代码**逐字**对得上（端点/字段/码/事件 一一吻合）？
3. ☐ 新文档 frontmatter 合法（`type`/`status`/`id`）、放对目录（GOVERNANCE §5）？
4. ☐ 删/移文档后无孤儿链接（`INDEX.md` 及他处指向它的都修了）？
5. ☐ 没编辑 `decisions/` 里的 ADR（不可变，只能新建 supersede）？
6. ☐ working 文档落地了（结论提取进 concepts/references + 填 `landed-into` + 移 `archive/`）？
