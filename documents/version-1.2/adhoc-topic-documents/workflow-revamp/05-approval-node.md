# 05 — Approval 节点

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

依赖纲领:[`00-overview.md`](./00-overview.md) 的 durable execution 模型(执行器 + 事件日志 + 确定性重放)。

---

## 产品语义:yes/no + 携带上下文

approval 节点的本质:**执行器走到这里 → durable 地挂起 → 渲染一段说明给用户 → 等用户决策(批准 / 拒绝) → 从 approval 之后沿不同端口继续走**。

核心简化:
- **决策只 yes/no 二元**(不做开放输入 / 不做选择题 / 不做条件 approve)
- 复杂的人机交互(收集参数 / 选择 / 迭代)推回 chat 层处理
- 跟"chat 老板 / workflow 员工"分工一致——workflow 是参数已齐自动跑,approval 只是"流程中段的二元卡点"

approval 在执行模型里是**一个 durable 等信号的步骤**(不是 activity,不调外部能力):执行器记一条"等信号"事件 + 建一条 parked 的 approvals 记录,然后挂起这一条执行路径;用户决策到来时,把决策当作一个**信号事件**记进日志,执行器据此从 approval 之后继续。

---

## config 字段

```yaml
type: approval
config:
  prompt: |                                    # 必填,markdown,可插值
    AI 准备发送邮件:
    - 收件人:{{ payload.to }}
    - 主题:**{{ payload.subject }}**
    - 正文:
      {{ payload.body }}
    
    是否批准发送?
  
  timeout: 30d                                 # AI 编排时拍;不填 = 永不超时
  timeoutBehavior: reject                      # AI 编排时拍(reject / approve / fail);填了 timeout 必填此项
  allowReason: true                            # AI 编排时拍(true / false)
```

| 字段 | 必/选 | 说明 |
|---|---|---|
| `prompt` | **必填** | markdown 模板,支持 `{{ payload.* }}` / `{{ ctx.* }}` 插值——让用户看清在批啥 |
| `timeout` | 可选 | duration;**平台无默认**;不填 = approval 永不超时(挂到用户操作) |
| `timeoutBehavior` | 条件必填 | 填了 timeout 必填此项;`reject` / `approve` / `fail` |
| `allowReason` | 必填 | `true` / `false`;**平台无默认**,AI 编排时根据业务拍 |

**`prompt` 必填** — 没说明,用户看到孤零零的按钮根本不知道在批啥,无意义。

模板插值跟 agent prompt / tool args **完全同一套机制**——心智一致,无新概念。插值在执行器记"等信号"事件时一次性渲染完(读的是已记账的 payload / ctx),渲染结果连同 prompt 落进 approvals 记录;之后即便重放也直接读这条记录,不重算。

---

## 输出端口

| 端口 | 触发条件 |
|---|---|
| `yes` | 用户点批准 |
| `no` | 用户点拒绝 / 超时(若 `timeoutBehavior: reject`) |

下游可分别连不同节点。或者只连一个端口,另一端口默认结束这条执行路径。

approval 节点**不改 payload** — 上游传啥下游收啥,纯路由 + 等待。决策(yes/no)只决定走哪个出边,不往 payload 里塞东西。

---

## 执行 / 信号流

approval 是 durable execution 里的"挂起等信号"步骤(对位 Temporal 的 signal / Step Functions 的 `waitForTaskToken`)。一次完整生命周期:

```
执行器照图走到 approval 节点
         ↓
渲染 prompt 模板(读已记账的 payload / ctx)
         ↓
事件日志 append 一条 awaiting_signal 事件  +  建一条 approvals 记录(status=parked)
         ↓
这条执行路径在此挂起(不占 goroutine 死等;flowrun.status 转 awaiting_signal)
         ↓
推通知(in-app SSE + 桌面通知)
         ↓
用户操作 / 超时:
   ├─ 批准 → append signal_received 事件(decision=yes)+ approvals.status=approved
   │         → 执行器从 approval 之后沿 yes 端口继续(payload 透传)
   ├─ 拒绝 → append signal_received 事件(decision=no) + approvals.status=rejected
   │         → 执行器从 approval 之后沿 no 端口继续(payload 透传)
   └─ 超时 → 按 timeoutBehavior 处理(reject 走 no 端口 / approve 走 yes 端口 / fail 终止 flowrun);
            同样落一条 signal_received(来源=timeout)事件
```

