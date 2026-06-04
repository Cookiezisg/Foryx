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
| 0013 | 2026-06-03 | 波次0 · M0.4 | SSE 三流统一协议 domain（改名 + 流式树重构） | ✅ 单一 domain/stream：信封+四动词Frame+**通用 Node{Type,Content}**+Bridge/ListReader；id 升信封层、frame 可丢性分级、close 带快照；**node 词表下放业务、砍三流 domain 包**；5 源 3 测试绿 |
| 0014 | 2026-06-04 | 波次0 · M0.5 | infra/stream 单一 Bus（三流底座） | ✅ per-workspace seq + frame 分级 buffer（durable 入环/ephemeral seq0 不入不卡）+ replay/ErrSeqTooOld + List；旧三抄 Bridge 收敛成 1 份、实例化三次=三流；D2 全量推；3 源 3 测试 -race 绿；infra/chat 移交 M5.2 |
| 0015 | 2026-06-04 | 波次0 · M0.6 | infra/llm 核心框架 + openai | ✅ Provider 接口+providerClient 铁律+共享传输+类型+factory+mock+openai(完整自包含)；error 内聚 domain/errors(+S20 守则)；**每家 provider 完整自含 wire、不共享基座**；删死代码、strip 历史；8 源 6 测试 -race 绿；trace 推迟；其余 10 provider=R0016 |
| 0016 | 2026-06-04 | 波次0 · M0.6 | infra/llm 其余 10 provider（各自完整自包含） | ✅ anthropic+gemini 原生方言 + deepseek 模板 + 7 家 OpenAI-compat（**workflow 并行 7 agent ~424k tok**）；每家自包含 wire、error sentinel、去 modelcatalog(→Request.MaxTokens)/slog、strip 历史；11 家 -race+合规 grep 全绿。**M0.6 完成** |
| 0017 | 2026-06-04 | 波次0 · M0.7 | transport 框架（response/middleware/router） | ✅ **errmap 293行27import → statusForKind ~50行1import**(零业务 domain)；envelope N1 + SSE marshal(M0.4 推迟，frame/node 判别注入) + pagination Parse；auth **user→workspace 改名落地**(本地 WorkspaceResolver 接口)；6 middleware + router chain；13 源 6 测试绿。完整 New→M7。**波次 0 收官** |
| 0018 | 2026-06-04 | 波次1 · M1.1 | workspace（原 user 正名）+ orm ErrConflict + handler 地基 | ✅ user→workspace 全量正名垂直切片(domain/store/app/handler 7 源)；**多 workspace 数据隔离 + 资源不分桶**定型、Name 自由名 UNIQUE(去 slug/GetByUsername/EnsureExists)；**orm 补 ErrConflict**(对称收口 UNIQUE 翻译，store 不碰 SQLite 字符串)；handler 地基首建(registrar/util/decode)；`Validate` 实现 WorkspaceResolver 端口；去 GORM+S20；store 8+app 10+orm 2 测试 -race 绿；契约 4 件同步 |
| 0019 | 2026-06-04 | 波次1 · M1.2 | apikey（收窄 + 首个 orm 隔离表）| ✅ 收窄为「加密保险箱 + 哑探针 + 按 id 发钥匙」：**选 key 下放**(model/搜索配置，防乱烧钱)、**哑探针**(只判 200 + 存 test_response 原始，砍解析器)、**解析下放**(models_found→test_response，model 靠 ProbeReader + 静态目录兜底/可推送)；KeyProvider 收窄 2 法全按 id；**orm 自动隔离首验**；去 IsDefault/GetByProvider/ErrNotFoundForProvider；modelcatalog/capabilities 移交 M1.3；store 8+app 10 测试 -race 绿；契约 4 件同步 |
| 0020 | 2026-06-04 | 波次1 · M1.3 | model 重写(聚合薄层) + infra/llm 11 家旋钮重构 + apikey 探针 + workspace 默认 | ✅ 删 ThinkingSpec 中立抽象、各家原生 Options 自包含、模型知识下沉 provider(DescribeModels)、model 退化无 store、默认搬 workspace 3 列、Resolve 收口、override 改运行时优雅报错；10 家官方调研；修 anthropic budget_tokens→400 bug；gofmt/build/vet/test -race 全绿 |
| 0021 | 2026-06-05 | 波次1 · M1.4 | relation（横切关系图）+ KindForID 收编 + wikilink doc-fix | ✅ 边类型收成 4 动词(create/edit/equip/link，删 from/to 冗余、CHECK 恒 4 值)、8 节点、KindForID 8 条(补 agent + 定 sk_/mcp_ 规矩)、显示名读时内存 hydrate(不落库/改名即新/无 reader port)、孤立节点不显示、override 式弱引用无删除保护；domain+store+app+handler+测试；契约 4 件同步(relation.md 整篇重写删 ErrInUse/runs_in/mentions)；gofmt/build/vet/test -race 全绿 |
