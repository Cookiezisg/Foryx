# Anselm demo — 能力清单（Capability Manifest）

> **覆盖闸**：demo 要“完整展示产品所有能力”，靠这张清单变成**可勾选**、而非口号。
> 每行 = 一个**用户面能力** → 它在 demo 的归属面 → 后端契约出处 → 状态。
> 投影自后端契约事实源：[`api.md`](../docs/references/backend/api.md) · [`database.md`](../docs/references/backend/database.md) · [`events.md`](../docs/references/backend/events.md)。
>
> **UI 范式归宿**见 [`PATTERNS.md`](PATTERNS.md)（原语/Pattern 覆盖登记——每个范式标 covered/compose/pattern/escape-hatch + 归宿，杜绝造轮子）。本清单管"做哪些面"，PATTERNS 管"用什么件拼"。
>
> **范围**（已定）：以**用户心智面**为主——默认隐藏 raw ID / endpoint / SSE scope / 执行行等内部细节（藏进 developer mode，排后）。
> 遇到“后端小改即可满足”的，标 🔧 并记一行给后端（真 Flutter 前后端一起开工，demo 可反向提需求）。

**状态**：✅ 已落 · 🔨 进行中 · ▢ 规划 · 🔧 需后端小改 · 🅓 developer mode（排后）

---

## 0. 地基（跨所有海洋）

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 设计令牌 / 原语层（Web Components） | `core/tokens.css` · `core/primitives/` · `reference.html` | SPEC 数系 | ✅ |
| L1 机械门禁 | `tools/lint.mjs` | — | ✅ |
| 三岛外壳（左岛侧栏 / 海洋 / 右岛） | `core/shell.js` `core/sidebar.js` | — | ✅ |
| 装配根 + feature 懒加载 loader | `core/app.js`（manifest→features/<id>/{rail,sea}.js 懒加载 + 优雅占位） | — | ✅ |
| 选中 / 实时契约 | `core/{manifest,intent,live}.js` | 3 条 SSE 流（E1） | 🔨 契约就位、随海洋接 |
| 状态翻译单源 / 实体类型单源 | `core/schema/{state-model,entity-kinds}.js` | events `node.type` · 9 kind | ✅ |
| 三流实时（messages/entities/notifications） | `core/live.js`（mock→真 SseGateway 同契约） | events E1/E2/E3 | 🔨 |

---

## 1. Entities 海洋（Quadrinity + 图节点 + 挂载实体）

> 默认呈现**用户能力面**（名称/说明/标签/当前版本状态/可执行能力），不裸露 id/endpoint/执行行。

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 实体侧栏（4 大组→kind→实体行 + New/过滤/排序 + 行尾 …） | `features/entities/rail`（an-sidebar-list；9 kind 归四项全能/图部件/连接/技能 4 可折叠大组） | `GET /{functions,handlers,agents,workflows,...}` | ✅ mock |
| 改名 / 改说明（就地编辑 metadata） | 页头标题 hover 铅笔 → input + 保存/取消（an-title-change）；说明字段 hover 铅笔就地改（an-field-change） | `PATCH /{kind}/{id}`（name/description） | ✅ mock |
| **Function**：代码 / 输入输出字段 / 依赖 / env 状态 / 运行历史 | entities sea（schema 渲染）+ 右岛试运行终端 | `function` 域；`:run` | ✅ |
| **Handler**：methods / init 配置完整度 / 常驻状态 / 调用 | 同上 | `handler` 域；`:call` `/config` | ✅ |
| **Agent**：prompt / skill / knowledge / tools 挂载健康 / 运行 | 同上 | `agent` 域；`:invoke` `/mount-health` | ✅ |
| **Workflow**：图 / 生命周期 / 并发 / 关注 / 运行历史 | 实体页内嵌**定义图**框（`an-graph-canvas[mode=edit framed]` 定义态预览 + 缩放 + 进入编辑器；运行态归 scheduler）；编辑走**图编辑器海洋**（`features/graph-editor`：拖拽增删改连线 + 规范化 + 方向切 + 右岛检查器，**纯编辑无运行态**，编辑动作抛 `:edit` ops） | `workflow` 域；`:trigger` `:edit` 等 | ✅ |
| **Control**：CEL 分支编辑（when→port + emit） | entities sea | `control` 域 | ✅ 展示（branch-editor 排后） |
| **Approval**：模板（{{CEL}}）+ 决策规则编辑 | entities sea | `approval` 域 | ✅ 展示 |
| **Trigger / MCP / Skill**：源配置·activations / 连接态·tools / frontmatter·激活 | entities sea | `trigger`·`mcp`·`skill` 域 | ✅ |
| 版本 + diff（非 git 版本模型） | 实体页**版本 tab**（左 an-row 版本轨 + 右 `an-version-diff` 单框红绿 unified diff，逐版本对比）；function/handler/agent/workflow/control/approval 各按其 diff 字段 | `*/versions`；方案 A | ✅ |
| 试运行终端（输入表单→状态→输出→日志） | 右岛 Terminal（可执行 kind 自动挂） | `:run/:call/:invoke` 同步执行 | ✅ |
| AI 编辑实体（`:iterate` 开对话） | 实体 … 动作「AI 编辑」(sparkles) → 切 chat | `:iterate`→conversationId | 🔨 动作已挂、对话待 chat 海洋 |
| 执行/调用记录 + 详情展开（点行下方展开 溯源/耗时/错误 kv） | 实体页运行历史/调用记录（`an-row` + `detail` → `an-kv` 展开面板）；全可执行 kind 通用（function/handler/agent/workflow/trigger/mcp） | `/executions` `/calls` `/flowruns` 等 | ✅ |

