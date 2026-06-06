---
id: DOC-302
type: reference
status: active
owner: @weilin
created: 2026-06-06
reviewed: 2026-06-06
review-due: 2026-09-01
audience: [human, ai]
---
# WebSearch Domain — 搜索配置（provider 词表 + key picker，无存储薄层）

> **核心地位**：`domain/websearch` 是网络搜索的 **provider 词表** + **调用方据以得知当前 workspace 选定了哪把搜索 key 的端口**（`SearchKeyPicker`）。
>
> **与 model 同构**：仅含类型 + 端口的薄层，**无 store**——选定的 key id 存在 workspace 行上（与默认模型选择并列），`workspace.Service` 实现 `SearchKeyPicker`（正如它实现 `model.ModelPicker`）。`WebSearch` 工具（`tool/web`）依赖本包拿 provider 常量与 picker；各 provider 的 HTTP 调用在工具里、不在此处。
>
> **命名避坑**：叫 `websearch`（非 `search`）——`tool/search` 是无关的**文件系统搜索**（Glob/Grep/LS）。这里是**网络搜索**。

---

## 1. 物理布局

```
backend/internal/domain/websearch/websearch.go
  ├─ Provider 常量 brave/serper/tavily/bocha + IsProvider() + Providers()
  └─ SearchKeyPicker 接口 { DefaultSearchKeyID(ctx)(string,bool) }
```

无 store、无 Repository、无 DDL（存储借 workspace 表）。无 HTTP 端点（设置端点在 workspace handler，见 §4）。

---

## 2. provider 词表

Forgify 可把 WebSearch 查询路由到的搜索 provider：

| Provider | 说明 |
|---|---|
| `brave` | Brave Search（国际） |
| `serper` | Serper.dev（Google 结果代理，国际） |
| `tavily` | Tavily（AI 调优搜索，国际） |
| `bocha` | 博查 Bocha（国内） |

- `IsProvider(p)`：判 p 是否为已知搜索 provider。`WebSearch` 工具用它拒绝 provider 非搜索后端的 key（如用户把 default-search 指向了 LLM key），返清晰 tool-result。
- `Providers()`：按规范顺序返回全部——供 UI 列举与文档。

> 选定 key 的 provider **由 api-key 自身隐含**（`apikey.Credentials.Provider`），这些常量让工具据以分派、不必硬编码字符串。

---

## 3. `SearchKeyPicker` 端口（DIP）

```go
type SearchKeyPicker interface {
    DefaultSearchKeyID(ctx context.Context) (string, bool)
}
```

- 报告当前 workspace（id 取自 ctx）为搜索选定的 api-key id；未配置时 `ok=false`。
- 由 `workspace.Service` 实现（镜像 `model.ModelPicker` 的实现方式）。
- **单一显式选择、无优先级列表**——agent 永不挨个试 provider 乱烧钱（见 §5）。
- `ok=false` 时 `WebSearch` 降级到下个后端（MCP）或返"去配搜索后端"引导，而非报错。

---

## 4. 存储 + 设置（借 workspace，完全同 default-models）

| | model | websearch |
|---|---|---|
| workspace 字段 | `DefaultDialogue/Utility/Agent *ModelRef`（JSON 列） | `DefaultSearchKeyID string`（TEXT 列） |
| 为何形态不同 | ModelRef 有 3 字段 | 搜索只需选**一把 key**，provider 由 key 隐含 |
| Service 方法 | `Pick(ctx,scenario)` | `DefaultSearchKeyID(ctx)` + `SetDefaultSearch(ctx,id,keyID)` |
| 端点 | `PUT /workspaces/{id}/default-models/{scenario}` | `PUT/DELETE /workspaces/{id}/default-search` |
| caller 解析 | `keys.ResolveCredentialsByID(ref.APIKeyID)` | 同 |

- **`PUT /api/v1/workspaces/{id}/default-search`**：body `{apiKeyId}`，设默认搜索 key，返更新后的 workspace。
- **`DELETE /api/v1/workspaces/{id}/default-search`**：清除（→ WebSearch 降级 MCP / 引导）。
- **不校验 provider/category**：镜像 `SetDefault` 的运行时优雅风格——WebSearch 调用时拒非搜索 key，UI 只让选 `category=search` 的 key。

---

## 5. 为什么单选显式 = 防乱烧钱

旧版 WebSearch 持一个 `SearchProviderPriority=[brave,serper,tavily,bocha]`，**自动遍历**：哪个 provider 配了 key 就烧哪个、挨个试。这正是"乱烧钱"的来源。

新版（M1.2 收窄 apikey「选 key 显式化」+ 本 domain）：用户**显式选定一把搜索 key**，WebSearch 只用这一把，provider 从 key 隐含。**单选、可控、零遍历**。apikey 收窄成只按 id 发钥匙（`ResolveCredentialsByID`），把"选哪把"的逻辑上移到这里——和 model 的 scenario 默认选择同源。

---

## 6. `WebSearch` 消费链（tool/web，R0035）

```
picker.DefaultSearchKeyID(ctx)        → "ak_xxx"（未配 → MCP tier → 引导文案）
keys.ResolveCredentialsByID("ak_xxx") → Credentials{Provider:"brave", Key, BaseURL}
websearch.IsProvider(creds.Provider) ? switch creds.Provider:
    brave/serper/tavily/bocha → searchXxx(BaseURL, Key, query)   （HTTP 在工具内）
    : 非搜索 provider → 返"这把 key 不是搜索后端"tool-result
401/403 → keys.MarkInvalidByID("ak_xxx")
```

---

## 7. 跨域接线

| 接线 | 当下 | 实接 |
|---|---|---|
| `SearchKeyPicker` 实现 | `workspace.Service`（R0034 ✅，`var _` 断言） | — |
| `WebSearch` 消费 picker + provider 常量 | 接口/词表就位 | `tool/web`（R0035） |
| MCP tier（无 BYOK 时） | — | mcp（M3.6）注入 `MCPSearchRouter` |

---

## 8. 决策快照

- **独立建包**（对齐 `domain/model`）：搜索 provider 知识 + picker 接口集中一处，后续管理清晰
- **无 store**：选择是 workspace 偏好，存 workspace 行（同 default-models），不建独立表
- **单选 string 而非 SearchRef struct**：搜索当下只需一个 key id，不像 ModelRef 有 3 字段值得包 struct——反预留
- **provider 由 key 隐含**：apikey.Credentials.Provider 已带，无需 workspace 另存 provider 名
- **不校验 category**：运行时优雅（对齐 model）+ 反校验剧场 + workspace 零新依赖
