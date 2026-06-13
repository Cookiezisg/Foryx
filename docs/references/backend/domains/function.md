---
id: DOC-011
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# function —— 无状态 Python 沙箱函数（Quadrinity 第一元）

## 1. 定位

用户锻造的**无状态** Python 代码：每次调用在**全新隔离的沙箱进程**里跑一遍就退出（对比 handler 的常驻进程）。代码层级：`domain/function`（实体+错误+Repository 端口，零外部依赖）→ `app/function`（锻造/执行/env 编排 + 三适配器）→ `infra/store/function`（orm 三表）+ `app/tool/function`（9 个 LLM 工具）+ transport。

## 2. 心智模型（先懂这个，代码就顺了）

**三个对象**：`Function`（身份：name/description/tags + 一个 `ActiveVersionID` 指针，**代码不在这**）→ `Version`（**不可变快照**：code + inputs/outputs 声明 + 依赖 + Python 版本 + **env 镜像**）→ `Execution`（一次运行的终态审计行）。

**版本模型 = 线性只增 + 自由指针**（三实体统一，"方案 A"）：
- 每次 edit = 在 active 基础上套 ops → 写**新** Version（号 = max+1，永不重排）→ 指针移过去，**立即生效**。没有 pending/accept/审批态。
- revert = **纯指针移动**到旧版本号——不复制、不删"更新的"版本，可以再 revert 回来。
- 版本数 cap 50，超出硬删最老的，**但绝不删 active**（revert 后 active 可能很老）。

**env 模型**：每个 Version 配独立 venv（`EnvID`，前缀 `fnenv_`——infra 自有前缀，不从 fn_ 派生）。sandbox 的 owner key = `functionID_envID` 复合（每版本的 venv 独立可寻址；Destroy 按 `functionID_` 前缀扫）。Version 行上有 env 的**状态镜像**（pending/syncing/ready/failed + 错误 + 同步时间）——读 Version 即知 env 健康度，不用查 sandbox。

## 3. 物理模型

三表 + 索引/约束见 [database.md](../database.md)。设计取舍：
- name 用 **partial-UNIQUE**（`WHERE deleted_at IS NULL`）：软删后名字立即可复用；唯一性**只**靠这个 DB 约束兜（app 层无预检——TOCTOU 安全且少一次查询），store 把 `orm.ErrConflict` 翻成 `FUNCTION_NAME_DUPLICATE`。
- 执行表的 5 列溯源（conversation/message/toolCall + flowrun/flowrunNode）全部从 **ctx** 读：chat 身份由 loop 注入，flowrun 身份由 workflow 调度器派发前注入（`reqctx.SetFlowrunID`）——哪条路径跑的就带哪份，执行入口签名不沾这些概念。

## 4. 生命周期 / 关键流程

**锻造（create/edit）**：唯一的变更词汇是 **ops**（JSON 判别式：`set_meta/set_code/set_inputs/set_outputs/set_dependencies/set_python_version`，闭集）。三条入口殊途同归——LLM 工具传 ops、HTTP `:edit` 传 ops、HTTP create 的扁平 payload 由 `buildOpsFromDirect` **反推成 ops**——全走 `ApplyOps`：逐 op 应用到 `VersionDraft` + **每步后** `validateIncremental`（name 正则 + 字段 schema）+ 末尾 `validateFinal`。LLM 的脏 JSON 先过 `jsonrepair` 容错。错误码：op 畸形/中途非法 = `FUNCTION_OP_INVALID`；终校验失败 = `FUNCTION_INVALID_CODE`。

**代码校验是刻意的词法检查、非 AST**：要求至少一个顶层 `def `（首个 def 名即执行入口）；黑名单 `import forgify_handler`（**D7 边界**：function 无状态、handler 持久——function 不许碰 handler SDK）。

