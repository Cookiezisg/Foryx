# Forgify — Claude 工作守则

> Claude Code 进入本项目自动加载本文件。**本文件是项目工程纪律的唯一事实源**。
> 项目愿景 / 架构 / Phase 路线 / Verification 见 [`docs/concepts/architecture.md`](docs/concepts/architecture.md)。

---

## 项目一句话

- **本地优先 Agentic Workflow Platform**，目标 **Wails 桌面 app**（不做 SaaS）
- **单人项目**；后端 Phase 0-5 已完成（地基/对话/锻造/持久化执行/智能化）；**当前重心：前端联调（V1.2 桌面 app）**
- **核心心智**：**Quadrinity (四项全能)** 实体 + **Durable Execution (持久化执行)** 引擎
- **架构**：4 层 Clean Architecture，依赖方向 `transport → app → (domain ∪ infra/store) → infra/db`

## 文档地图

| 用途 | 路径 |
|---|---|
| 项目愿景 / 架构 / 路线图 | `docs/concepts/architecture.md` |
| 开发日志 / 决策快照 | `docs/references/changelog.md` |
| 契约: 全量 REST API | `docs/references/backend/api.md` |
| 契约: 全量 DB Schema | `docs/references/backend/database.md` |
| 契约: SSE 实时协议 | `docs/references/backend/events.md` |
| 契约: 181 错误码对账 | `docs/references/backend/error-codes.md` |
| 领域: 29 个 Domain 详设计 | `docs/references/backend/domains/<domain>.md` |
| 前端: FSD 层级与 Slice 设计 | `docs/references/frontend/` |
| 架构决策记录（ADR） | `docs/decisions/README.md` |

---

# 设计原则（9 条，#9 最高优先级）

1. **Quadrinity 实体化**：任何能力必须归属于 Function, Handler, Agent (智能体), Workflow (工作流) 之一。
2. **Durable 为魂**：Workflow 必须基于 Journal 实现崩溃恢复与确定性重放。
3. **依赖自下而上**：Domain 层严禁依赖任何外部包；Service 层协调 Domain 与 Infra。
4. **前后端阶段解耦**：后端接口已定型进入“法律效力”阶段，前端严格按 FSD 架构进行 Page/Feature 交付。
5. **端到端推演先行**：开工前必走完整数据流 + 列出跨域依赖（Relation 边）。
6. **反校验剧场**：只保留有物理价值的校验（JSON、必填、CHECK/UNIQUE）；不加多余的 null-check。
7. **零历史包袱**：项目未上线，禁止维护任何兼容性、禁止带有任何历史演化描述。修改直接一步到位，只保留当前物理事实。
8. **复用优先、不造轮子**：每个模块动工前先盘点 `pkg/*` 与 `infra/*` 既有能力——能复用就复用；若业务层将要手搓的样板本应由地基提供（如 orm 补 UNIQUE 冲突翻译），**强化地基**而非在模块内重抄。错误抽象与重复样板比多写一行更糟。
9. **📌 文档与代码物理同步**：每个代码改动必须伴随契约文档的 1:1 更新。**文档落后于代码 = 严重 Bug**。


---

# Standards — 契约宪法

## HTTP API（N 系列）

- **N1 统一 Envelope**：成功 `{"data": ...}`；失败 `{"error": {"code", "message", "details"}}`。
- **N2 状态码**：202 Accepted (异步流) / 204 No Content / 410 Gone (SSE 淘汰)。
- **N3 命名规约**：API 线缆使用 camelCase；数据库物理列使用 snake_case。
- **N4 强制分页**：所有 List 接口必须支持 `?cursor=...&limit=...`。
- **N5 动作后缀**：非 CRUD 逻辑使用 `:action`。
    - **`:run`** (fn) / **`:call`** (hd) / **`:invoke`** (ag) / **`:trigger`** (wf) 为标准执行动词。
    - **`:iterate`** (AI 编辑) / **`:triage`** (AI 诊断) 统一返回 `conversationId` 开启对话。

