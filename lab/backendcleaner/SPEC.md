# backendcleaner — SPEC

本文定义 Forgify 后端 clean-room 清理工程的执行契约。

## 1. 目标

清理目标不是修补旧测试，而是重建后端质量体系：

- 先删除全部旧后端测试，解除旧实现约束。
- 再按主文件清理生产代码，每轮必须先扫描上下游依赖。
- 每个主文件或小职责切片清理后写对应单元测试。
- 每个模块完成后写模块集成测试。
- 所有模块完成后写全链路集成测试。

旧测试和旧实现都只是历史材料，不是权威。权威来自当前产品契约、清理后的模块职责和新写测试。

“干净实现”的定义：

- 读代码能直接看出业务意图。
- 一个函数只做一件主要事情。
- 分支和特殊情况少，且每个分支都有产品理由。
- 错误路径明确，不吞错，不用隐式 fallback 掩盖问题。
- 命名表达领域含义，不靠注释解释含混命名。
- 依赖方向清楚，跨层调用少。
- 没有为了旧测试、旧 harness、旧执行模型保留的绕路。
- 不用额外抽象掩盖复杂度；只有在能减少真实重复或明确边界时才抽象。

## 2. 阶段

### Phase 0：计划审查

位置：`main`

只允许：

- 维护 `lab/backendcleaner` 计划。
- 审核执行顺序和原则。

禁止：

- 删除后端测试。
- 修改后端生产代码。
- 开始实际 clean-room。

### Phase 1：新建清理分支

从最新 `main` 创建：

```bash
git switch -c codex/backend-cleanroom
```

所有破坏性清理都在该分支执行。每个阶段或轮次必须提交。

### Phase 2：删除全部后端旧测试

删除范围：

- `backend/**/*_test.go`
- `backend/test/**`
- 后端旧 test harness、fake server、pipeline/e2e 测试资料

规则：

- 不在仓库 active tree 内归档旧测试。
- 需要参考旧测试时，从 git 历史查看。
- 删除完成后先提交一次，作为 clean-room 起点。

### Phase 3：主文件 clean-room + 依赖扫描 + 单元测试

默认粒度：一个主文件一轮。这里的“文件”是提交和主改动粒度，不是分析边界。

每轮必须先做依赖扫描：

- 谁调用主文件中的公开类型、函数、方法。
- 主文件调用了哪些 domain / app / repo / pkg / infra。
- 同包其他文件是否共享状态、helper、接口、错误、常量。
- 是否存在循环依赖、隐性耦合、跨层调用。
- 是否存在“为了旧测试或旧路径”留下的兼容分支。

如果问题跨多个文件互相调用，允许升级为“小职责切片”。升级条件：

- 不一起改会把脏逻辑推到邻居文件。
- 主文件的职责边界必须通过相邻文件调整才能成立。
- 需要移动 helper / interface / error / type 才能消除反向依赖。

升级要求：

- 在本轮记录中写明为什么不能保持单文件粒度。
- 明确本轮切片边界。
- 不得借此同时清理多个无关职责。

单轮流程：

1. 选择一个文件。
2. 扫描上下游依赖。
3. 写或更新模块契约。
4. 阅读旧实现，记录历史包袱和跨文件耦合。
5. 清理或重写该文件逻辑；必要时升级为小职责切片。
6. 为该文件或职责切片写单元测试。
7. 跑目标包和直接依赖包验证。
8. 记录本轮。
9. 提交。

每轮必须自证“更干净”：

- 分支是否减少，或每个保留分支是否有当前产品理由。
- fallback / alias / legacy / best-effort 是否减少。
- 职责是否更直接。
- 调用关系是否更清楚。
- 是否新增了不必要抽象；如果新增抽象，必须说明减少了什么真实复杂度。
- 如果主文件清不干净，必须记录前置依赖，或升级为小职责切片，不得局部粉饰。

建议顺序：

1. `backend/internal/domain/*`
2. `backend/internal/pkg/*`
3. `backend/internal/infra/store/*`
4. 叶子 app service：apikey、model、memory、document、agent、skill、mcp
5. tool adapters
6. HTTP handlers / response / router / SSE
7. workflow / scheduler / trigger
8. chat / loop / agent orchestration
9. server wiring / boot / dev-only endpoints