**env 物化（`ensureEnv`）**：写 syncing → 委托 `envfix.Provisioner`（带 LLM 改依赖的修复循环，≤3 次——装不上时让 LLM 改依赖列表重试）→ 终态（ready/failed + **修正后的依赖**）写回 Version 行。create/edit **容忍**失败（env failed 也创建成功，状态可见）；run 时未 ready 才报 `FUNCTION_ENV_NOT_READY`。`Edit` 空 ops = "重建 active env"路径（重试失败的安装），发 `function.env_rebuilt`。

**执行（`RunFunction`，所有路径唯一漏斗）**：nil input 在 runner 前归一成 `{}`（driver 做 `f(**input)`，nil→JSON `null`→`f(**None)` TypeError；无参调用方如 sensor/无接线 workflow 节点不该崩）；取版本（空→active）→ env 未 ready 则懒物化 → `runner.Run` → **`ErrEnvNotFound`（env 被 GC 回收）= 重建 env + 重试一次** → `recordExecution`。四个调用方：`run_function` 工具（chat/agent，按 ctx 推）、HTTP `:run`（manual）、workflow 调度器 `dispatch.RunAction`（workflow，fail-fast：`OK=false` 转 error 使节点行写 failed）、sensor 触发器。

**沙箱驱动（精妙处）**：`SandboxAdapter.Run` 写 `main.py` = 用户代码 + driver 模板。driver 在调用期间**把 stdout 重定向到 stderr**、结束后才把 JSON 结果打回真 stdout——既保证 stdout 是可解析的单一 JSON，又让函数自己的 `print()` 变成实时进度（stderr 被**三写**：messages 流 tool_call progress + entities 流 run 终端 + `pkg/logtail` 限长收集器——后者随执行记录落 `logs` 列，run_function 的返回也携带）。

**env 物化可见性**：ensureEnv 把每次尝试/模型修复行经 `envfix.WriterSink` tee 到 entities 流 forge 终端（不分入口——HTTP 编辑器路径与 chat 锻造同等可见）；状态级信号另走 `sandbox.env_status_changed` 通知（installing/ready/failed 带 errorMsg）。

**记账（`recordExecution`）**：best-effort、走 `reqctx.Detached(wsID)`（被取消的运行仍落审计行）；status 按 `ctx.Err()` 区分 timeout/cancelled/failed；`logs` 随行落盘（List 置空、单条 Get 携带）。

## 5. 关键设计决策

- **ops 是唯一变更词汇**：LLM/HTTP/直接创建三口径合一管线，校验只写一处。
- **无 pending/accept**：本地单用户，编辑立即生效 + revert 兜底，审批态是多余仪式。
- **env 失败不阻塞创建**：让用户先看到实体 + env 状态，修复（edit 空 ops / 改依赖）是后续动作。
- **Search/ListAll 全表载入内存过滤**：本地单用户规模的标准取舍（catalog/relation 同路）。
- 删除 = 软删 + best-effort 销毁 env/代码目录 + 硬删 relation 边。

## 6. 契约（引用，不重列）

端点 → [api.md](../api.md)#function · 表 → [database.md](../database.md)#function · 码（`FUNCTION_*`，domain 12 + 工具校验 4）→ [error-codes.md](../error-codes.md) · 事件 → [events.md](../events.md)。LLM 工具 9 个：search/get/create/edit/revert/delete/run + 执行日志两查询；create/edit 是 **forge 工具**（流式 code args 镜像 entities 流，面板实时填充；env-fix 尝试折进结果 + 实时流出）。

## 7. 跨域集成

- **执行被谁调**：chat loop（`run_function`）/ HTTP / workflow 调度器（窄接口 `FunctionRunner`，bootstrap/dispatch.go）/ sensor。
- **能力暴露**：catalog（name+desc 名录）· @ 提及（快照 description+active 代码）· relation 图（`NamesByIDs` 读时 hydrate + conversation→function 的 create/edit 边，edit 边在每次 active 变更时重算、origin 对话除外）。
- **agent 挂载**：`fn_<id>` 可被 agent 挂为专属工具（见 [agent.md](agent.md)#3）。
- **依赖的端口**：sandbox（Run/Destroy/Ready）· envfix.Provisioner（env 物化+LLM 修复）· relation/notification（nil 容忍）。
