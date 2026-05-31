# 04 — Case 节点 + 控制流

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 00-overview)。

依赖纲领:[`00-overview.md`](./00-overview.md) 的 durable execution 模型(执行器照图走 + 事件日志 journal + 确定性重放)。

---

## 一个节点覆盖所有控制流

废弃 `condition` / `loop` / `variable` 三个节点。合并成一种 **case 节点**:

- **多路 switch** 取代二元 if-else
- **回边** 形成**结构化循环**,取代嵌套 body 子图
- **变量完全砍** — 循环外算的值就是**程序作用域里的变量**,循环体直接读;真要持久化跨执行状态写进 **journaled 作用域变量 / payload** 或**外部 store**(DB 经 handler 方法,或 Forgify document·memory 实体)。handler 实例进程内存只放可重建的 ephemeral 资源(连接池 / 缓存 / 客户端),绝不跨执行保存业务态。不再有隐式全局变量

---

## case 节点形态

```yaml
type: case
config:
  branches:                       # 有序列表;逐条求 when:(布尔 CEL),第一个为 true 的中选
    - when: payload.category == "invoice"
      to: handle_invoice_node
      # 不写 emit → 完全透传 payload
    - when: payload.category == "inquiry"
      to: lookup_faq_node
      emit:                       # 可选:CEL 构造下游 payload
        question: payload.text
    - when: payload.category == "spam"
      to: end_node
    - when: "true"                # 末条兜底(总命中,= 旧 _default)
      to: notify_human_node
```

每个 branch:

- `when` — 一个**布尔 CEL 守卫**;branches 按顺序求值,**第一个为 true 的中选**(first-true-wins)。**末条写 `when: "true"` 作兜底**(必须有,否则全 false 时无路可走)。
- `to` — 下游节点 ID(可连**任意节点**,**包括循环头 → 形成结构化循环**)。
- `emit`(可选)— 每字段一个 CEL 表达式,**构造下游 payload**;不写 = **透传上游 payload**。

> **为什么 per-branch `when:` 守卫,而不是单个 `expression` 按值匹配分支名**:研究 finding A 实测——`expression == 分支名` 只对**分类**场景能用(~100%),**布尔路由**(如 `payload.attempt > 5` 配 fast/slow 分支名)会崩到 0-18%;改成每分支一个布尔 `when:` 守卫后,**分类 + 布尔统一到 ~100%**(四处验证)。详 [`13-llm-facing-implementation-guide.md`](./13-llm-facing-implementation-guide.md) §1-A / [`14-llm-validation-research-record.md`](./14-llm-validation-research-record.md)。

case 的执行语义是**纯控制流、无副作用**:逐分支求 `when:` 布尔,**第一个 true 的中选、往下走**,**其余分支根本不执行**(不存在"消息在等没来的分支"这种空票问题——控制流去哪是程序定的)。`when:` 只读 payload / ctx(= 已记账的结果),因此**对确定性重放天然友好**(详 00 的"确定性"段)。

---

## Loop 表达 — case + 回边 = 结构化循环

```
trigger → [tool init] → [agent process] ←─┐
                              ↓             │
                         [case 分支 when:]   │
                              ├─ when 继续条件 ─┘   ← 回边,emit 时 attempt+1
                              └─ when "true" → [tool finalize]
```

这是**程序里的结构化循环**(对位 00 的"循环与汇合"段):一个明确的循环头(这里是 `agent process`),分支从头进、绕回头。执行器**只重跑循环体内的节点**;循环外算的值(如 `tool init` 的输出)是**作用域变量**,循环体每轮直接读、不重算;崩溃重放只补没做完的那轮,绝不从头傻跑。

业务"第几次"计数 **由编排者放 payload**(平台不管):

- case 回边时在 `emit` 里显式 `attempt: (payload.attempt || 0) + 1`
- 下游节点(agent prompt / 后续 case)读 `payload.attempt`

平台**不**提供 `iterationIdx` 这种平台层业务计数字段 — 跟 Mechanism vs Policy 原则一致,计数是业务的事。

> 内部实现细节:执行器用"节点 + 第几轮"(`iteration_key`)当**日志查找键**区分同一节点不同轮次的结果——但它只是**重放去重键**,不是用户可见的版本、没有偏序匹配、没有前沿。详 00 的"循环与汇合"段。

---

## 回边的硬约束 — 图必须良构 / 可归约

case 是唯一能产生回边的节点,但**回边不能乱画**。accept 时校验器要求图**良构 / 可归约**:

- **循环单入口** — 一个明确的循环头,大家从头进、绕回头;不允许跳进循环体中间。
- **分支自包含** — 循环体里的状态不跳到循环外,外部也不跳进循环体中间。

**鬼画符式的乱回边(交叉 / 不可归约)会被 accept 拒并说清原因**。AWS Step Functions 也是这么强制的。对"AI 画规整工作流 + 画布可强制结构"的 Forgify,这反而更干净——真实的回环(重试到成功 / 改到合格 / 轮询到就绪 / 修一部分留另一部分)全是良构循环。执行语义(执行器怎么走回边、怎么重放)细节见 00 的"循环与汇合"段,这里不重述底盘。

