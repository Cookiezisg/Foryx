---
id: DOC-126
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-04
review-due: 2026-09-01
audience: [human, ai]
---
# Workspace Domain — 本地隔离根、数据边界与自愈准入全书

> **宪法级定义**：Workspace 是 Forgify 的**物理数据隔离单元**。本地单机可建多个 workspace（如「个人」「客户 A」），每个 workspace 拥有独立的 API Key、对话、Agent、Workflow、文档与运行记录——在数据库层通过 `workspace_id` 物理列做到绝对解耦。但应用级资源（MCP 配置 / 技能 / 设置 / 工具发现缓存）**跨 workspace 共享一份磁盘文件、不分桶**。所以 **workspace 是数据边界，不是文件系统边界**。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Workspace` 实体结构（纯 struct + db tag，已去 GORM）
```go
// workspaces 是隔离根——唯一不带 workspace_id 列的业务表。
type Workspace struct {
    ID          string     `db:"id,pk" json:"id"`                            // ws_<16hex>，§S15
    Name        string     `db:"name" json:"name"`                          // 自由展示名，全机唯一
    AvatarColor string     `db:"avatar_color" json:"avatarColor,omitempty"` // 前端色点，纯展示
    Language    string     `db:"language" json:"language"`                  // workspace 级 UI 偏好
    // 按 scenario 的默认模型选择——与 Language 并列的 workspace 级偏好，JSON 存 ModelRef；nil = 该 scenario 未配置。
    DefaultDialogue *modeldomain.ModelRef `db:"default_dialogue,json" json:"defaultDialogue,omitempty"`
    DefaultUtility  *modeldomain.ModelRef `db:"default_utility,json" json:"defaultUtility,omitempty"`
    DefaultAgent    *modeldomain.ModelRef `db:"default_agent,json" json:"defaultAgent,omitempty"`
    DefaultSearchKeyID string `db:"default_search_key_id" json:"defaultSearchKeyId,omitempty"`  // "" = 未配；WebSearch 的唯一显式搜索 key（provider 由 key 隐含）
    LastUsedAt      *time.Time `db:"last_used_at" json:"lastUsedAt,omitempty"`  // 切换时刷，前端 picker 排序
    CreatedAt       time.Time  `db:"created_at,created" json:"createdAt"`
    UpdatedAt       time.Time  `db:"updated_at,updated" json:"updatedAt"`
    DeletedAt       *time.Time `db:"deleted_at,deleted" json:"-"`               // 软删
}
```

- **`DefaultDialogue/Utility/Agent` 是 workspace 级模型偏好**：每个是一个 `modeldomain.ModelRef`（`apiKeyId`+`modelId`+原生 `options`），JSON 序列化进 `TEXT` 列，`nil` = 该 scenario 未配置。模型选择不再有独立表——它就是跟着 workspace 走的偏好（见 §5）。`DefaultFor(scenario)` / `SetDefaultFor(scenario, ref)` 在三列上做 scenario→列的开关。

- **`DefaultSearchKeyID` 是 workspace 级搜索偏好**：WebSearch 用的唯一显式搜索 api-key id（`TEXT NOT NULL DEFAULT ''`，`""` = 未配置），provider 由 key 隐含（`apikey.Credentials.Provider`）。**单选、无优先级列表**——agent 永不挨个试 provider 乱烧钱。`Service` 经 `DefaultSearchKeyID(ctx)` 实现 `websearch.SearchKeyPicker`（同 `ModelPicker` 的 DIP；详见 `domains/websearch.md`）。

- **`Name` 是自由展示名，不是 slug**：允许中文、空格、大小写，`TrimSpace` 后非空、长度 ≤ 64 rune。全机唯一（大小写敏感），由物理 `UNIQUE INDEX idx_workspaces_name ... WHERE deleted_at IS NULL` 保证——partial index 使软删掉的名字可被重用。重名冲突由 `pkg/orm` 把 SQLite UNIQUE 违例翻译成 `ErrConflict`，store 再翻成 `ErrNameConflict`。
- **`Language` 是 workspace 级偏好**：物理 CHECK 约束限制集合 `{'zh-CN','en'}`，默认 `'zh-CN'`。它是 workspace 一组未来偏好里的**第一个**（见 §5）。
- **无 `workspace_id` 列**：它就是隔离根。这让 `pkg/orm` 的自动 workspace 过滤对它天然失效（`meta.ws == nil`），故 `List`/`Get`/`Count` **在尚未选定任何 workspace 时（onboarding）也能查询**。

### 1.2 ID 生成原理 (`pkg/idgen`)
- `idgen.New("ws")` → `ws_<16hex>`，熵源 `crypto/rand` 读 8 字节。
- **鲁棒性**：OS 熵源损坏时 `New` 立即 **panic**——坏随机源会静默产生碰撞 ID，远比响亮崩溃更糟。

---

## 2. 准入与自愈中间件 (Auth Stack)

Forgify 采用 **「无状态声明 + 状态校验」** 模式，后端不持 Session，全靠 Request Header 驱动。注意：这不是密码认证，`workspace id` 只是「当前活跃隔离根」的声明。

### 2.1 流量入口识别 (`IdentifyWorkspace`)
- **Header**：`X-Forgify-Workspace-ID: ws_<16hex>`。客户端从 `localStorage.activeWorkspaceId` 读后填入每个请求。
- **Query Fallback**：SSE / `EventSource` 请求通过 `?workspaceID=ws_xxx` 注入。
- **深度校验**：中间件持有最小端口 `WorkspaceResolver { Validate(ctx, id) error }`（本地接口，零业务 domain 依赖）；workspace `Service.Validate` 实现它（装配时注入）。拿到 id 后调 `Validate` 实查一次——若格式对但库里没这个 workspace（如刚被删），则 **ctx 仍视为空**，消除脏 id。

### 2.2 强制准入 (`RequireWorkspace`)
- **逻辑断路**：ctx 无有效 workspace id → 立即 `401 Unauthorized` + Code `UNAUTH_NO_WORKSPACE`。
- **豁免白名单**：`/api/v1/workspaces`（CRUD，onboarding 前可用）、`/api/v1/health`、`/api/v1/providers`、`/api/v1/scenarios`。

### 2.3 前端自愈环 (Self-Healing Loop)
1. 前端 `apiFetch` 收到 401、捕获 `UNAUTH_NO_WORKSPACE`。
2. 清除 `localStorage.activeWorkspaceId`。
3. App 监控到 id 为空，自动拉起 **Onboarding** 或 **Workspace Picker**。

---

## 3. 隔离模型 (Isolation)

### 3.1 数据隔离（自动，由 `pkg/orm` 兜底）
- **铁律**：除 `workspaces` 表自身外，所有业务表必须持 `workspace_id` 物理列（§D2）。
- **自动注入**：`pkg/orm` 在每次读时拼 `WHERE workspace_id = ctx.workspaceID`、每次写时 stamp `workspace_id`——取代每个 store 手写隔离条件，且**不可能误写进别的 workspace**。系统级跨 workspace 查询须显式 `CrossWorkspace()`。

### 3.2 资源共享（不分桶）—— 关键设计决策
- 应用级资源 `mcp.json` / `skills/` / `settings.json` / `.catalog.json` 在 `~/.forgify/` 下**共享一份，不按 workspace 分桶**。
- **理由**：这些是**应用级配置**（MCP 服务器、技能库、权限规则、工具发现缓存），不是某个 workspace 的私有业务数据。按 workspace 各拷一份只会徒增复杂度、无实际价值。
- 故旧的 per-user 文件根（`~/.forgify/users/<uid>/`）连同其历史迁移逻辑**整体删除**。再次强调：**workspace 是数据边界，文件资源是全局的**。

---

## 4. 生命周期 (Lifecycle)

### 4.1 创建 (`Create`)
- **名字校验**：`TrimSpace` 后非空（`ErrNameRequired`）、长度 ≤ 64 rune（`ErrNameTooLong`）。自由文本，**无 slug 正则、不强制小写**。
- **语言**：空则默认 `zh-CN`；非空则校验在集合内（`ErrLanguageInvalid`）。
- **重名**：依赖 DB `UNIQUE(name)` + orm 冲突翻译 → `ErrNameConflict`，不做应用层预检（避免 TOCTOU）。

### 4.2 激活与切换 (`:activate`)
- `POST /api/v1/workspaces/{id}:activate` → `TouchLastUsed` 刷墙钟，返回最新 `Workspace`。
- 前端切换 workspace 时调用，以此为信号刷新整个 app 的隔离上下文。

### 4.3 设默认模型 (`default-models`)
- `PUT /api/v1/workspaces/{id}/default-models/{scenario}`（`scenario ∈ dialogue/utility/agent`）→ body 是 `ModelRef`（`apiKeyId`+`modelId`+原生 `options`），写进对应的 `default_*` 列，返回更新后的 `Workspace`。
- 校验：scenario 非白名单 → `modeldomain.ErrScenarioInvalid`(`MODEL_SCENARIO_INVALID`)；ModelRef 缺 `apiKeyId`/`modelId` → `modeldomain.ErrRefInvalid`(`MODEL_REF_INVALID`)。
- 前端在 Settings 的模型配置页为三个 scenario 分别选模型，每选一次打一发。

### 4.4 设默认搜索 key (`default-search`)
- `PUT /api/v1/workspaces/{id}/default-search` → body `{apiKeyId}`，写进 `default_search_key_id` 列，返回更新后的 `Workspace`。`DELETE` 同路径清除（写 `""`）。
- **不校验 provider/category**（运行时优雅，对齐 default-models）——WebSearch 调用时拒非搜索 key，前端只让选 `category=search` 的 key。
- **单选显式 = 防乱烧钱**（替代旧 `SearchProviderPriority` 自动遍历 4 个 provider）。详见 `domains/websearch.md`。

### 4.3 终极保护 (The Last Guardian)
- `Service.Delete` 内部先 `Count()`，若 `count <= 1` 拒删（`ErrCannotDeleteLast`）。
- **原理**：隔离根必须存在，防止误删到 0 个 workspace 导致系统失去数据边界。

---

## 5. 偏好归属 (Preference Ownership)

workspace 行承载**跟着 workspace 走的偏好**：当前是 `Language` 与三个 scenario 的默认模型（`default_dialogue/utility/agent`）。划界原则：

- **进 workspace 行**：随 workspace 切换的偏好（language、默认模型选择…）。模型选择**刻意不另立表**——它就是 workspace 的偏好，落列即可。
- **留全局，不进 workspace**：应用 / 机器级配置（`limits` 运营上限、遥测开关、机器指纹…），跨 workspace 共享。

> 模型默认值的形状（`ModelRef`）与分派规则归 **model 域**（见 `domains/model.md`）；workspace 只持有这三列、并实现 `ModelPicker` 把它们喂给 model 的 `Resolve`（见 §6.3）。`settings.json`（`settingsinfra`）文件存储与 workspace 行偏好的边界，留待 **settings 模块那一轮**判定哪些字段归 workspace 行、哪些留文件。

---

## 6. 跨域集成场景 (Principles in Action)

### 6.1 国际化 (i18n)
- `InjectLocale` 中间件读当前 workspace 实体的 `Language`，注入 ctx（`locale` key），后续 LLM Prompt 模块据此动态拼装中英文。

### 6.2 触发器所有权 (Trigger Owner)
- **Cron 场景**：即便当前坐在电脑前的是 workspace A，后台正在跑的 workspace B 的 Cron 任务仍须以 B 的身份执行。
- **实现**：`trigger_schedules` 表固化 `workspace_id`；调度器 Firing 时从配置读取 owner，注入 ctx workspace 模拟上下文。

### 6.3 默认模型分派 (ModelPicker 实现)
- workspace 域**实现 model 域的 `ModelPicker` 端口**（装配时注入 model 的 `Resolve`）：`Pick(ctx, scenario)` 读当前 workspace 的 `DefaultFor(scenario)`——已配则返回该 `ModelRef`，未配则返 `modeldomain.ErrNotConfigured`。
- **职责切分**：workspace 只回答「这个 workspace 在该 scenario 选了谁」（存储侧）；override-then-default 规则、`ModelRef` 形状、能力聚合都归 model 域。这样 model 不依赖 workspace，workspace 也不懂分派规则。

---

## 7. 错误字典 (Clinical Sentinels)

全部经 `errorsdomain.New(kind, code, msg)` 构造（§S20），transport 直接读 Kind/Code，无集中映射表。

| Sentinel | HTTP | Wire Code | 场景及处理建议 |
|---|---|---|---|
| `ErrNotFound` | 404 | `WORKSPACE_NOT_FOUND` | id 错误。前端应清空 `localStorage.activeWorkspaceId` 并重 onboard。 |
| `ErrNameRequired` | 400 | `WORKSPACE_NAME_REQUIRED` | 名字为空白。Onboarding 高亮输入框。 |
| `ErrNameTooLong` | 400 | `WORKSPACE_NAME_TOO_LONG` | 超过 64 字符。前端应做同等长度限制。 |
| `ErrNameConflict` | 409 | `WORKSPACE_NAME_CONFLICT` | 名字重复。Onboarding 阶段高亮红色。 |
| `ErrCannotDeleteLast` | 422 | `CANNOT_DELETE_LAST_WORKSPACE` | 强力阻断。UI 置灰最后一个 workspace 的删除按钮。 |
| `ErrLanguageInvalid` | 400 | `WORKSPACE_LANGUAGE_INVALID` | 手敲 API 注入了非 `zh-CN`/`en` 值。 |
| - | 401 | `UNAUTH_NO_WORKSPACE` | Header 缺失或 id 在后端被物理注销。 |