**状态归属**:挂起期间的真相在 **approvals 表(parked)+ 事件日志的 awaiting_signal 事件**,不在任何内存里。决策本身是日志里的一条 signal_received 事件——**它是已记账的结果,所以重放时控制流走哪个端口是确定的**(满足 00 的确定性约束:控制流只读已记账的值)。

**进程重启 / 长部署重启,approval 状态自动保持**——靠 **approvals 表 + 事件日志确定性重放恢复**:重启后扫到 `status=awaiting_signal` 的 flowrun,从头重放程序、命中日志的步骤抄结果,走到这个 approval 时读到只有 awaiting_signal、没有 signal_received,就**继续挂起等信号**;若决策已在重启前到达(日志里已有 signal_received),重放直接沿对应端口往下走。详 [`00-overview.md`](./00-overview.md) 的"崩溃重放"。

---

## reason 字段 — 纯审计,不进数据流

用户操作 approval 时可选填一段 reason 文本:

- 写入 **flowrun 历史**(audit trail)——落在 approvals 记录的 `reason` 列(随 signal_received 一并记账)
- **不进**下游 payload
- 下游节点拿不到 reason

理由:reason 进数据流会让用户期待"AI 看 reason 改下次",workflow 变成迭代容器——这不是 workflow 的职责(那是 chat 的事)。reason 只是审计 / debug / 历史回顾的附属信息。

---

## UI 形态

approval inbox / 通知点开 / flowrun 详情页展示同一份内容(都读同一条 approvals 记录 + 其渲染后的 prompt):

```
┌─────────────────────────────────────┐
│ 待审批 · workflow "邮件助手"          │
├─────────────────────────────────────┤
│  [渲染后的 markdown 内容]            │
│   AI 准备发送邮件:                   │
│   - 收件人:user@example.com          │
│   - 主题:**Q3 预算报告**             │
│   - 正文:...                         │
│                                     │
│   是否批准发送?                      │
├─────────────────────────────────────┤
│  备注(可选):                        │
│  [文本框]                            │
│                                     │
│        [拒绝]      [批准]            │
└─────────────────────────────────────┘
```

触达通道:
- ✅ Forgify in-app notifications(SSE,已有)
- ✅ 桌面系统通知(macOS Notification Center / Windows Toast)— Wails 桌面 app 应启用
- ✅ 专门的 Approvals Inbox 页面(所有待审批一处看 = 扫所有 `status=parked` 的 approvals)
- ✅ flowrun 详情页里直接 approve(上下文丰富)
- ❌ 邮件 / 短信(单用户桌面不需要)

---

## 跟其他节点的关系

| 跟谁 | 关系 |
|---|---|
| **case 节点** | approval 也有命名分支(`yes` / `no`),但**固定两个**,不像 case 多路 + 动态。本质 approval 是 case 的"二元 + durable 等待"特例:case 当场按 CEL 选边,approval 挂起等一个外部信号再选边 |
| **agent 节点** | prompt 字段同一套模板插值机制 |
| **trigger 节点** | 跟 listener/手动两类触发都兼容——approval 状态持久化在 approvals 表 + 事件日志,不依赖任何常驻内存实例 |
| **tool 节点** | 不是 tool(tool 是同步调能力的 activity,approval 是 durable 等用户信号、不调外部能力) |

---

## 跟纲领的对齐

- 员工思维 ✓ — approval 节点接到输入 + 渲染 + 等响应 + 沿端口继续,不改变流程结构
- durable execution ✓ — 状态在 approvals 表 + 事件日志,挂起/恢复靠确定性重放,无常驻内存隐藏状态
- 用户心智简化 ✓ — 二元决策 + 可选备注,UI 极简
- 把复杂人机交互推回 chat ✓ — 收集参数 / 选择题 / 迭代等场景由 chat 处理

---

## 5 节点全集完成

workflow-revamp 5 个保留节点全部落档:

| 节点 | doc | 一句话 |
|---|---|---|
| trigger | [01](./01-triggers.md) | workflow 入口,程序起点(5 种 kind 统一 event 契约) |
| agent | [02](./02-agent-node.md) | LLM activity,4 类挂载 + outputSchema |
| tool | [03](./03-tool-node.md) | activity:调用 forge callable(function/handler/mcp/agent) |
| case | [04](./04-case-node.md) | 纯控制流:多路 switch + 回边形成结构化循环 |
| approval | [05](./05-approval-node.md) | durable 等信号:二元决策 + markdown prompt + 挂起恢复 |