---

## 终止 / 死循环防护

平台**不**对"循环轮数"设业务相关 hard cap。终止靠两层:

| 层 | 谁定 |
|---|---|
| **程序循环条件**(主要)| 编排者在 case 的 `when:` 守卫写合理终止:`payload.confidence > 0.9` / `payload.attempt > 5` 等 |
| **Workflow timeout**(兜底)| 用户/AI 编排时拍 `workflow.timeout`;不填 = 永不超时(整体跑多久强杀,资源安全兜底) |

跟 [`07-error-handling.md`](./07-error-handling.md) 的 Mechanism vs Policy 原则一致——平台**永远不猜**"100 次算异常"这种业务相关阈值。

> 注:超长循环(几千轮)会让事件日志变大、重放变慢,届时需要 continue-as-new(快照 + 新日志)。但"N 塞进工具"的哲学让循环天然短(都是有限重试,不是数据迭代),本地单用户不急。详 00 的"持久化"段。

---

## 表达式语言 = CEL

砍 Go text/template(模板渲染语言不是表达式语言)。

锁定 **CEL**(Google Common Expression Language,Go 实现 `google/cel-go`)。

理由:
- 业界标准 — K8s admission webhook / Istio / Tekton / OPA 全用 CEL,**LLM 训练数据见得最多**
- accept 期能做**部分**校验(诚实范围,详下"accept 期校验的诚实范围"):语法/parse + 变量作用域(只许 `payload`/`ctx`)+ 函数白名单 + 对 typed 的 `ctx` 做类型检查;**查不了** payload 的字段类型/存在(payload 动态无类型),残留靠运行时兜
- Google 官方维护,长期可靠
- 设计就是沙箱 + null 安全

例子:

```cel
// 简单分类(意图识别场景)
payload.category == "invoice"

// 终止条件(case 回边场景,业务计数靠 payload)
payload.attempt > 5
payload.attempt > 5 || payload.confidence >= 0.9

// 字段判断
payload.items.size() > 0
payload.user.name.startsWith("admin")

// 复合条件
payload.score >= 0.8 && ctx.triggerKind == "polling"

// 包含判断
"important" in payload.tags
```

### 字段 → 语法对照(全平台一套语言,用法由字段类型定死)

**全平台只有一种表达式语言 = CEL。** 按字段输出类型分两种用法,**由字段类型定死、作者不用选**:

| 字段 | 输出类型 | 写法 | 例子 |
|---|---|---|---|
| `case.when` | 布尔 | **裸 CEL** | `payload.attempt > 5` |
| `case` 分支 `emit` 的字段值 | 类型化值 | **裸 CEL** | `attempt: (payload.attempt \|\| 0) + 1` → 数字 |
| `tool.args` 的字段值 | 类型化值 | **裸 CEL** | `x: payload.x + 1` → 数字 `6` |
| `agent.prompt` | 文本文档 | **模板串 `{{ CEL }}`** | `分析以下工单:{{ payload.text }}` |
| `approval.prompt` | 文本文档 | **模板串 `{{ CEL }}`** | `批准对 {{ payload.user.name }} 的退款?` |

- **求值/布尔字段**(`case.when`、`emit` 字段值、`tool.args` 字段值)→ 裸 CEL,**产出类型化值**(如 `payload.x + 1` 出数字 `6`,不是字符串 `"6"`)。
- **文本文档字段**(`agent.prompt`、`approval.prompt`)→ 模板串 `{{ CEL }}`,**`{{ }}` 里是 CEL,求值后字符串化插入**。
- **`{{ }}` 不是第二种语言**,只是 CEL 的**插值定界符**。
- **Go text/template 作为语言整个退役** — 没有 `if`/`range`/`funcMap` 这类控制流;需要拼列表用 CEL 函数一行搞定(如 `{{ payload.items.map(i, i.name).join(", ") }}` 出逗号串)。
- 实现:`backend/internal/app/workflow/expression.go`(原 text/template)退役 → **一个 CEL 求值核心 + 一个薄的 `{{ CEL }}` 插值 pass**。

### accept 期校验的诚实范围

accept 时 CEL **能查**:语法/parse + 变量作用域(只许 `payload`/`ctx`,挡 `env.x`/`nodes.X` 这类隐式状态)+ 函数白名单 + 对 typed 的 `ctx` 做类型检查。

accept 时 CEL **查不了**:payload 的字段类型 / 存在(payload 动态无类型)。

残留由运行时兜两层:

- **N1**(forged agent run 强制 `outputSchema`)— agent 真吐合规值,详 02-agent-node。
- **G9**(case guard fail-to-false)— case guard 求值出错时落 `_default`,详 13-llm-facing-implementation-guide。

即:**结构性错误 accept 挡、payload 值错运行时优雅降级**。

### 产品边界 — case 是"看牌发牌员",不是"分析师"

case 节点的职责**严格限定**:**对上游已经准备好的字段做简单判断 → 路由**。

