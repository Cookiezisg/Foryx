# backendcleaner

后端 clean-room 全量重写计划。把 Forgify 后端从历史迭代累积态，在 **`backend-new/` 平行重建**成当前产品契约驱动的干净实现 + 干净测试，全部完成后覆盖回 `backend/`，再调前端 / testend 兼容。

"干净" = 实现清晰、逻辑简单、职责直接、分支少、命名准确、错误路径明确、**无任何历史包袱**。不是增加抽象层，不是为显得架构化而拆文件，不是把旧复杂度换种形式藏起来。

## 核心决策

1. **backend-new 平行重写**，不在原 backend 上修补；现有 backend 全程可跑、仅作只读考古。
2. **不开分支**：backend-new 是新增目录，重写期全程在 `main` commit+push；只有最后"覆盖"是破坏性操作。
3. **不删测试步骤**：旧测试随旧 backend 在覆盖时消失；backend-new 从零按新标准写测试。
4. **判据 = 目标架构符合性**：不在白名单的概念/能力即死代码，无论是否被注册调用（见 `criteria.md`）。
5. **垂直切片**逐模块重写，严格按依赖拓扑（基础→复杂，见 `order.md`）。
6. 契约可改，每改 take note；完成定义 = 文件/模块/产品旅程三清单闭环，coverage 只做辅助。

## 目录

| 文件 | 作用 |
|---|---|
| `SPEC.md` | 执行阶段、验收闸门、禁止事项 |
| `PLAYBOOK.md` | AI 每轮（每模块）四步循环手册 |
| `target/criteria.md` | 目标架构判据白名单 = 删除/保留标准 |
| `target/order.md` | 依赖拓扑 + 7 波次重写顺序（不重不漏） |
| `target/module-template.md` | canonical clean-arch 模块骨架（按需取层） |
| `target/capability-ledger.md` | 能力对账全集（覆盖前逐项闭环） |
| `target/contract-changes.md` | 契约变更日志（驱动覆盖后前端/testend 兼容） |
| `target/STATE.md` | **单一状态源**：阶段 + 模块进度 + 下一步 |
| `target/ROUNDS.md` | 已执行轮次索引 |
| `target/contracts/<m>.md` | 每模块契约（逐模块写） |
| `target/rounds/NNNN/` | 每轮执行记录 |

## 当前状态

**Phase 0 计划** — lab 已定稿。等确认后进 Phase 1（建 backend-new 骨架 + 波次0 地基）。在此之前不写 backend-new 代码、不动现有 backend。
