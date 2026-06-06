---
# Round 0034 — 搜索配置：建 domain/websearch + workspace 加 default_search_key_id（web 前置）

类型 / 目标:M2.3#3 web 的**前置**——补上 M1.2 主动推迟的"搜索配置"。建独立 `domain/websearch` 包（用户要求,对齐 domain/model 便于管理）+ 给 workspace 加"默认搜索 key"列（选 key 显式化、防乱烧钱）。

## 核心方针(一句话)
**搜索配置 = workspace 选定一把显式搜索 key(单选、provider 由 key 隐含、防乱烧钱);domain/websearch 独立薄层(provider 词表 + SearchKeyPicker 端口,无 store)、存储借 workspace——完全对齐 model 的 default-models 套路。**

## 背景(为什么现在做)
- M2.3#3 web 考古发现:旧 WebSearch 的 BYOK 靠旧 apikey 的"按 provider 找搜索 key"(`SearchProviderPriority`/`DefaultSearchProvider`/`ResolveCredentials(provider)`),而 **M1.2 收窄 apikey 全砍了**(只剩 `ResolveCredentialsByID`),STATE 原话"搜索→未来搜索配置"。
- 研究确认:**apikey 这层已就绪**——`app/apikey/providers.go` 的 brave/serper/tavily/bocha 4 个搜索 provider 都在白名单(DefaultBaseURL+TestMethodSearchPing+CategorySearch),domain 注释多处预留"model / search config"。唯一缺口 = workspace 的"默认搜索 key 选择"。
- 用户拍板:对齐 model,key 选择跟着 workspace 表(加列)+ **单独建包**便于管理。

## 关键决策(用户拍板)
1. **建独立 `domain/websearch` 包**(对齐 `domain/model`):Provider 常量(brave/serper/tavily/bocha)+ IsProvider/Providers + `SearchKeyPicker` 接口 + **无 store**(同 model 薄层)。命名 `websearch` 避开 `tool/search`(文件搜索)。
2. **存储借 workspace**(完全同 model 的 default-models):workspace 加 `DefaultSearchKeyID string` 列,`SetDefaultSearch` 就在 `workspace.Service`(正如 `SetDefault` model 也在 workspace.Service,不在 app/model)。**不建 app/websearch**——model 的 app/model 是 capability 聚合,搜索无此需求(一个 key=一个搜索服务,前端直接用 apikey list)。
3. **单选显式 string,不是优先级列表/SearchRef struct**:搜索只需一把 key、provider 由 key 隐含(`Credentials.Provider`)——替代旧 `SearchProviderPriority` 自动遍历(乱烧钱来源)。反预留:一个 id 不值得包 struct。
4. **不校验 provider/category**:镜像 model `SetDefault` 的运行时优雅风格 + 反校验剧场——workspace 零新依赖(仍只 `repo`);WebSearch 运行时拒非搜索 key,UI 只让选 search 类 key。

## 新实现
- `domain/websearch/websearch.go`(★新独立包):Provider 4 常量 + IsProvider + Providers + SearchKeyPicker 接口。无 store/错误/HTTP。
- `domain/workspace/workspace.go`:+ `DefaultSearchKeyID string` 字段(db tag,与 default models 并列)。
- `infra/store/workspace/workspace.go`:DDL + `default_search_key_id TEXT NOT NULL DEFAULT ''`(orm 自动读写)。
- `app/workspace/workspace.go`:+ `DefaultSearchKeyID(ctx)(string,bool)`(实现 SearchKeyPicker)+ `SetDefaultSearch(ctx,id,keyID)`(空=清除);`var _ websearchdomain.SearchKeyPicker = (*Service)(nil)`。
- `transport/.../handlers/workspaces.go`:+ `PUT/DELETE /workspaces/{id}/default-search`(对齐 SetDefaultModel)。

## 测试(全离线)
- `domain/websearch` 2:IsProvider(4 真/others 假)+ Providers(4 个有序)。
- `app/workspace` 4:SetDefaultSearch+读 / 未配 ok=false / 清除 / 无 ws ctx ok=false。
- `store/workspace` 1:default_search_key_id 往返。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet ./...` 0 · `go test -race -count=1`(websearch 1.3s / workspace app 1.7s / store 1.8s)。

## 契约
- `domains/websearch.md` **新建**(DOC-302,对齐 domains/model.md 薄层结构)。
- `database.md`:workspaces 表 + `default_search_key_id` 列。
- `api.md`:+ `PUT/DELETE /workspaces/{id}/default-search` 路由 + 说明。
- `domains/workspace.md`:+ DefaultSearchKeyID 偏好 + §4.4 default-search 端点。
- `contract-changes.md #14`。

## 跨波次接线
- **WebSearch 消费**(R0035 web):`searchKeyPicker.DefaultSearchKeyID(ctx)` → `keys.ResolveCredentialsByID(id)` → `websearch.IsProvider(creds.Provider)` ? switch → searchBrave/Serper/Tavily/Bocha。
- **前端 default-search 选择器**(覆盖阶段):从 `category=search` 的 apikey 里选一把;调 PUT/DELETE default-search。
- **MCP tier**(无 BYOK 时):mcp(M3.6)注入 `MCPSearchRouter`。

## 进度
波次 2 M2.3:#1 filesystem ✅ → #2 search ✅ → **搜索配置 ✅(R0034,web 前置)** → #3 web(R0035,下一)→ #4 toolset。