| 不该塞进 case 表达式 | 应该走的路 |
|---|---|
| 计算 / 统计 / 数据转换 | 上游用 agent 节点(LLM)或 tool 节点(forge function)产出结果,case 只读 |
| 调用外部 API 判断 | 上游用 tool 节点 |
| 多步骤推理 | 拆成多个 case 串联 或 上游 agent 节点判断 |

跟员工思维一致 — case 是简单看牌的发牌员。要"分析"必须上游做完,case 只看结果。

反模式:

```
❌ [case 复杂 CEL 表达式硬塞业务逻辑]
```

正确模式(意图识别):

```
✅ [agent classifier outputSchema=enum] → [case: per-branch when: 守卫]
```

### 平台约束(安全 / 资源兜底,跟业务无关)

| 项 | 值 | 类别 |
|---|---|---|
| 评估超时 | 100ms | **安全兜底**(防恶意表达式 CPU 卡死);**撞超时 = 记一笔确定的"超时"结果进 journal**(重放抄它、不重算),不让机器负载决定分支 |
| 可用变量 | 只 `payload` / `ctx`,不暴露 `env` 等隐式状态 | **机制层**(纯控制流只读已记账的值) |
| 可调函数 | CEL 默认包的 string / list + time 的**纯运算**(duration/timestamp 算术,作用于已记账的值);**禁读取当前时刻**(`now()` / wall-clock——控制流读墙钟会让重放分叉)、不暴露 LLM 调用 / HTTP / 任意计算 | **安全兜底 + 确定性**(case 无副作用且不读墙钟,否则破坏确定性重放) |

表达式长度等"业务相关"约束**不在平台层**——AI 编排时自律(写太长就拆成 agent + case 组合)。

---

## 输出语义

case 节点**逐分支求 `when:` 守卫、第一个 true 的中选,按其 emit 构造下游 payload**(不写 emit 则透传):

- 按顺序求各 branch 的 `when:` 布尔 → 第一个 true 的为 branch X(其余不执行;末条 `when:"true"` 保证总有路)
- 取 branch X 的 `to`(下游节点)+ `emit`(可选)
- 下游 payload:`payload = branch.emit ? evalCEL(branch.emit, payload) : payload`
- 这一步选择本身按序记进事件日志(`branch_taken`),**因果可追**

case 节点**不做业务计算 / 推理** — `emit` 表达式只用于"构造下游需要的 payload 形状"(包括 attempt+1 这种简单计数),业务逻辑(分类 / 提取 / 推理)仍由上游 agent / tool 节点完成。case 是"看牌发牌员",不是"分析师"。

---

## 跟意图识别的天然对齐

agent 节点的 `outputSchema: enum` 输出 + case 节点的 switch 完美咬合:

```
trigger
  ↓
[agent classifier]              ← outputSchema: enum [invoice, inquiry, spam]
  ↓ payload.category = <enum 值>
[case]                          ← per-branch when: 守卫,first-true-wins
  ├─ when payload.category=="invoice" → [tool: handle_invoice]
  ├─ when payload.category=="inquiry" → [tool: lookup_faq]
  ├─ when payload.category=="spam"    → 结束
  └─ when "true"                      → [tool: notify_human]
```

意图识别不需要"专门 intent 节点"——agent + case 自然组合。Forgify Phase 5 backlog 里那个 `intent` domain 不需要在 workflow 这边做。

---

## 跟 variable 砍除的关联

variable 节点砍除后,跨节点状态有两条路:

1. **程序作用域变量**:循环外算的值(节点 A 的输出)就是作用域里的一个变量,下游节点 / 循环体直接读出来。**因果可追**(每个值都记进事件日志,能 trace 回它的产生节点)
2. **真持久化状态**:写进 **journaled 作用域变量 / payload**,或**外部 store**(DB 经 handler 方法调用,或 Forgify document·memory 实体)。handler 实例是 **per-flowrun 隔离**的(Owner 恒为 `{Kind:"flowrun", ID:flowrunId}`,首次调用时 lazy spawn、flowrun 结束时随之销毁),进程内存只放可重建的 ephemeral 资源,绝不跨执行保存结果态 / 累积态

variable 想做的"workflow 级全局变量"完全被这两条覆盖,且**没有隐式状态污染**——任何节点拿到的值都能从事件日志 trace 回它的产生节点。

---

## 累计节点数

跟纲领锁定的 5 节点:

| | |
|---|---|
| trigger / agent / tool / case / approval | 保留 |
| 退役 / 合并(11 项)| llm / function / handler / mcp / skill(独立) / condition / loop / variable / parallel / wait / http —— function/handler/mcp/skill 的**调用**并入新 `tool` 节点、condition → `case`、llm → `agent`、loop/variable/parallel/wait/http → case 回边 / 作用域变量 / fork-join / durable timer |

**14 → 5**。控制流只剩 case 一种,其他控制能力(并发 = fork-join / 延迟 = durable timer / 状态 = 作用域变量或 handler)在 infra 层 / 程序结构原生表达。