## 数据库（D 系列）

- **D1 软删除**：业务表使用 `deleted_at DATETIME`；Journal 与 Log 表严禁物理或逻辑删除。
- **D2 物理隔离**：所有表（除全局配置外）必须持有 `user_id` 物理列。
- **D3 唯一性铁律**：`idx_fre_record_once` (Journal) 与 `idx_trf_dedup` (Trigger) 必须确保幂等性。

## SSE 协议（E 系列）

- **E1 三条流限制**：全系统仅允许 `messages`, `notifications`, `entities` 三条 SSE，永不再加。前端启动即常驻全连；三流 **workspace 级、后端不过滤**（始终发完整 delta，前端按对话/实体自滤）；订阅端点统一在 `StreamHandler`（`GET /api/v1/{messages,entities,notifications}/stream`）。
- **E2 Ephemeral Ticks**：`flowrun:tick` 事件必须标记为 `seq=0`，不入 Buffer，不产生背压。
- **E3 Eventlog 递归**：支持 `parentBlockId` 嵌套，前端以此渲染 Subagent 树。

---

# 代码规范（S 系列）

- **S5 物理文件对齐**：Handler 文件名必须与 API 资源域对应；Domain 文件名必须与 Repository 接口对应。
- **S9 确定性上下文**：每个跨层调用强制传 `ctx`；异步 Finalize 必须使用 **Detached Context**。
- **S11 注释双语化**：`// English \n\n // 中文`。只写 Why，不写 What。
- **S13 导入别名**：所有 `internal/` 包导入必须带 `<name><role>` 别名（如 `apikeydomain`, `chatapp`）。
- **S15 ID 宪法**：`<prefix>_<16hex>`。前缀全集（33 种）必须在 `database.md` 中登记。
- **S18 Tool 规范**：Tool 必须实现 5 方法接口（`Name`/`Description`/`Parameters`/`ValidateInput`/`Execute`）；`summary`/`danger`（三级 safe/cautious/dangerous，LLM 逐次自报）/`execution_group` 三字段由 Framework 强制注入 schema 并从 args 剥离。无中央权限门控（M1.9 解散）：危险靠 LLM 自报 + 逐次确认。
- **S20 错误构造**：创建会冒泡到 HTTP 的 domain 错误一律 `errorsdomain.New(kind, code, msg)`（带 Kind→HTTP status + 稳定 wire code），**禁止**用标准库 `errors.New` 造命名错误；`errors.Is`/`errors.As`（匹配/解包）仍用标准库。

---

# 测试与门禁（T 系列）

- **T5 Pipeline 优先**：大功能必须提供 `backend/test/<axis>/` 下的集成测试。
- **T6 Fake LLM**：默认测试环境必须使用 `fake_llm` 实现 0 Token 消耗测试。
- **门禁命令**：
    - `make unit`: 后端单测。
    - `make mock`: 离线 Pipeline 验证（16 包全绿）。
    - `make verify`: 5 平台编译 + 静态检查 + 覆盖率矩阵审计。
    - `make lint`: 前端 TS + ESLint + Steiger 架构检查。

---

# 前端开发守则

- **FSD 架构**：严格遵守 `app -> pages -> widgets -> features -> entities -> shared` 依赖链。
- **DIP 注入**：`shared` 层不准依赖上层；UserID 注入与 401 拦截由 `app` 层通过 Provider 注入。
- **视觉灵魂**：明亮、通透、轻盈。`--row-h: 32px` 紧凑布局。`tool_call` 与 `reasoning` 默认折叠。
- **i18n**：严禁在 TSX 中硬编码中英文；所有文案必须走 `t("key")` 并登记在 `locales/` 下。

---

**发现文档与代码不符 -> 立刻停下修文档，记 `[doc-fix]` dev log。**
