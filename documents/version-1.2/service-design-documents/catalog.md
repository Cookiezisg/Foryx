# Capability Catalog — V1.2 详设计

**Phase**：Phase 4 准备件（提前到位）
**状态**：✅ 重构为懒生成 + mechanical（2026-05-25）：domain types + `ErrAllSourcesFailed` + Service{New/RegisterSource/build/Get/GetForSystemPrompt} + 4 CatalogSource（function/handler/skill/mcp）+ chat runner SystemPromptProvider 注入 + 1 HTTP endpoint。**移除**：1s 轮询 / per-user 扇出 / LLM Generator / `.catalog.json` 磁盘 cache / version history / fingerprint / `catalog` notification。document 不再进 catalog（走 @-mention，独立功能）。
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 无表（纯派生，不落盘）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — `ErrAllSourcesFailed` → 503
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — catalog notification 已停发
- 关联设计：[`function.md`](./function.md) / [`handler.md`](./handler.md) / [`mcp.md`](./mcp.md) / [`skill.md`](./skill.md)（4 个 CatalogSource 实现方）

---

## 1. 一句话

把 function / handler / skill / mcp 的能力**机械汇总成一段结构化 Markdown 清单**注入 chat system prompt，让 LLM 知道"我有哪些可调用能力"。Catalog 是**纯派生视图**——开聊时按需现查各 source 拼装，不缓存、不落盘、零 LLM。

> **不含 document**：文档是"知识内容"不是"能力"，且不 scale（几十个文档名塞进每条系统提示纯噪音）。文档走用户 **@-mention** 引用时才进上下文（独立功能，本次未实现，留 TODO）。
> **不含 subagent / workflow**：Subagent tool 自身 description 已覆盖；workflow 是用户触发不是 LLM 意图匹配的能力。

**触发机制**：**懒生成（on-demand）**。chat runner 组装 system prompt 时调 `GetForSystemPrompt(ctx)` 现查四源拼清单。无后台 goroutine、无轮询、无 per-user 扇出、无缓存陈旧。MCP status 抖动无害——只读当前态拼字符串，不触发任何东西。

---

## 2. 端到端推演

### 运行期 — chat runner 拼 system prompt

```
user msg → chat.Send → runner.SystemPromptSections(ctx, conv)
  → catalogService.GetForSystemPrompt(ctx)   ← ctx 带 userID
      → build(ctx)：现查 function/handler/skill/mcp 四源（按 user）
      → assemble(items)：拼 "## Available capabilities\n### function (N, PerItem)\n- ..."
  → 拼进 system prompt（"catalog" 段）→ + tool 定义 → 发给 LLM
```

构建极便宜（几条 DB 查询 + 字符串拼装），单用户每回合现查可忽略；不加缓存（YAGNI）。

### HTTP 巡检

```
GET /api/v1/catalog → handler → svc.Get(r.Context()) → build(ctx) → 200 + Catalog
  全源失败 → 503 CATALOG_ALL_SOURCES_FAILED
```

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **派生视图，非 source of truth** | 不落盘、不缓存；每次从 4 个 source 现查重建；零数据损失 |
| **接口反转（CatalogSource port）** | catalog 不知道任何具体 source 类型；新增 source 0 行修改 catalog |
| **懒生成而非轮询** | 开聊时现查，不后台轮询、不 per-user 扇出。简单、永远最新、零空转、零成本 |
| **mechanical only** | 直接机械拼结构化清单喂给 LLM——精确、零幻觉、零 LLM 成本、瞬时确定。比"LLM 润色的二手摘要"更适合喂给另一个 LLM |
| **Generator 接口当缝** | domain 保留 `Generator` port（默认 nil → mechanical）。将来清单变大，往缝里塞按规模触发的**压缩/检索**策略；本次不实现（YAGNI） |
| **失败隔离** | 单 source `ListItems` 挂 → log Warn + 空列表代替；**全部 source 失败** → `GetForSystemPrompt` 返 ""（聊天照常）/ `Get` 抛 `ErrAllSourcesFailed`（503） |
| **document 排除** | 文档不进 catalog；走 @-mention 进上下文（独立功能 TODO） |

---

## 4. 领域接口

