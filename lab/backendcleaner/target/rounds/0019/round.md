# Round 0019 — apikey（波次 1 · M1.2）

类型 / 目标：apikey 垂直切片重写 + **大幅职责收窄**为「加密保险箱 + 哑探针 + 按 id 发钥匙」。**第一个真正吃 orm 自动 workspace 隔离的业务表。** 设计经多轮讨论敲定，详 `contracts/apikey.md`。

依赖扫描：
- **上游**：crypto(M0.3) / orm(隔离 + ErrConflict) / idgen / pagination(ParsePage) / errorsdomain / handler 地基(M1.1) 全就绪。
- **复用盘点（#8 纪律）**：满屏手写 `RequireUserID + Where` → **orm 自动隔离全删**；`isDisplayNameConflict` 手搓字符串 → **orm ErrConflict**；手写 cursor 分页 → **orm Page**；加密 → crypto；id → idgen。**tester 不复用 infra/llm**（探连通 ≠ 调 generate + search 不在 llm）。
- **考古**：apikey 被 28 模块依赖（凭证中枢）；旧版把「选 key」「测连通」「解析模型」三件事混在一起。

关键设计决策：
- **选 key 下放**：删 `GetByProvider` 启发式 + `IsDefault`/`ClearDefaultForCategory`/`DefaultProvider`/`DefaultSearchProvider`/`SearchProviderPriority` → LLM 选择在 model（api_key_id 显式），搜索选择 → 未来搜索配置模块（**防乱烧钱**）。
- **哑探针**：tester 保留各家探测方式（端点+认证），**砍解析器**（parseOpenAIModels/parseModelsByName），存 `test_response` 原始返回；连通 = HTTP 200。
- **解析下放**：`models_found`（解析后）→ `test_response`（原始）；model 模块靠 `ProbeReader.ListProbed` 取档案、实时解析 + 静态目录兜底（Claude 无 list-models 端点 → 静态目录是其**唯一**来源；静态目录应**可更新推送**）。
- `KeyProvider` 收窄 2 法、全按 id（`ResolveCredentialsByID` + `MarkInvalidByID`）。

删 / 移交：
- **删**：上述选择系 + `ResolveCredentials(provider)` + `ErrNotFoundForProvider` + 解析器。
- **移交 model(M1.3)**：`modelcatalog`（去 pkg + 补静态枚举 + 可更新推送）/ `capabilities.go` / capabilities handler / 模型解析器。
- **移交搜索配置模块**：搜索选 key。
- `RefScanner` DIP 端口留（slice + AddRefScanner），scanner 实现注入留下游 + M7。

新实现：
- **domain**：实体去 GORM（`workspace_id,ws` + `test_response` 替 models_found，去 is_default）+ `Credentials`/`ProbedKey` + 7 sentinel(S20) + `Repository`(6 法) + `KeyProvider`(2 法) + `ProbeReader`。
- **store**：orm 自动隔离（删手写）+ `ErrConflict→ErrDisplayNameConflict` + `Page` + `Schema`（partial unique index）。
- **app**：Service(CRUD + 加密 + 按 id 发 / 标失效 + RefScanner slice) + tester 哑探针 + providers 注册表(收窄，留连接/探测所需)。
- **handler**：CRUD + `:test` 瘦身（去 modelsFound）+ `GET /providers`。

新测试：store 8（**workspace 隔离首验**：跨 ws miss / 同名不同 ws ok；ErrDisplayNameConflict；test_response 存储；ListProbed；软删）· app 10（fake repo/tester/encryptor：加密 / 校验 / key 旋转重置探测 / RefScanner / 哑探针写回 / 按 id 发与标失效）。

验证：`gofmt`/`go build ./...`/`go vet`/`go test ./... -race` 全绿。

是否更干净：apikey 从「选 + 测 + 解析」三合一收窄成「钥匙本身」单一职责；store 删满屏手写隔离；解析 / 选择 / 模型理解全下放。✅

契约（→ contract-changes #4）：端点不变（`/api-keys` + `/providers` 文件源改 apikey.go）；字段 `user_id→workspace_id`、`models_found→test_response`、删 `is_default`；error code 统一 `API_KEY_*` 前缀（删 ErrNotFoundForProvider）；`:test` 返回去 `modelsFound`。

遗留 / 下一步：**M1.3 model**（接 modelcatalog + capabilities + 模型解析 + RefScanner 注入；静态目录可更新推送）。
