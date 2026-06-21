---
id: DOC-012
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# handler —— 有状态常驻 Python 类（Quadrinity 第二元）

## 1. 定位

用户构建的**有状态** Python 类：每个 handler 跑**一个长生命周期的常驻进程**（像 MCP server）——开局/首调 spawn、跨调用保活（`self.xxx` 状态留存）、edit（改类代码/schema）/改 config/crash 时重启（**纯 meta 改名/改述不重启**——见 §5）、退出软件才优雅关闭。**所有调用方（chat/agent/workflow）共享这一个实例**（真共享状态）。代码层级：`domain/handler` → `app/handler`（16 文件，最大的实体 app）→ `infra/store/handler` + `infra/handler`（RPC 客户端）+ `app/tool/handler`（12 工具）。

## 2. 心智模型

**四个对象**：`Handler`（身份 + active 指针 + **`ConfigEncrypted`**：init-args 的值，加密存盘）→ `Version`（不可变快照：类的**各部分**——imports/init_body/shutdown_body/**methods**/**init_args_schema**/依赖 + env 镜像）→ `Call`（一次方法调用的审计行）→ `Instance`（**内存态**运行时：进程 + RPC 客户端，`hdi_` 前缀，不落库）。

**与 function 的本质差异一句话**：function 的执行单位是"一次进程"，handler 的执行单位是"常驻进程上的一次 RPC"。版本模型/构建管线/env 物化与 function **完全同构**（见 [function.md](function.md)#2/#4——本文不重复），下面只讲 handler 独有的。

**类不是用户直接写的整文件**——是**组装**出来的：Version 存类的各部分，`AssembleClass` 生成 `user_handler.py`：
```python
class HandlerImpl:
    def __init__(self, api_key: str, base_url: str = "..."):   # ← init_args_schema 生成签名
        <init_body>
    def shutdown(self):
        <shutdown_body>
    def send(self, to: str):                                    # ← 每个 MethodSpec 一个 def
        <method body>
```
**两套参数体系刻意分开**：method 的 I/O 用通用 `schema.Field`（与 function/agent 同款）；`__init__` 的参数用专属 `InitArgSpec`——因为它带 `Required/Sensitive/Default` 语义（API key 等实例化配置，加密存+读时掩码），不是 method I/O。

**spawn 单飞**：实例缺失/crashed 时并发调用（chat 并行工具批）共享一次 in-flight spawn（per-handler done channel），不重复支付秒级 env+进程+__init__ 开销。

## 3. 物理模型

三表见 [database.md](../database.md)#handler。独有列：`handlers.config_encrypted`（整 blob AES-GCM）、`handler_calls.instance_id`（哪个实例服务的这次调用——重启后实例 id 变，可据此分代）。

## 4. 生命周期 / 关键流程

### 实例生命周期（`instanceManager`，handler 的灵魂）

`map[handlerID]*Instance` + mutex，**每 handler 至多一个**：
- **Get**：有且未 crashed → 直接用；crashed → 摘掉+杀掉，重新 spawn；并发缺失/crashed 走 §2 的 per-handler 单飞 channel（并发调用方等 channel settle 后重新 Get，绝不造重复实例）。
- **Boot**：启动时为每个"active 版本 env-ready 且 config 完整"的 handler 预热实例（best-effort，起不来就停着、首调重试）。
- **Restart** = Stop + Get（edit/revert/改 config 后必走——实例要吃新代码/新 config）；**StopAll** = 退出软件。
- `State()` 报 running/stopped/crashed（`Get` 单读上 RuntimeState 计算字段）。

### spawn 链（`spawnInstance`）

加载 active 版本 + **解密** config → 校验必填 init-args（缺 → `HANDLER_CONFIG_INCOMPLETE`，不 spawn）→ **按 active schema 过滤 config**（孤儿 key——被后续版本删掉的 arg、revert 留下的——会成为 `__init__` 的意外 kwarg → Python TypeError → 永久 spawn 失败；在 spawn 这个唯一咽喉点过滤，防住所有漂移来源）→ env 未 ready 则物化（尝试/修复行 tee 到 entities 流 build 终端，同 function）→ `AssembleClass` 写 `user_handler.py` + `driver.py` → 起长跑 `python driver.py` → **`ErrEnvNotFound`（env 被 GC）= 重建+重试一次** → stderr 进日志（崩溃诊断）**并进实例级 stderr 扇出**（`stderrFan`——调用挂 per-call sink 收窗口内输出）→ `client.Init(config)` 跑 `__init__`。**driver 协议护盾**：进程启动即把用户态 stdout 整体重定向到 stderr（import/`__init__`/method/shutdown 里的 print() 全部变成调用日志），协议帧只经保存的真 stdout 写——一个 print() 永远炸不了协议（与 function driver 同款护盾）。