### CatalogSource port（`internal/domain/catalog/source.go`）

```go
type CatalogSource interface {
    Name() string                 // 稳定标识，用于日志 + 清单分组
    Granularity() Granularity     // PerItem（function/handler/skill/workflow/document）/ PerServer（mcp）
    // InvokeTool is the tool-call name the LLM must emit to use items from this source.
    // The menu renderer formats each entry as "- name [InvokeTool()]: desc".
    InvokeTool() string           // e.g. "run_function", "trigger_workflow", "call_handler"
    // ListItems 返当前全量 items；出错则该 source 用空列表代替（不打断其他）。
    // 必须返"当前真实状态"——半成品（如正在 connect 的 MCP server）不应出现。
    ListItems(ctx context.Context) ([]Item, error)
}

type Granularity int
const (
    PerItem   Granularity = iota  // function/handler/skill/workflow/document — 每条独立
    PerServer                      // mcp — 每 server 一条
)
```

### Item

```go
type Item struct {
    Source      string `json:"source"`            // = CatalogSource.Name()
    ID          string `json:"id"`                // source 内唯一
    Name        string `json:"name"`              // LLM-facing
    Description string `json:"description"`
    Category    string `json:"category,omitempty"`
}
```

### Catalog（生成的产物）

```go
type Catalog struct {
    Summary     string              `json:"summary"`     // 进 system prompt 的清单文本
    Coverage    map[string][]string `json:"coverage"`    // source → [item ID]
    GeneratedAt time.Time           `json:"generatedAt"`
    GeneratedBy string              `json:"generatedBy"` // 恒为 "mechanical"
}
```

### Sentinel（1 个）

```go
var ErrAllSourcesFailed = errors.New("catalog: all sources failed")
```

`build` 在全部 source `ListItems` 都报错时返此 sentinel。`GetForSystemPrompt` 吞掉返 ""；`Get`（HTTP）上抛 → errmap 503。

### SystemPromptProvider（chat runner 消费）

```go
type SystemPromptProvider interface {
    GetForSystemPrompt(ctx context.Context) string
}
```

`*catalogapp.Service` 实现；通过 setter 注入 chat runner，避免 chat import catalog 具体实现。

---

## 5. Service 层（`internal/app/catalog/catalog.go`）

```go
type Service struct {
    log       *zap.Logger
    sourcesMu sync.RWMutex
    sources   []catalogdomain.CatalogSource
}

func New(log *zap.Logger) *Service
func (s *Service) RegisterSource(src catalogdomain.CatalogSource)
func (s *Service) build(ctx) (*Catalog, error)        // 内部：现查所有 source + assemble；ctx 必须带 userID
func (s *Service) Get(ctx) (*Catalog, error)          // HTTP 巡检
func (s *Service) GetForSystemPrompt(ctx) string      // chat runner；err / 空 → ""
```

无后台 goroutine、无 ticker、无磁盘、无 version、无 fingerprint、无 notif。`build` 缺 userID 返 `reqctxpkg.ErrMissingUserID` 包装；全源失败返 `ErrAllSourcesFailed`；部分失败用成功的拼。

### assemble（`internal/app/catalog/mechanical.go`）

按 source 分组（字母序）拼能力菜单。每源 header 格式 `### <name> [invokeTool]`（`InvokeTool()` 提供工具名），每 item `- name: desc`，desc 截断 48 字符（`const descMaxRunes = 48`，rune-safe）。空库跳整段（返空 Summary）。`GeneratedBy="mechanical"`。Coverage = source→[id] 机械分组。

**为什么 48 字符**：防历史数据/超长描述击穿 token 预算；源头 `create_*`/`edit_*` 描述字段已有"一句话"引导，48 chars 兜底。

---

## 6. 集成点

### 6.1 注入 chat runner（`internal/app/chat/runner.go`）

`SystemPromptSections(ctx, conv)` 里，catalog 内容进 `capabilities` 段（capability-disclosure §4.1），不再是独立 `catalog` 段。

### 6.2 main.go 装配（已含 workflow + document）

