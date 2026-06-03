# backendcleaner ROUNDS

| Round | 日期 | 阶段 | 目标 | 结果 |
|---|---|---|---|---|
| 0001 | 2026-06-03 | 波次0 · M0.1 | pkg reqctx/idgen/pagination 重写 | ✅ stdlib-only，测试绿（含 R0001.1：reqctx 按 concern 拆 workspace.go/reqctx.go） |
| 0002 | 2026-06-03 | 波次0 · M0.1 | tokencount 迁移 | ✅ 原样保留（干净叶子），测试绿 |
| 0003 | 2026-06-03 | 波次0 · M0.1 | pathguard 迁移 + #7 清理 | ✅ 逻辑不动，删 V1.2 叙述/死变量/过时注释，测试绿 |
| 0004 | 2026-06-03 | 波次0 · M0.1 | userpath 去留判定 | ⏭️ 判定删除（多用户分桶+历史迁移，新架构不存在）；登记 M1.1/M7.1 |
| 0005 | 2026-06-03 | 波次0 · M0.1 | wikilink 剥成纯抽取 | ✅ 去 Kind/去 idgen 依赖，Kind 映射归 relation(M1.4)，测试绿 |
| 0006 | 2026-06-03 | 波次0 · M0.1 | jsonrepair 迁移 + 补测试 | ✅ 实现原样（高质量），补 10 unit，gofmt 归一，测试绿 |
