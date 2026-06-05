# Round 0028 — document（波次 1 · M1.10）Notion 树知识库

类型 / 目标：M1.10 document 重写——去 GORM + workspace 隔离 + Emitter + 砍 attach 子树注入 + 4 适配器对齐前三模块新地基。波次 1 最后一个实质模块（M1.11 todo 待判定）。

## 核心方针（一句话）
**document = Notion 树知识库（树 CRUD + path 级联 + 防环 + 软删）+ wikilink 互链 + 显式挂载注入（无 RAG / 无子树）；第一个接通 catalog/relation/mention 的实体，验证前三模块端口。**

## 关键决策
1. **去 GORM + workspace 隔离**：Document 纯 struct + db tag、`UserID→WorkspaceID`（orm 自动隔离，app 去掉所有 `RequireUserID`/uid 传递）、`DeletedAt`→orm `deleted` tag（业务表**软删**）。
2. **砍 attach 子树注入**（用户拍板）：`AttachedDocument` 去 `IncludeSubtree`，`ResolveAttached` 只取显式那几篇——挂载有界，不"挂一篇拖出一整棵树"炸 context。
3. **4 适配器对齐前三模块新地基**（document 是第一个接通的实体，验证它们的端口）：
   - catalog_source：去 `Granularity`/`InvokeTool`/`Category`（catalog R0022 收窄），对齐 `Item{Source,ID,Name,Description}`
   - relations：`wikilink.Parse`（去 Kind R0005）→ `relation.KindForID` 解析过滤 → `link` 边（4 动词 R0021）
   - mention_resolver：对齐新 `Reference`（微调）
   - Namer：实现 `relation.Namer.NamesByIDs`（读时 hydrate 给 doc 节点贴名）
4. **notification.Emitter**：`document.created/updated/moved/deleted`。
5. **UNIQUE `COALESCE(parent_id,'')`**：SQLite NULL 不参与 UNIQUE，根级同名需 COALESCE 兜住。

## 考古发现（旧实现评估）
- 树算法质量高（path 级联 / 防环 / 重名加后缀 / 软删子树）——本质复杂度，照搬。
- 文档腐烂：domains/document.md 标题写"RAG 引擎"但实际**无 RAG**；`includeSubtree` 已设计但用户判砍。

## 新实现
- **domain**：Document + AttachedDocument(去 IncludeSubtree) + 6 错误 + Repository(去 userID 参数)。
- **store**：orm 树 CRUD + BFS collectDescendants + IsAncestor 防环 + 软删子树 + COALESCE UNIQUE。
- **app**：树 CRUD 照搬（去 uid，orm 隔离）+ Emitter + ResolveAttached 砍子树 + RenderAttachedAsXML + 4 适配器。
- **handler**：树 CRUD + move（`:iterate` 留波次 6 askai）。

## 测试（全离线）
store 8（树 CRUD / 重名 COALESCE / BFS / 防环 / 软删+名复用 / MaxPos / ws 隔离）；app 9（自动加后缀 / path 级联 / 防环 / **attach 无子树** / validate / catalog / mention / wikilink→link / Namer）。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet ./...` 0 · `go test ./...document... -race` 全 ok（store 2.2s / app 2.5s）。

## 契约
domains/document.md 整篇重写（去 RAG 标题、去 includeSubtree、4 适配器）；database.md 4.1 去 GORM+workspace+COALESCE 索引；api.md `:iterate` 备注波次 6；events.md document 4 动作事件；error-codes 6 码已对（`INVALID_PARENT` 改 422 对齐）；contract-changes #8。

## 遗留 / 下一步
- **M1.11 todo（待判定）** → 波次 1 收尾。
- 4 适配器注入（CatalogSource / MentionResolver / RelationSyncer / Namer）→ M7。
- attach 消费者（chat/scheduler `ResolveAttached`+`RenderXML`）→ 波次 4/5。
- `:iterate`（askai）→ 波次 6；`app/tool/document`（LLM 工具）→ 波次 3。
