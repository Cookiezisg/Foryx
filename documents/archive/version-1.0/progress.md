# Forgify 开发进度

**更新于**：2026-04-22  
**当前状态**：Tier 1-4 全部完成，**V1.1 全部完成**（Tab 系统 + 分屏 + 工具增强 + 版本历史 + 分屏按钮隐藏 + AI 修复闭环）

> **V1.1 迭代**：前端 Tab 架构重构 + Chat+Tool 分屏 + 工具系统增强。
> 详见 `Documents/V1.1/PRD_1.1.md` 和 `Documents/V1.1/TDD_1.1.md`。
> V1.1 完成后再继续 Tier 5（工作流画布）。

---

## 状态标记说明

```
⬜ 未开始
🔄 进行中
✅ 已完成
⏸ 暂缓（有依赖未完成）
```

---

## Tier 1 — 地基（必须最先做）

这三个切片是整个项目的骨架，后续所有切片都依赖它们。

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **A1** App Shell | Electron 项目初始化，侧边栏四 Tab，SplitView | ✅ | |
| **A2** Data Layer | SQLite 连接，001-012 迁移自动执行 | ✅ | |
| **A3** Event System | EventBridge，events 常量定义 | ✅ | |

---

## Tier 2 — AI 核心（对话能跑起来）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **B1** API Key Management | API Key 存储，UI 管理 | ✅ | |
| **K1** Model Settings | 模型配置，连接测试 | ✅ | 和 B1 一起做，先能选模型 |
| **B2** Streaming Core | Eino ConversationAgent，流式 token | ✅ | 核心 AI 调用 |
| **B3** Model Strategy | 多模型切换策略 | ✅ | 可以最后再做 |

---

## Tier 3 — 对话界面

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **C1** Conversation Management | 对话列表，1:1 资产绑定 | ✅ | 搜索、重命名、归档、删除、自动命名、资产徽章 |
| **C2** Message UI | 消息气泡，Markdown 渲染，流式 | ✅ | 滚动控制、模型名称、空状态、卡片框架 |
| **C3** File Attachment | 文件拖拽上传 | ✅ | 拖拽/点击上传、Excel/PDF/Word/图片解析、Eino 多模态注入 |
| **C4** Context Compression | 三层压缩 | ✅ | MicroCompact/AutoCompact/FullCompact、压缩提示条 |

---

## Tier 4 — 工具基础（工具能创建和运行）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **D3** Python Sandbox | uv venv，subprocess 执行 | ✅ | 沙箱执行器+安装器+runner模板 |
| **D6** Built-in Tools | 内置工具注册，go:embed | ✅ | 4个代表工具(file/web/data/system)，启动自动注册 |
| **D1** Tool Library | 工具列表，搜索 | ✅ | 工具库UI+搜索+分类筛选+卡片 |
| **D4** Tool Detail | Monaco 代码查看/编辑 | ✅ | 代码/参数/测试三Tab，Monaco编辑器 |
| **D2** Tool Forge | AI 对话创建工具 | ✅ | 代码块检测+自动解析保存草稿工具 |
| **D5** Tool Sharing | 导入导出 | ✅ | .forgify-tool JSON导出导入+冲突处理 |

---

## Tier 5 — 工作流画布（节点能渲染和编辑）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **E1** Workflow Canvas | ReactFlow，BaseNode，DB 迁移 | ⬜ | 先渲染一个空画布 |
| **E2** Workflow Creation | AI 输出 flow-definition，状态注入 | ⬜ | 需要 B2 + E1 |
| **E3** Basic Nodes | Trigger/Tool/Condition/Variable/Approval | ⬜ | 需要 E1 |
| **E4** Advanced Nodes | Loop/Parallel/Subworkflow | ⬜ | 需要 E3 |
| **E5** AI Nodes | LLM/Agent | ⬜ | 需要 E3 |

---

## Tier 6 — 执行引擎（工作流能跑起来）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **F1** Flow Compiler | FlowDefinition → Eino Graph | ⬜ | 核心，其他 F* 依赖它 |
| **F2** Manual Run | 执行引擎，节点状态推送 | ⬜ | 需要 F1 |
| **F4** Error Handling | 重试，错误展示 | ⬜ | 需要 F2 |
| **F5** Run History | 历史记录，历史回放 | ⬜ | 需要 F2 |
| **F3** Mailbox Approval | Approval 节点阻塞等待 | ⬜ | 需要 F2 |

---

## Tier 7 — 自动化触发

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **G2** Deploy Config | 状态机，部署前检查 | ⬜ | 需要 F1 |
| **G1** Cron Scheduler | robfig/cron，定时触发 | ⬜ | 需要 G2 |
| **G3** Event Triggers | 文件监听 + Webhook | ⬜ | 需要 G2 |
| **G4** Error Fix Conversation | 失败后 AI 诊断 | ⬜ | 需要 F4 + B2 |

---

