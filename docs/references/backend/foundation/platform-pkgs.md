---
id: DOC-033
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# 平台小件 —— cel · crypto · db · transport · pkg 工具箱

> P7 收尾合篇。orm / reqctx / errors / loop / stream / llm / sandbox 各有专篇（foundation/）。

## pkg/cel

裸 CEL 编译求值，根变量固定三个：`payload`/`ctx`（trigger sensor）、`input`（control when/emit + approval 模板）。**env 无 now()/墙钟**——guard 重放确定（durable 引擎的前提之一）。`ScopedEnv`（scheduler 用）以图 node id 为根。模板模式 `{{ CEL }}`（approval 渲染）。

## infra/crypto

AES-GCM 整密文加解密（apikey 密文 / handler config / mcp config_enc 共用）+ 机器指纹派生密钥种子（`CRYPTO_*` 2 码）。本地单用户的"防瞄一眼"级别，非威胁模型级。

## infra/db

无业务知识的 SQLite 网关：`Open`（glebarez 纯 Go 驱动、WAL）+ `Migrate`（各 store 导出幂等 DDL、cmd/server 汇总、单事务按序应用——无 ALTER 机制，未上线期改 DDL = 本地库重建）。

## transport

`router.Chain` 中间件栈（workspace identify/require → locale → CORS）+ 26 个资源 handler 注册到一个 mux + `response`（N1 Envelope + `errmap.statusForKind` 唯一 Kind→HTTP 表 + FromDomainError）。auth：`RequireWorkspace` 在边界以 401 `UNAUTH_NO_WORKSPACE` 拒（与内部 500 `MISSING_WORKSPACE_ID` 之分见 [reqctx.md](reqctx.md)#4）。

## pkg 工具箱（一行职责）

`agentstate`（run 内跨工具共享状态：discovered 工具/active skill/读写不变式）· `idgen`（`<prefix>_<16hex>`，S15）· `jsonrepair`（LLM 脏 JSON 尽力修复，strict 解析前置）· `limits`（用户可调上限单源——schema 即现实投影：每字段必有消费方；`app/settings` 启动读 `<dataDir>/settings.json` 装源、PATCH /limits 热换；默认值钉死接线前各模块常量）· `logtail`（头+尾限长日志收集器，io.Writer；fn/hd/mcp 执行链落 `logs` 列的共用预算 64KiB）· `pagination`（keyset 游标编解码）· `pathguard`（文件系统工具的 deny-list 安全层）· `schema`（Field 粗类型模型 + JSON Schema 双向转换）· `tokencount`（启发式 token 估算+可校准）· `wikilink`（`[[id]]` 引用抽取）· `fspath`（绝对路径/~ 展开守卫）。
