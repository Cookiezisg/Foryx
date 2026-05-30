# 04 — Case 节点 + 控制流

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 00-overview)。

依赖纲领:[`00-overview.md`](./00-overview.md) 的 durable execution 模型(执行器照图走 + 事件日志 journal + 确定性重放)。

---

## 一个节点覆盖所有控制流

废弃 `condition` / `loop` / `variable` 三个节点。合并成一种 **case 节点**:

- **多路 switch** 取代二元 if-else
- **回边** 形成**结构化循环**,取代嵌套 body 子图
- **变量完全砍** — 循环外算的值就是**程序作用域里的变量**,循环体直接读;真要持久化跨执行状态用 handler。不再有隐式全局变量

---

## case 节点形态

```yaml
type: case
config:
  expression: <CEL 表达式>       # 例: payload.category / payload.attempt > 5
  branches:                     # N 路命名分支,每个是输出端口
    invoice:
      to: handle_invoice_node
      # 不写 emit → 完全透传 payload
    inquiry:
      to: lookup_faq_node
      emit:                     # 可选:CEL 表达式构造下游 payload
        question: payload.text
    spam:
      to: end_node
    _default:
      to: notify_human_node
```

每个 branch 端口:

- `to` — 下游节点 ID(可连**任意节点**,**包括循环头 → 形成结构化循环**)
- `emit`(可选)— 每字段一个 CEL 表达式,**构造下游 payload**;不写 = **透传上游 payload**

case 的执行语义是**纯控制流、无副作用**:按 CEL 选中**一个**分支往下走,**没被选中的分支根本不执行**(不存在"消息在等没来的分支"这种空票问题——控制流去哪是程序定的)。case 的判断只读 payload(= 已记账的结果),因此**对确定性重放天然友好**(详 00 的"确定性"段)。

---

## Loop 表达 — case + 回边 = 结构化循环

```
trigger → [tool init] → [agent process] ←─┐
                              ↓             │
                         [case continueExpr]│
                              ├─ yes ────────┘   ← 回边,emit 时 attempt+1
                              └─ no → [tool finalize]
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
| **程序循环条件**(主要)| 编排者在 case expression 写合理终止:`payload.confidence > 0.9` / `payload.attempt > 5` 等 |
| **Workflow timeout**(兜底)| 用户/AI 编排时拍 `workflow.timeout`;不填 = 永不超时(整体跑多久强杀,资源安全兜底) |

跟 [`07-error-handling.md`](./07-error-handling.md) 的 Mechanism vs Policy 原则一致——平台**永远不猜**"100 次算异常"这种业务相关阈值。

> 注:超长循环(几千轮)会让事件日志变大、重放变慢,届时需要 continue-as-new(快照 + 新日志)。但"N 塞进工具"的哲学让循环天然短(都是有限重试,不是数据迭代),本地单用户不急。详 00 的"持久化"段。

---

## 表达式语言 = CEL

砍 Go text/template(模板渲染语言不是表达式语言)。

锁定 **CEL**(Google Common Expression Language,Go 实现 `google/cel-go`)。

理由:
- 业界标准 — K8s admission webhook / Istio / Tekton / OPA 全用 CEL,**LLM 训练数据见得最多**
- 强类型系统让 workflow accept 时能 validate expression(编排时报错优于运行时报错)
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
✅ [agent classifier outputSchema=enum] → [case payload.category]
```

### 平台约束(安全 / 资源兜底,跟业务无关)

| 项 | 值 | 类别 |
|---|---|---|
| 评估超时 | 100ms | **安全兜底**(防恶意表达式 CPU 卡死) |
| 可用变量 | 只 `payload` / `ctx`,不暴露 `env` 等隐式状态 | **机制层**(纯控制流只读已记账的值) |
| 可调函数 | CEL 默认包(string / list / time);**不暴露 LLM 调用 / HTTP / 任意计算** | **安全兜底**(case 不该有副作用,否则破坏确定性重放) |

表达式长度等"业务相关"约束**不在平台层**——AI 编排时自律(写太长就拆成 agent + case 组合)。

---

## 输出语义

case 节点**按 branch 的 emit 表达式构造下游 payload**(不写 emit 则透传):

- 评估 expression → 选中**一个** branch X(没选中的分支不执行)
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
[case payload.category]
  ├─ invoice → [tool: handle_invoice]
  ├─ inquiry → [tool: lookup_faq]
  ├─ spam    → 结束
  └─ _default → [tool: notify_human]
```

意图识别不需要"专门 intent 节点"——agent + case 自然组合。Forgify Phase 5 backlog 里那个 `intent` domain 不需要在 workflow 这边做。

---

## 跟 variable 砍除的关联

variable 节点砍除后,跨节点状态有两条路:

1. **程序作用域变量**:循环外算的值(节点 A 的输出)就是作用域里的一个变量,下游节点 / 循环体直接读出来。**因果可追**(每个值都记进事件日志,能 trace 回它的产生节点)
2. **真持久化状态**:用 handler stateful class(跨执行复用的实例,Owner 由 lifecycle 决定)

variable 想做的"workflow 级全局变量"完全被这两条覆盖,且**没有隐式状态污染**——任何节点拿到的值都能从事件日志 trace 回它的产生节点。

---

## 累计节点数

跟纲领锁定的 5 节点:

| | |
|---|---|
| trigger / agent / tool / case / approval | 保留 |
| 砍 9 个 | llm / function / handler / mcp / skill(独立) / condition / loop / variable / parallel / wait / http |

**14 → 5**。控制流只剩 case 一种,其他控制能力(并发 = fork-join / 延迟 = durable timer / 状态 = 作用域变量或 handler)在 infra 层 / 程序结构原生表达。
