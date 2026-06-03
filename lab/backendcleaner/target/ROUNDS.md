# backendcleaner ROUNDS

| Round | 日期 | 阶段 | 目标 | 结果 |
|---|---|---|---|---|
| 0001 | 2026-06-03 | 波次0 · M0.1 | pkg reqctx/idgen/pagination 重写 | ✅ stdlib-only，测试绿（含 R0001.1：reqctx 按 concern 拆 workspace.go/reqctx.go） |
| 0002 | 2026-06-03 | 波次0 · M0.1 | tokencount 迁移 | ✅ 原样保留（干净叶子），测试绿 |
| 0003 | 2026-06-03 | 波次0 · M0.1 | pathguard 迁移 + #7 清理 | ✅ 逻辑不动，删 V1.2 叙述/死变量/过时注释，测试绿 |
| 0004 | 2026-06-03 | 波次0 · M0.1 | userpath 去留判定 | ⏭️ 判定删除（多用户分桶+历史迁移，新架构不存在）；登记 M1.1/M7.1 |
| 0005 | 2026-06-03 | 波次0 · M0.1 | wikilink 剥成纯抽取 | ✅ 去 Kind/去 idgen 依赖，Kind 映射归 relation(M1.4)，测试绿 |
| 0006 | 2026-06-03 | 波次0 · M0.1 | jsonrepair 迁移 + 补测试 | ✅ 实现原样（高质量），补 10 unit，gofmt 归一，测试绿 |
| 0007 | 2026-06-03 | 波次0 · M0.1 | limits 迁移(搬+清+补测试) | ✅ 保留全局 getter，清 P0/P3+adhoc 叙述，补 4 测试，绿。**M0.1 完成** |
| 0008 | 2026-06-03 | 波次0 · M0.2 | 自研 pkg/orm（去 GORM） | ✅ 链式/类型安全/自动 workspace+软删+时间戳，9 源+21 测试绿；domain 去 GORM 化成全局方针 |
| 0009 | 2026-06-03 | 波次0 · M0.2 | infra/db 网关 GORM→database/sql | ✅ glebarez/go-sqlite + 单连接 + DDL 迁移机制；schema_extras 删（分散各模块）；orm 补 Exec/Close。**M0.2 完成** |
| 0010 | 2026-06-03 | 波次0 · M0.3 | infra/logger | ✅ zap.go 保留+简化(去 extras)；broadcast.go 删(日志 SSE 违反 E1)；2 测试绿 |
| 0011 | 2026-06-03 | 波次0 · M0.3 | crypto 切片(domain port + infra adapter) | ✅ AES-GCM + 机器指纹，原样保留，13 测试绿；port-adapter 范本。**M0.3 完成** |
| 0012 | 2026-06-03 | 波次0 · M0.4 | domain/errors 结构化强化 | ✅ Error{Kind,Code,Details,cause}+Is by Code；根除 errmap 293 行+27 import 巨耦合；6 测试绿；契约 UNAUTH_NO_WORKSPACE |
| 0013 | 2026-06-03 | 波次0 · M0.4 | SSE 三流统一协议 domain（改名 + 流式树重构） | ✅ 单一 domain/stream：信封+四动词Frame+**通用 Node{Type,Content}**+Bridge/ListReader；id 升信封层、frame 可丢性分级、close 带快照；**node 词表下放业务、砍三流 domain 包**；6 源 3 测试绿 |
