# Round 0021 — relation（波次 1 · M1.4）+ KindForID 收编 + wikilink doc-fix

类型 / 目标：M1.4 relation 模块新建——跨实体关系图（实体血缘网）。考古旧实现 + 多轮讨论敲定全新设计。

## 核心方针（一句话）
**边类型收成 4 个动词、显示名读时内存现查（不落库）、KindForID 收编自 idgen 并为 skill/mcp 定下前缀规矩。**

## 关键设计决策（经讨论拍板）
1. **边类型 = 4 动词** `create/edit/equip/link`：旧的 14 种 `{from}_{verb}_{to}` 把两端编码进 kind 是冗余（from_kind/to_kind 列已存两端）。kind 只需动词 → DB CHECK 恒 4 值、加实体不改枚举。中央枚举保留（全局拓扑契约 + DB CHECK，非各家碎旋钮，与 M1.3 删中立抽象不矛盾）。
2. **显示名读时内存 hydrate，不落库**：曾考虑「胖边存 name」（干掉 reader port）但改名要双写刷新；再考虑「读时 orm join」但多态 6 表给轻量 orm 动大手术。最终落「读时 batch-load」——边只存 id，读返回前按 kind 批量 `Namer.NamesByIDs` 拼瞬态 `RelationView`。**改名即新、零 stale、零写时维护、orm 不动**。代价：取名依赖以轻量 `Namer` 回归（比旧 8 个 `ListAllMeta` 轻）。
3. **孤立节点不显示**：relgraph 节点从边端点去重而来；无边实体不入图（图展示关系非清单）。对话「只显示被连到的」旧特例随之自动消失。
4. **KindForID 收编 + skill/mcp 前缀规矩**：从 idgen 搬入 prefix→EntityKind（旧表漏 agent，本轮补）。**为 skill/mcp 定前缀 `sk_`/`mcp_`**（避开已占用的执行流水 `ske_`/`mcl_`/`mch_`）——归一 id 体系是波次 3，但前缀此刻定死、`KindForID` 已识别、wikilink 可抓，故 document 现在就能 `[[tag]]` 它们，未来零改动接入。
5. **无删除时引用保护**：删被引用实体不阻断（对齐 M1.3 model override 弱引用方针）；sync best-effort（失败只 log）。
6. **写侧自包含全立、消费侧留后**：domain/store/app(diffSync 写 + BFS + hydrate)/handler 本轮全建可测；各实体 sync 胶水 + Namer 实现注入留波次 2/3/5（同 M1.1 boot 遗留挂 M7）。

## 考古发现（旧实现 / 旧文档的历史错误）
- 旧契约 `relation.md` 烂：6 种旧 kind 命名、虚构的 `ErrInUse` 阻断删除、不存在的 `runs_in`/`mentions`——整篇重写。
- 旧 `idgen.KindByPrefix` 只 5 条（漏 agent，agent 是后加的 quadrinity）——补成 6 + 定 sk_/mcp_。
- 旧 error-codes：relation 错误名/wire code 全旧（`INVALID_ENTITY_REF` 等）、`ErrSelfLoop` 挂 500 未映射——重写 5 行 + 修成 400。

## 新实现
- **domain/relation**：`entitykind.go`(8 EntityKind + 4 Kind + prefixKind 8 条 + KindForID) + `relation.go`(Relation 实体 db tag/无 name 列/无软删 + RelationView/Node/Snapshot + SyncEdge/Filter + Service/Repository 接口 + 5 错误 S20)。
- **infra/store/relation**：orm Repository + relations DDL（CHECK 4 动词、idx_rel_dedup 幂等、from/to 索引）；InsertBatch 循环 Create 忽略 ErrConflict 幂等（orm 无批量/dedup-upsert，不动地基）。
- **app/relation**：Service(SyncOutgoing/Incoming diffSync 写 + PurgeEntity + List/Neighborhood BFS/GetRelgraph 读) + hydrateNames 内存 batch + Namer 接口 + relgraph 节点去重。
- **handler**：3 只读端点 GET /relations(filter+分页) / /neighborhood / /relgraph。
- **wikilink doc-fix**：`SkillMcpNotMatched` 注释基于旧 name-keyed 假设 → 改「名字形态不被抓」+ 新增 `SkillMcpIDsMatched`(sk_/mcp_ ID 可抓)。

## 测试
domain：KindForID 全前缀/未知/执行流水前缀/名字形态 + IsValidKind(4 动词，拒旧词表) + IsValidEntityKind(8 种)。app：diffSync 增删改幂等 + SyncIncoming at-most-one+方向 + 自环/非法 kind 拒绝 + Neighborhood BFS depth + hydrate 填名+回退 id + relgraph 去重 + purge 双向 + incomplete filter。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet ./...` 0 · `go test ./... -race` 全 ok。

## 契约（无对外破坏性变更，端点不变）
domains/relation.md 整篇重写；database.md §4.2 重写 + 前缀全集 +sk_/mcp_；error-codes.md relation 5 行重写(REL_* + ErrSelfLoop 修 400)；api.md §5.3 已一致（3 端点不变）。

## 工程方式
单线程顺序：考古(并行读旧 domain/app/store/doc + 契约) → 多轮设计讨论(边类型粒度 / 胖边 vs 读时取名 / orm join vs batch-load / skill-mcp 归一) → domain→store→app→handler→测试→文档逐层编译验证。

## 是否更干净
旧：14 种冗余 kind、8 个 reader port 全量 ListAllMeta、对话特例分支、idgen 漏 agent、文档虚构 ErrInUse、ErrSelfLoop 未映射 500。
新：4 动词零冗余、读时轻量 batch hydrate、孤立/对话自然处理、KindForID 8 条补全 + skill/mcp 规矩定死、文档据实重写、错误全 400 映射。✅

## 遗留 / 下一步
- **M1.5 catalog**（波次 1 续）。
- skill/mcp 归一 id 体系（建表 + 生成器 + Namer）→ 波次 3；各实体 sync 胶水 + Namer 注入 → 波次 2/3/5；handler 路由装配 → M7。（见 deps-todo）
