# R0046 — approval 渲染实体（18 图模型第 2 个落地实体）

> control（R0045）之后，18 图模型 5 节点 × 5 实体的**第 2 个落地**。把旧 `approval` 节点内联的「prompt 模板 + 决策规则」物化成独立 **审批渲染实体**——「把数据渲染成人能看懂的审批点」。与 control 几乎双胞胎（同 AI 工作实体范式），3 处真实差异。

## 前缀分层修正（重要）

- **`apf_`/`apfv_`（approval **form**）= 渲染实体（配置）** —— 本轮做的。
- **`apv_` = `approvals` 运行时表**（per-flowrun 的 parked/approved 记录，17 §1，波次 4）—— **已被占用**。
- 18 文档 §3.2/§8 我之前写 `apv_`/`apvv_` **撞了运行时前缀** → 本轮 **doc-fix 全局改 `apf_`/`apfv_`**。对位 trigger 实体（`trg_`）vs `trigger_firings`（运行时）。

## 跟 control 的 3 处真实差异

1. **内容是 template + 决策规则**（不是 branches）：`template`(markdown) + `allowReason`(bool) + `timeout` + `timeoutBehavior`(reject/approve/fail)。
2. **`{{ CEL }}` 模板**（不是裸 CEL）→ **给 `pkg/cel` 加模板地基**（强化地基，非业务层手搓，原则 #8）：`Template` 类型 + `CompileTemplate`（提取 `{{ expr }}` 各段编译，approval 校验用）+ `Render`（求值拼接，波次 4 渲染用）。**agent.prompt（波次 4）复用同一套。**
3. **timeout duration**（支持 `30d`/`2w`）+ `timeoutBehavior` 联动校验（`time.ParseDuration` 不支持 d，`ParseTimeout` 在 approval domain 扩展；波次 4 durable timer 复用时抽 pkg）。

## 实现（地基 → domain → store → app → tool → handler）

- **pkg/cel**：`template.go`（`Template`/`CompileTemplate`/`Render` + `stringify`）+ 测试。
- **domain/approval**：`ApprovalForm`(apf_) + `Version`(apfv_) + `ValidateForm`（template 非空 + timeout↔behavior）+ `ParseTimeout`（d/w 扩展）+ `IsValidTimeoutBehavior` + **7** `errorsdomain`。
- **infra/store/approval**：orm 两表 + 手写 DDL（`approval_forms` partial-UNIQUE / `approval_form_versions` UNIQUE(approval_id,version)；`allow_reason INTEGER` bool）+ MaxVersion + TrimProtectsActive。
- **app/approval**：Service（Create/Edit/Revert/UpdateMeta/Get/List/Search/Delete/**Resolve**）+ `validateForm`（结构 domain + `CompileTemplate`）+ catalog/relation/notif。
- **app/tool/approval**：6 Lazy 工具（无 run）。
- **transport/handlers/approval**：REST（无 `:run`/pending）。
- **relation/entitykind**：第 **11** 类 EntityKind `approval` + 前缀 `apf_`。

## 测试（全离线 · 0 token）

- **pkg/cel**：CompileTemplate（纯文本/单/多段/未闭合/语法错/`now()`）+ Render（插值 + 数字字符串化 + 纯字面）。
- **domain**：ValidateForm 表驱动 + ParseTimeout（d/w/h/m + 非法）+ IsValidTimeoutBehavior。
- **store**：真 SQLite + ws ctx —— 往返 / **template+规则+allow_reason bool 往返**（验证 orm bool↔int）/ 重名 / 隔离 / 软删 / 分页 / MaxVersion / TrimProtectsActive / SetActive / GetByIDs 序。
- **app**：真 store —— Create(v1 + 规则往返) / EmptyTemplate / BadTemplateCEL / TimeoutNoBehavior / BadDuration / EmptyName / Dup / Edit / Revert / UpdateMeta(不 bump) / Search / Delete / Resolve(active+pinned 返 *Version)。
- **tool**：Wiring(6) + ValidateInput 表驱动 + RoundTrip + InvalidTemplate `errors.Is`。
- **relation**：`entitykind_test` 加 `apf_` 用例 + 全集加 `approval`。

## 契约文档（doc-sync）

- 新建 `domains/approval.md`（DOC-306）。
- `database.md`：§1 全索引 + **§4.6 Approval** + 前缀 `apf_`/`apfv_` + 注释（强调 ≠ apv_）。
- `api.md`：**§2.7 Approvals**。
- `error-codes.md`：**§2.6c** 7 个 `APPROVAL_*`。
- `domains/relation.md`：§1.2 **10 → 11**。
- `contract-changes.md` **#26**。
- **`18-graph-model-redesign.md` doc-fix**：apv_/apvv_ → apf_/apfv_（前缀撞运行时表，全局改）。

## 留 M7（总装配）

`ApprovalTools` → `Toolset.Lazy`、`ApprovalHandler.Register`、catalog `RegisterSource`、relation `Namer['approval']`、`approvalstore.Schema` → `db.Migrate`、`SetRelationSyncer`。

## 验证

gofmt clean · `go build ./...` BUILD_OK · `go vet` VET_OK · test pkg/cel + domain/store/app/tool/relation **ALL PASS**（纯新增 + cel 加模板 + relation 加一类，不破坏任何既有包）。

## 波次 4 进度

18 图模型 5 节点 × 5 实体：trigger（trg_，R0039 已建）· agent（ag_，R0043）· action→callable（fn/hd/mcp，已建）· **control（ctl_，R0045）** · **approval（apf_，R0046）**。**两个新 AI 工作实体收齐**。下一站：**workflow domain 改造**（node 引用实体、边 = payload）→ flowrun（journal）→ scheduler（durable interpreter，消费 control.Resolve / approval.Resolve）。
