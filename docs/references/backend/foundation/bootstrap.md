---
id: DOC-034
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# bootstrap —— composition root（P8 收口）

## 1. 定位 + 心智模型

**唯一**允许横跨所有 app/infra 包 import 的地方（无人 import 它 → 无依赖环；`cmd/server/main.go` 是薄壳）。`Build` 把 SQLite + 全部 store + infra 单例（加密/LLM factory/三条流 Bus）+ **26 个 app Service**（按依赖 Tier 构造）+ 跨服务适配器 + 工具集 + HTTP router 焊成一个 `*App`。

**关键装配模式**：
- **后注入破环**（SetRelationSyncer / SetInvokeDeps / SetExecutionPorts / SetLifecycleReconciler…）：构造序无环、能力后接。
- **toolsetHolder 破环**：subagent 读工具集要等 Subagent 工具追加、而那又要 subagent Service——holder 懒读破此环。
- **窄接口适配**（dispatch.go 四执行端口 / sensor.go invoker / runnerAdapter / RefResolver / KnowledgeProvider）：调度器与实体互不知具体类型。

## 2. 生命周期

- **settings**：`app/settings` 在装配前读 `<dataDir>/settings.json`（limits 段；缺文件=纯默认、坏文件=boot 失败）装成 `limits.Current()` 活动源；PATCH /limits 持久化 + 热换。
- **日志**：zap 双 sink——stderr 控制台（dev 彩色 / 否则 JSON，级别 dev=DEBUG / 否则 INFO）+ `<dataDir>/logs/forgify.log` 轮转 JSON 文件（10MB×3、28 天、gzip；`infra/logger`）——桌面报障 = 发这一个文件。
- **Serve**：Boot → ListenAndServe → 信号 → **三步优雅关停**（① 先 cancel base 请求 ctx——三条常驻 SSE 永不 idle，不先断它们 http.Shutdown 会干等满 grace 窗 ② http.Shutdown 瞬间排空 ③ App.Shutdown 停后台、最后关 DB checkpoint WAL）。
- **Boot**：sandbox bootstrap（失败=degraded）→ trigger.Start → scheduler.Recover（跨 ws 重走 running run）→ **`forEachWorkspace`**（后台播种铁律，[引擎文档](scheduler-flowrun.md)#5）逐 ws：handler/mcp 预热 + chat.SweepOrphans（崩溃孤儿回合对账）+ workflow.ReattachActive → 启 5s drain ticker（同样逐 ws：DrainFirings + CheckTimeouts）。
- **Shutdown 逆序**：停 ticker → trigger → chat 队列 → mcp/handler 常驻进程 → sandbox 兜杀残留 handle → flush 日志 → 关 DB。每步 best-effort（一个卡死子系统不拖垮其余）。

## 3. 契约（引用）

守护测试 `background_ctx_test.go`（裸 ctx 必败/播种必通）。码 `UNTRIAGEABLE_EXECUTION`（aispawn triage 适配在此实现）。`Config{DataDir("" = 内存 DB 测试), Addr, Fingerprint, Dev}`；Fingerprint 空（服务正常路径）时 newEncryptor 解析真实机器指纹（`MachineFingerprint()`），平台拿不到才回退 `forgify-local:<dataDir>` 种子。