```go
catalogService := catalogapp.New(log)
catalogService.RegisterSource(functionService.AsCatalogSource())   // InvokeTool="run_function"
catalogService.RegisterSource(handlerService.AsCatalogSource())    // InvokeTool="call_handler"
catalogService.RegisterSource(skillService.AsCatalogSource())      // InvokeTool="activate_skill"
catalogService.RegisterSource(mcpService.AsCatalogSource())        // InvokeTool="call_mcp_tool"
catalogService.RegisterSource(workflowService.AsCatalogSource())   // InvokeTool="trigger_workflow" ← 已注册
catalogService.RegisterSource(documentService.AsCatalogSource())   // InvokeTool="read_document"   ← 已注册
chatService.SetSystemPromptProvider(catalogService)
```

无 `SetGenerator` / `Start` / `Stop`。

---

## 7. HTTP API

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/catalog` | 巡检——按需现查并返当前用户的能力清单（summary + coverage）；全源失败 503 |

（旧的 `POST :refresh` / `GET /history` / `GET /diff` 已移除——懒生成下 refresh 等价于 get，无版本故无 history/diff。）

---

## 8. 错误码 + Notifications

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `catalogdomain.ErrAllSourcesFailed` | 503 | `CATALOG_ALL_SOURCES_FAILED` | `build` 全部 source 报错时触发；errmap 已登记 |

**Notifications**：catalog 不再发任何通知（懒生成无"变更事件"）。原 `catalog` notification 类型已停发。

---

## 9. 测试覆盖 ✅

| 层 | 文件 | 覆盖 |
|---|---|---|
| domain | `internal/domain/catalog/catalog_test.go` | Catalog JSON 4 字段 round-trip / Granularity String()+枚举 pin / Item omitempty / `ErrAllSourcesFailed` 前缀 |
| app/catalog | `internal/app/catalog/catalog_test.go` | no-sources 空 / 缺 userID（Get err、GetForSystemPrompt ""）/ mechanical 多源清单 + Coverage / 空库跳段 / 全源失败 err / 部分失败用成功源 / RegisterSource 并发 |
| app/chat | `internal/app/chat/runner_test.go` | nil/空 provider 跳 catalog 段 / 非空注入顺序 / SetSystemPromptProvider |
| transport/handlers | `internal/transport/httpapi/handlers/catalog_test.go` | GET 空库返空 Catalog（mechanical）/ GET 现查多源 |
| pipeline | `test/catalog/*.go` | AllSourcesCovered E2E / DescriptionChange 重建反映 / AlwaysMechanical / **DocumentsExcluded**（文档不进 catalog）/ trinity function+handler coverage |

---

## 10. 与各 source domain 的关系

```
┌──────────────┐
│   catalog    │ ← domain 定义 CatalogSource port（含 InvokeTool()）
└──────┬───────┘
       │ 被 implement（各 app 包内 catalogsource.go，svc.AsCatalogSource() 暴露）
   ┌───┴────┬────────┬────────┬──────────┬──────────┐
   ↓        ↓        ↓        ↓          ↓          ↓
function  handler  skill    mcp      workflow  document
catalog 永远不 import 任何 source 的 app 包。
```

| Source | InvokeTool | description 来源 | Granularity |
|---|---|---|---|
| **function** | `run_function` | 创建/编辑时生成 | PerItem |
| **handler** | `call_handler` | 创建时写 | PerItem |
| **skill** | `activate_skill` | author 写在 SKILL.md frontmatter | PerItem |
| **mcp** | `call_mcp_tool` | server 自报（tools/list）| PerServer |
| **workflow** | `trigger_workflow` | 创建时写 description | PerItem |
| **document** | `read_document` | 创建时写 description | PerItem |

所有 6 个 source 均已在 main.go 注册。

---

## 11. 演化方向

- **@-mention 文档 → 注入内容**（下一个独立 spec）：后端目前无 mention 处理；要新建"前端 @ 文档 + 后端 mentions→取内容→注入"。
- **规模化压缩 / 检索**：清单变大（~30-50+ item，每条系统提示固定 token 税）时，往 `Generator` 缝塞策略——倾向**检索相关子集**（保精度、天然 scale）而非"LLM 总结全量"。懒触发 + 缓存，绝不退回轮询。
- **Knowledge / Workflow source**（Phase 5）：实现 CatalogSource 接口加进去；catalog 0 行修改。