---

## 2. Scheduler 海洋（Workflow 运行驾驶舱）

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 运行图（2D DAG + 回边 + 实时推进） | scheduler sea（复用 `an-graph-canvas` run 态——原语已支持 edit/run，run 态待此处接 flowrun tick；实体页/编辑器均纯定义态、不显运行） | flowrun tick（entities 流 ephemeral） | ▢ 面待建（图原语✅） |
| 节点逐行记忆化（`已记忆化` 徽标 / 循环 ×N） | run 节点活动列 | `flowrun_nodes` record-once（D3） | ▢ |
| 运行历史 + 状态（running/completed/failed/cancelled） | run 历史 chip 轨 | `GET /flowruns` | ▢ |
| 审批收件箱 + 决策（yes/no，倒计时，first-wins） | 右岛 / 通知 deep-link | `flowrun-inbox` · `:decide` | ▢ |
| replay 修复失败 run | run 动作 | `:replay` | ▢ |
| 生命周期（stage/activate/deactivate/kill） | workflow 头动作 | `workflow :stage/:activate/...` | ▢ |
| 并发策略（serial/skip/buffer/allow_all） | workflow 设置 | `concurrency` 字段 | ▢ |
| 关注态（run 失败点亮 / completed 熄灭，自愈） | 侧栏/通知红点 | `workflow.attention_changed` | ▢ |

---

## 3. Triggers（信号源，挂 Entities/Scheduler）

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 4 源配置（cron/webhook/fsnotify/sensor） | trigger 实体页 | `trigger` 域；`config` 自由 map | ▢ |
| 引用计数 / 监听态 / 最近 fire | 实体头 meta | 派生 refCount/listening/lastFiredAt | ▢ |
| 手动 fire | 实体动作 | `:fire`→activation | ▢ |
| activations 日志（“为什么没触发”可查） | trigger tab | `/activations` | ▢ |
| firings 收件箱（“触发了为什么没跑”处置） | trigger tab | `/firings`（pending/skipped/...） | 🅓 |

---

## 4. Chat 海洋（对话运行时，主战场）

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 会话侧栏（置顶/最近/归档 + 搜索/排序） | `features/chat/rail` | `conversations`（?sort/?archived） | ▢ |
| 块流：text/reasoning/tool_call/tool_result/progress | chat sea（块原语，tool_call/reasoning 默认折叠） | messages 6 块型（E2） | ▢ |
| subagent 子树嵌套 | 左导轨缩进 | E3 `parentBlockId` | ▢ |
| 工具危险确认（danger，逐次内存阻塞） | ApprovalGate(chat 味) | humanloop interaction（ephemeral 信号） | ▢ |
| agent 反问（ask）/ 决议 | 同上 | `/interactions` | ▢ |
| Todo 实时面板 | chat dock | todo 信号 | ▢ |
| 附件（多模态）/ @提及（冻结快照） | composer | `attachments` · mention | ▢ |
| 构建实体镜像（create/edit 流式填充右岛卡） | 右岛 EntityCard | entities 流 build 镜像 | ▢ |
| 压缩标记（compaction）/ 用量 / system-prompt 预览 | dev mode | `conversation.compacted` `/usage` | 🅓 |

---

## 5. Documents 海洋

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 文档树（嵌套 + 拖拽 + New） | `features/documents/rail` | `/documents/tree` `?parentId` | ▢ |
| 无标记 WYSIWYG 编辑（slash/选区工具/块 gutter） | documents sea（**逃生舱**：编辑器画布） | `documents` content | ▢ |
| move / duplicate（深拷子树） | 文档动作 | `:move` `:duplicate` | ▢ |
| 反链 / wikilink（relation 入边） | 右岛 | relation 边 | ▢ |
| @引用跳实体 | inline | Intent.select | ▢ |

---

## 6. 检索 / 记忆 / MCP / Skill

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 综搜 / 垂搜（混合 BM25+语义） | 全局搜索面 | `GET /search` | ▢ |
| 搜索设置（embedder 引擎态：ready/downloading/...） | settings | `/search/settings` | ▢ |
| Memory（pinned 常驻 + 列表 + pin/unpin） | memory 面 | `/memories` `:pin` | ▢ |
| MCP 服务器（列表+连接态 / 安装市场 / tools / reconnect） | mcp 面（连接态实时变色） | `mcp` 域；`status` 信号 | ▢ |
| Skill（文件编辑 + activate inline/fork + allowed-tools 预授权） | skill 面 | `skill` 域；`:activate` | ▢ |
| MCP 调用日志 / stderr | dev mode | `/calls` `/stderr` | 🅓 |

---

## 7. Notifications / Settings / Onboarding

| 能力 | demo 面 | 后端出处 | 状态 |
|---|---|---|---|
| 通知 inbox（“需要你”可操作 + FYI + 已读） | 铃铛轴 inbox | notifications 流；`workflow.approval_pending` 等 | ▢ |
| 设置：模型（3 场景）/ APIKey 保险箱(BYOK) / 工作区 / 搜索 / MCP / 运行时 | 头像轴 settings | workspace/apikey/model/sandbox 域 | ▢ |
| Onboarding（外观+语言 → API key → 搜索 provider） | 独立页 | `/providers` model-capabilities | ▢ |
| sandbox 运行时/磁盘/GC / 限额 | dev mode | `/sandbox/*` `/limits` | 🅓 |

---

## 待提后端的小改（🔧，随设计推进登记）

> 真 Flutter 前后端一起开工，此处记“demo 形态需要、后端小改即可满足”的项，避免凭空发明产品能力。

- _（暂无；逐海洋设计时增补）_