### RPC 协议（`infra/handler` 客户端 ↔ `driver.py`）

stdio 行-JSON：`init`→`ready`/`init_error`；`call{id,method,args}`→`return`/`error`（带 Python traceback）/`progress`×N（generator 的每个 `{"progress":...}` yield）；`shutdown`→跑 `shutdown()`。**generator 终值**：driver 取「最后一个**非** `{progress}` yield」**或** generator 的 `return` 值（driver 显式捕获 `StopIteration.value`，故 `yield 终值` 与 `return 终值` 两种写法都生效、裸 return 不被吞）。要点：
- **mutex 串行**：单 stdio 管道，并发调用方逐个过——故每次调用都用**墙钟封顶**（method 自带 `timeout`(ms) 若有、否则全局 `limits.Timeout.HandlerCallSec` 默认 300s 兜底，与 fn/agent/mcp 同款；`PATCH /limits` 可调、校验 >0），否则一个卡死/失控的 method 会无限期堵死整条管道。
- **`Timeout`（MethodSpec，ms）**：app 层 `Call` 先解析 method spec——找不到 method 即报 `HANDLER_METHOD_NOT_FOUND`（不进 RPC）；`Timeout>0` 给本次调用加 ctx deadline。
- **crashed 语义（重要）**：任何读写失败/EOF/协议错乱/**ctx 取消**都把客户端标 `crashed`——包括取消：取消等待意味着回复还在路上，下一个调用会读到错位的迟到回复，**管道已脏**，唯一正确动作是废弃实例（下次 Get 自动重生）。这不是 bug，是协议正确性。
- driver 拒调 `_` 前缀方法（私有）。
- 错误出口：ctx 超时 → `HANDLER_RPC_TIMEOUT`(504)；崩溃 → `HANDLER_CRASHED`(502)；**method 内的 Python 异常原样冒泡**（`HANDLER_CLIENT_CALL_FAILED` + traceback——给 LLM 自纠，刻意不翻译）。**traceback 放进错误 Details**（`{error, traceback}`，非 `fmt.Errorf` 包裹）——因 LLM 错误面 `errorspkg.Surface` 渲 Message+Details 但**剥 fmt 包裹链**（F89/F104/F122 防 Go 路径泄露），traceback 藏包裹里就会在每条 agent/flowrun 路径被剥成不透明 "call failed"（`callFailedErr`，F-handler-call-opaque）。**坏 `__init__` spawn 失败同理**：`spawnInstance` 用 `errorspkg.Wrap`（非 `%w: %v`，后者把内层 `*Error` 拍平出链、Details 全丢）把 init 错误的 traceback Details **抬到** `HANDLER_INSTANCE_SPAWN_FAILED` 上、并 WithCause 保审计链（F116 记 failed call 行 + F131 surface 双补——`Wrap` 是地基助手，凡用 sentinel 包底层结构化错误皆用它，不再 `%w: %v` 拍平）。

### config 生命周期（`config.go`）

`UpdateConfig` = JSON Merge Patch（null 删 key）→ 整 blob 重加密回写 → **Restart 实例重跑 `__init__`**（"改 config → 重启"是核心心智）。`ClearConfig` = 清空 + 停实例。读侧：`ComputeConfigState`（unconfigured/partially_configured/ready + missing 列表，挂单读）、`MaskedConfig`（sensitive → `********`）。

### 调用与记账（`Call`）

resolve handler → 解析 method spec（校验 + 墙钟 deadline：method 自带 `timeout`(ms) 或全局 `HandlerCallSec` 默认兜底，使每次调用有界）→ `manager.Get`（懒 spawn；**失败也记账**——Get 出错=坏 `__init__`/config 缺/env 未就绪/sandbox 挂，记成 `status=failed` 的 Call 行使其现身调用历史 + `failedCount` + `:triage`，`recordCall` 容忍 nil 实例、instanceId 留空，否则该次失败无迹可查）→ **一律 `StreamCall`**（非流式 method 无 yield 自然退化为普通返回）：yield 三写到 entities run 终端 + 调用方 progress sink + 限长 logtail；调用窗口同时在实例 stderr 扇出上挂 sink（print()/日志 → chat progress + run 终端 + logtail；**有注入 secret 时该 sink 经 `scrubbingWriter` 源头掩码**——print 携注入 secret 绝不明文流上 messages/entities SSE 或持久 progress block，F108 补 F82 只盖 recordCall 审计副本的漏；**窗口归属**：同实例并发调用各收各窗口的行、明示可能串扰；收尾留 30ms stderr 宽限——stdout/stderr 两管道乱序，先于 return 写出的 print 可能后到）→ `recordCall`（Detached ctx，best-effort；溯源 5 列从 ctx 读，同 function；**持久化的 `errorMessage` 用 `errorspkg.Surface` 清洗**（F159）——与实时 LLM 错误面一致、渲 Message+Details（含 traceback）但剥 Go 路径面包屑（"handler.Client:"/"context deadline exceeded"），否则 get_handler_call/REST/LLM 读回的耐久记录会漏内部面包屑；**记账前把本实例 sensitive init-arg 的解密值从 error/logs/output 掩成 `********`**——平台抹自己注入密钥的防御纵深（用户代码若把它泄进 traceback/print；只能抹平台已知的注入密钥、非任意用户密钥；config-at-rest 加密 + 读时掩码另算，F82）；`logs` 随行落盘，List 置空、单条 Get 携带）。**错误面也掩、非只审计副本**（F164）：旧 recordCall 只掩持久副本的 errMsg，故返给调用方（`call_handler`→LLM/用户）的**实时**错误仍明文漏密；且 spawn 失败路径以 `inst==nil` 记账、连那道掩码都被绕过。现 `scrubErr`（掩结构化错误 Details 里抬上来的 traceback + cause、保留外层 Code 供 errors.Is）在**构造处**施加——`spawn.go` init 失败 `Wrap` 后即掩（覆盖 spawn 路径 inst==nil）、`call.go` method 路 `mapCallErr` 后即掩——使审计记录与实时返回错误两面皆净。TriggeredBy 空时按 ctx 推（subagent→agent，否则 chat）。

## 5. 关键设计决策

- **单例共享、非池化**：状态留存是 handler 的存在意义，每调用方一份副本就不是共享状态了。
- **create 不 spawn**：刚构建的 handler 大概率缺 config——spawn 推迟到 config 配齐（UpdateConfig 的 restart）/Boot/首调。
- **config 整 blob 加密**而非逐字段：简化密钥管理；掩码在读侧按 schema 的 Sensitive 标志做。
- **Edit/Revert 必重启实例**（**纯 meta 编辑除外**）：常驻进程不会自己换代码。例外——`Edit` 的 ops **全是 `set_meta`** 时，改的是 Handler 行的 name/description/tags（非版本化类快照），常驻实例跑的东西没变，故**只更行、不铸版本、不重启**——否则一次改名就无谓抹掉有状态实例的内存态（agent 又无别的改名路径）。真代码/schema op 仍重启。**空 ops 的 Edit** = 重建 env + 重启实例（**抹内存态**）、不应用 op、不铸版本、返当前 active 版本——故 `opsApplied:0` + 版本不变读着像 no-op，edit_handler 工具结果遂带 **`restarted:true` + restartNote**（F140）使态丢失可见；只想重置实例用 `restart_handler`。
- **`Client` 只有 StreamCall 一个调用动词**（统一入口，progress 回调 nil 即非流式）。

## 6. 契约（引用）

端点 → [api.md](../api.md)#handler（注意 config 三端点 + `:restart`）· 表 → [database.md](../database.md)#handler · 码 → [error-codes.md](../error-codes.md)（domain `HANDLER_*` 18 + infra `HANDLER_CLIENT_*` 5——RPC 客户端原语独立命名空间 + 工具校验 5；含 `HANDLER_CALL_INVALID_STATUS`——`search_handler_calls` 的 status 越出 `{ok,failed,cancelled,timeout}` 即 422、非静默空页，F168-M2）· 事件 → [events.md](../events.md)（10 个通知——实体里最多，含 restarted/config_updated/config_cleared/crashed）。LLM 工具 12 个（含 **`update_handler_meta`**——只改 row 的 name/desc/tags、**不重启**，使有状态实例改名不丢内存态，与 §5 的纯 meta-edit 短路互补、给 agent 一个可发现的无重启改名入口）。

## 7. 跨域集成

- **调用方**：chat loop（`call_handler`，yield 流成 progress 块）/ HTTP `:call`（manual）/ workflow 调度器（`HandlerCaller` 窄接口，`hd_<id>.method` ref 拆分派发）/ sensor / **agent 挂载**（`hd_<id>.<method>` → 合成 `<name>__<method>` 专属工具）。
- **catalog 是容器形态**：item 带 `Members`（active 版本的方法名列表）——LLM 看到 handler 有哪些方法、再 get_handler 看签名、call_handler 调（对齐 mcp 的形态）。
- **@ 提及**：快照 description + **组装后的完整类代码**。
- **依赖端口**：sandbox（SpawnLongLived/Destroy/DestroyEnv）· envfix · crypto Encryptor · ClientFactory（测试可换 RPC 客户端）· relation/notification（nil 容忍）。版本模型/trim 与 function 同构（[function.md](function.md)#2）：edit 越过 cap 50 时 `TrimOldestVersions` 返回被删版本 EnvID，`reclaimTrimmedEnvs` 经 `DestroyEnv` 回收其孤儿 venv，否则泄漏到盘上直到手动 `sandbox:gc`。
