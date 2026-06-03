# backendcleaner — PLAYBOOK

> 每轮（= 每个模块）开工前必读：本文 + `SPEC.md` + `target/criteria.md` + `target/order.md` + `target/module-template.md` + `target/STATE.md` + 目标模块的 `target/contracts/<m>.md`。
> 上下文断了，读完这几份就能一致地接着干。

## 1. 权威顺序

1. 当前产品意图（`docs/concepts/architecture.md` 核心能力）
2. `criteria.md` 判据（什么该存在 / 什么是范式残留）
3. 模块契约 `contracts/<m>.md`
4. 重写后的 `backend-new` 代码
5. 新写测试
6. 现有 `backend` 代码与测试 —— **只读考古**：提取产品逻辑 + 血泪边界，绝不 copy 结构、绝不被它驱动实现。

## 2. 每模块四步循环（核心）

### 步骤 1 — 完整梳理逻辑，写 `contracts/<m>.md`，讲给人听

- **考古**：现有 backend 这块在做什么——产品职责、输入/输出、副作用、错误路径。
- **去包袱**：列出删哪些历史包袱、合并哪些"一半新一半旧"的实现 → 给出**修改后的完整、清晰逻辑**（给人看的，不是代码堆砌）。
- **守边界**：哪些边界是旧测试/血泪换来的（如 durable engine 的 replay 确定性、record-once dedup、approval first-wins）→ 写进契约的"必须保证的行为"，作为新测试的规格。
- **契约变更**：若要改对外 API/SSE/error code → 记入 `contract-changes.md`（模块 / 原→新 / 为什么 / 前端&testend 受影响点）。
- **核心模块（workflow / scheduler / chat / loop / wiring）契约写不清，就停，不动代码。**
- 这一步的产出**给用户审阅**后再进步骤 2。

### 步骤 2 — 理解后重构（不是 grep），在 backend-new 写干净实现

- 按 `module-template.md` 的分层、命名、注释规范。
- 整模块删掉重写；不把旧复杂度搬进新 helper。

### 步骤 3 — 补测试（新标准）

- **unit**：业务规则 / 错误边界 / 状态转换。不测私有细节，不依赖旧 harness。
- **模块集成**：store+service 协作 / HTTP contract / schema·事务·约束 / SSE·eventlog·notification / sandbox·permissions·provider 边界。
- **e2e（全链路）**：跨模块，留到一条产品旅程上的模块都就绪后统一写（Phase 5）；不在单模块阶段强写。

### 步骤 4 — 落文档（两处）

- **后端 reference**：`docs/references/backend/` 的 `api.md` / `database.md` / `events.md` / `error-codes.md` / `domains/<m>.md` 同步（CLAUDE.md #8 物理同步，文档落后 = bug）。
- **lab**：`rounds/NNNN/round.md` 本轮记录 + 更新 `STATE.md` + `capability-ledger.md` 勾稽 +（若契约变）`contract-changes.md`。

## 3. 开工先做依赖扫描（backend-new 语境）

- **上游**：它依赖的模块在 backend-new 是否已就绪（按 `order.md` 应已就绪；没就绪说明顺序排错了，回 order 修）。
- **下游**（还没写的）：用 domain 接口声明 + 接口注入（DIP），不 import 未来的具体类型。
- **考古**：现有 backend 同模块的同包共享状态/helper/错误/常量、跨层耦合、为旧路径留的兼容分支——记进 round 笔记，新实现据此规避。
- **下游需调整**：若发现某下游模块设计有问题、得等那一轮改，在该下游 `contracts/<下游>.md` 顶部"待调整"区登记，**不当场跨界改**。

## 4. 清理判断

优先删：只服务旧测试的逻辑 / 注释中已声明未来删的旧路径 / tolerated alias / 重复 adapter·resolver·registry / 混在核心路径的 dev·mock / 巨型文件里的无关职责。
优先留（boundary-kept）：当前产品真实需要 / 外部 API 明确契约 / 权限·安全·sandbox / LLM provider wire / generated 资源。
清不干净时：不局部粉饰、不把旧复杂度搬进新 helper；记录前置依赖或升级为小职责切片，边界小而明确。

## 5. round 记录模板（`target/rounds/NNNN/round.md`）

```md
# Round NNNN — <模块>（波次 Mx.y）

类型 / 目标:
依赖扫描: 上游就绪 / 下游接口 / 考古发现
旧实现历史包袱:
修改后完整逻辑（= contracts/<m>.md 摘要）:
删除 / 合并:
契约变更（→ contract-changes.md）:
新实现要点:
新测试: unit / 模块集成
验证: gofmt / go build / go test / smoke
是否更干净（自证：分支减少？fallback/alias 减少？职责更直接？无多余抽象？）:
覆盖状态（capability-ledger 勾了哪些能力）:
遗留 / 下一步:
```

## 6. 验证纪律

每轮至少：`gofmt` + `backend-new` `go build` + 目标包 `go test`。改共享接口跑直接依赖包；改 HTTP 跑相关 handler；核心模块（workflow/scheduler/chat）模块完成时必须补集成测试，不能只靠 unit。每波次结束跑最小 smoke。

## 7. 停止条件（停下记录，不硬改）

- 当前产品行为不清楚。
- 权限 / sandbox / security 语义不确定。
- 需要跨多个核心模块才能完成。
- 清理后只能靠新增抽象才能解释（说明设计还没想清楚）。
- 核心模块契约写不清。