核心模块额外规则：

- workflow / scheduler / chat / loop / server wiring 在清理具体文件前，必须先写模块级契约。
- 模块级契约必须说明新模型是什么、旧模型哪些路径要删除、哪些路径暂时保留在边界。
- 如果模块契约无法写清楚，停止执行，不进入代码清理。

长期 smoke：

- 从 Phase 3 开始，清理分支应保留最小启动 smoke。
- smoke 只验证系统没有跑偏：后端可启动、`/api/v1/health` 可过、用户初始化路径可过。
- smoke 不是全链路 e2e，不承担覆盖职责。

### Phase 4：模块集成测试

一个模块所有文件完成 clean-room 和单元测试后，再写模块集成测试。

模块集成测试覆盖：

- store + service 协作
- HTTP contract
- 数据库 schema / 事务 / 约束
- SSE / eventlog / notification
- sandbox / permissions / LLM provider 边界

### Phase 5：全链路集成测试

所有模块完成后，按当前产品旅程写全链路集成测试。

首批旅程：

- 启动和用户初始化
- 配置 API key / model
- 基础对话
- function / handler / workflow 创建和执行
- agent 节点执行
- document attach
- memory 注入
- MCP server 注册和调用
- SSE / notification / eventlog
- permissions / sandbox

### Phase 6：覆盖闭环

最后不以 coverage 数字作为唯一标准，而以三张清单闭环作为完成定义。

#### 6.1 文件清单闭环

每个后端生产文件必须有状态：

- `cleaned`：已 clean-room。
- `deleted`：已删除。
- `generated`：生成代码或资源，不需要手工清理。
- `boundary-kept`：作为明确边界保留，例如 provider wire compatibility、migration boundary。

每个 `cleaned` 文件必须对应：

- dependency scan 记录。
- clean-room 轮次记录。
- 单元测试或“不适合单元测试”的明确理由。

#### 6.2 模块清单闭环

每个模块必须有状态：

- 模块契约已写。
- 模块内文件状态全部闭环。
- 模块集成测试已写，或明确说明该模块只需要 unit。
- 直接依赖模块验证通过。

#### 6.3 产品旅程闭环

全链路测试必须覆盖当前产品首批旅程：

- 启动和用户初始化。
- 配置 API key / model。
- 基础对话。
- function / handler / workflow 创建和执行。
- agent 节点执行。
- document attach。
- memory 注入。
- MCP server 注册和调用。
- SSE / notification / eventlog。
- permissions / sandbox。

#### 6.4 数字覆盖只做辅助

可以跑 Go coverage，但只用于发现明显盲区。coverage 数字不能替代文件清单、模块清单和产品旅程清单。

## 3. 验收闸门

单轮最低要求：

- `gofmt`
- `go test <target package>`
- 无反向依赖
- 新测试描述当前产品行为，不描述旧实现细节
- 清理结果必须更简单清楚；禁止用更多抽象掩盖复杂度
- 本轮记录完整

阶段要求：

- Phase 2 后，后端至少能完成编译扫描。
- Phase 3 中，每个主文件或职责切片清理后有新单元测试。
- Phase 3 中，每轮记录必须包含 dependency scan 摘要。
- Phase 4 中，每个模块完成后有模块集成测试。
- Phase 5 后，跑全链路集成测试和全量 backend verification。
- Phase 6 后，文件清单、模块清单、产品旅程清单全部闭环。

## 4. 禁止事项

- 禁止为了旧测试保留旧逻辑。
- 禁止恢复旧测试体系。
- 禁止让旧 harness 成为新测试基础设施。
- 禁止用 coverage 数字替代产品旅程验证。
- 禁止一次清理多个高耦合核心模块。
- 禁止只看主文件、不看调用方和被调用方。
- 禁止把 dry-run/mock 当真实执行行为。
- 禁止把 legacy migration 无限期留在核心路径。
- 禁止把“干净”误解为增加 interface、helper、manager、adapter；抽象必须证明能减少真实复杂度。
- 禁止用“go test 通过”替代覆盖闭环。