## Tier 8 — 权限 & Inbox

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **H2** Mailbox Queue | 统一消息队列 DB | ⬜ | 其他 Inbox 依赖它 |
| **H1** Permission Gate | 工具权限门控 | ⬜ | 需要 H2 |
| **I1** Inbox Core | 消息列表，未读徽章 | ⬜ | 需要 H2 |
| **I2** Approval Workflow | 审批操作 UI | ⬜ | 需要 I1 + F3 |
| **I3** Inbox Context View | 只读 Canvas + 代码 | ⬜ | 需要 I1 + E1 |

---

## Tier 9 — 收尾

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **J1** Home Page | 状态摘要 + 最近活动 | ⬜ | |
| **K2** General Settings | 超时配置，数据导出，权限管理 | ⬜ | |

---

## 依赖关系速查

```
A1 ← 所有切片
A2 ← 所有切片（数据库）
A3 ← 所有有事件推送的切片

B1, K1 → B2 → C1, C2, D2, E2
D3 → D6, D2
E1 → E2, E3, E4, E5
F1 → F2 → F3, F4, F5
F1 → G2 → G1, G3
H2 → H1, I1, I2, I3
```

---

## 各 Tier 完成后能跑的东西

| Tier 完成后 | 可以体验的功能 |
|---|---|
| Tier 1 | 空应用跑起来，四个 Tab 可以切换 |
| Tier 2 | 和 AI 对话，流式输出 |
| Tier 3 | 完整对话体验，对话列表 |
| Tier 4 | 创建工具，运行工具，内置工具可用 |
| Tier 5 | 通过对话创建工作流，画布渲染节点 |
| Tier 6 | 工作流可以手动运行，节点状态实时显示 |
| Tier 7 | 工作流自动运行（定时/文件/Webhook）|
| Tier 8 | 权限确认，Inbox 消息，审批操作 |
| Tier 9 | Home 页、设置页，全功能完整 |

---

## 开发日志

| 日期 | 内容 |
|---|---|
| 2026-04-19 | PRD v0.3 + TDD v0.3 + 38 个切片文档全部完成 |
| 2026-04-19 | A1+A2+A3 完成：Electron + Go HTTP subprocess，SSE 事件系统，SQLite 迁移 |
| 2026-04-20 | B1+K1+B2+B3 完成：API Key 加密存储，模型设置，Eino 流式对话，模型降级策略 |
| 2026-04-20 | C1+C2 完成：ConversationService，搜索/重命名/归档/自动命名，ChatContext，滚动控制，模型名称，空状态 |
| 2026-04-20 | C3+C4 完成：文件拖拽/点击上传，Excel/PDF/Word/图片解析注入，三层上下文压缩引擎，CompactBanner |
| 2026-04-20 | Tier 4 全部完成：Python沙箱(uv+venv+subprocess)，工具CRUD+测试历史，forge解析器(AST提取函数/参数/依赖)，4个内置工具(go:embed)，Monaco代码编辑器，工具导入导出 |
| 2026-04-20 | Tier 4 补全：forge 锻造链路（ForgeCodeBlock+TestParamsModal+SaveToolModal），自动绑定，forgeToolId持久化 |
| 2026-04-20 | V1.1 PRD+TDD 完成：Tab 架构重构 + Chat+Tool 分屏 + 工具系统增强（inline编辑/标签/版本/AI修复） |
| 2026-04-20 | V1.1 Phase 1+2+3 实现：TabContext/TabBar/NavSidebar/LayoutRouter/App重构，ChatToolLayout分屏，后端标签/版本/测试用例/元数据API，AI工具列表注入 |
| 2026-04-21 | V1.1 Phase 4 完善：Tab 拖拽排序+右键菜单(关闭/关闭其他/关闭右侧/全部关闭/固定)，TabBar空白区+6px顶栏可拖窗口 |
| 2026-04-21 | 工具元数据双向同步：NormalizeCodeAnnotations 确保代码 `# @` 注释与 DB 一致，UI 改名/描述/分类→代码同步更新，@custom/@builtin 标注，system prompt 加 @version |
| 2026-04-21 | 工具编辑器增强：InlineSelect 分类下拉编辑，版本 badge 可点击进入历史模式，VersionHistoryView（版本列表+Monaco DiffEditor side-by-side），恢复任意历史版本 |
| 2026-04-21 | ChatToolLayout 双向面板折叠：左侧聊天可折叠（渐变遮罩浮动按钮），右侧工具可折叠（已有），互斥约束（不可同时折叠） |
| 2026-04-22 | 代码审计 + 优化计划（OPTIMIZATION_PLAN.md）：15 项任务分 5 个 Stage |
| 2026-04-22 | Stage 1 #1：Chat+Tool / Chat+Workflow 分屏下隐藏对话气泡内的测试/保存按钮（TabContext 暴露 useCurrentLayout hook，ForgeCodeBlock 早返 null；代码块文本由父组件 SyntaxHighlighter 保留） |
| 2026-04-22 | Stage 1 #2：测试失败"让 AI 修复"一键链路（Pub-Sub 模式：TestTab 广播 forge:fix-requested 事件 → ChatContent 按 conversationId 匹配后 sendMessage 进 Eino 对话管道 → AI 基于完整对话历史生成新代码 → 现有 ForgeMiddleware 自动推送右侧代码面板刷新）。后端零改动。V1.1 验收清单 100% 达成 |
