# Capability Catalog — V1.2 详设计

**Phase**：Phase 4 准备件（提前到位）
**状态**：✅ D8 全部交付（2026-05-06）：domain types + 2 sentinels + Service{Start/Stop/Refresh/RegisterSource/GetForSystemPrompt} + LLMGenerator（3-attempt retry + coverage 校验 + mechanical fallback）+ pollLoop 1s + atomic.Bool 单 flight + fingerprint dedup + atomic disk cache + 3 CatalogSource（forge/skill/mcp）+ chat runner SystemPromptProvider 注入 + 2 HTTP endpoints + 3 离线 pipeline 场景
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 无新表（.catalog.json + memory）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — catalog ×2 内部消化（不进 errmap）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — **不**发 SSE（详 §13）
- 关联设计：[`forge.md`](./forge.md) / [`mcp.md`](./mcp.md) / [`skill.md`](./skill.md)（3 个 CatalogSource 实现方）

---

## 1. 一句话

把 forge / skill / mcp / 未来更多能力**统一汇总成一段薄文本**（~500 token）注入 system prompt，让 LLM 永远知道"我有什么类目能力 + 何时该 search 哪个 + 哪些容易功能重叠"。Catalog 本身是**纯派生 cache**——不是 source of truth，删了能从各 source 完全重建。

> **不包含 subagent**——Subagent system tool 自身的 description 已经说明它能 spawn 哪些 subagent 类型（Explore/Plan/general-purpose），catalog 再列一遍是冗余。LLM 看 tool 定义直接知道。

**触发机制**：**1 秒轮询 + fingerprint 检查 + atomic 单 flight**——简单到不会出错。无事件订阅、无中间态烦恼、无并发竞态——拉数据 → 算 hash → 变了就 regen。MCP 还在加载？fingerprint 自然反映"还没准备好"，等 MCP ready 后下一 tick 自动捕获。

---

## 2. 端到端推演

### 启动期

```
main.go → catalogapp.NewService(deps).Start(ctx)
  → 已注册的 sources: [forgeSource, mcpSource, skillSource]
  → 加载 ~/.forgify/.catalog.json 进 in-memory cache（瞬时）
      ✅ 文件存在 + 合法 → 用上次的 cache + 记下 lastFP
      ❌ 不存在 / 损坏 → 空 cache + lastFP=""，移损坏文件到 .bak
  → 启动 polling goroutine（每 1 秒一 tick）
  → 暴露 GetForSystemPrompt() 给 chat runner
```

### 运行期 — chat runner 拼 system prompt（热路径）

```
user msg → chat.Send → runner.buildSystemPrompt(ctx)
  → catalogService.GetForSystemPrompt() ← 进程内 cache 直接返
  → 拼到 system prompt 顶部："## Available capabilities\n..."
  → + tool 定义 → 发给 LLM
```

**永远从 in-memory cache 读，零 IO 零等待**——polling 在另一 goroutine 维护 cache。

### 运行期 — polling tick 干啥

```
每 1 秒 tick：
  1. atomic CAS busy:false→true
     失败 → 上次还在跑（>1s 的 LLM 调用）→ 直接跳过这次 tick
     成功 → 继续
  2. defer atomic.Store(busy, false)
  3. 拉所有 source items（各 source.ListItems(ctx)）
     - 单 source 失败 → log Warn + 用空列表代替（不打断其他 source）
     - 全 source 失败 → 直接 return（保留上次 cache）
  4. fingerprint = sha256(sort(source+name+description))
  5. 与 lastFP 比较：
     - 相同 → return（最常见，~99% 的 tick）
     - 不同 → 走 generator
  6. Generator.Generate（含 coverage 校验 + 重试 + LLM 轮训 + mechanical fallback）
  7. 成功 → 替换 in-memory cache + 写 .catalog.json + lastFP=新 fingerprint
     失败 → log Warn + lastFP 不更新（下 tick 再试，相当于自动重试）
```

### Cold start 优化

```
进程启动时 catalog cache 为空
  → 优先尝试加载 .catalog.json 作为初始值（Quick start）
  → 同时启动 background Build 任务
  → background Build 完成后 atomic 替换 cache
  → 这段窗口期内 chat turn 用的是上次 cache（可能略 stale 但可用）
```

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **派生视图，非 source of truth** | rm .catalog.json 后能从 4 个 source 完全重建；零数据损失 |
| **接口反转（CatalogSource port）** | catalog 不知道任何具体 source 类型；新增 source 0 行修改 catalog 代码 |
| **轮询而非事件订阅** | **每 1 秒 tick 一次**，拉 ListItems → 算 fingerprint → 变了 regen。**不订阅 events bridge**——避免事件中间态、并发竞态、forge streaming 噪音、各种 bug |
| **Atomic 单 flight** | `atomic.Bool busy` 守卫——上次 tick 的 regen 还没跑完，下一 tick 直接跳过。零依赖、无锁竞争 |
| **Fingerprint 防无效 regen** | hash(sort(source + name + description))；未变绝不调 LLM |
| **LLM-gen + 全覆盖校验** | 自由合并允许（"3 个 CSV 工具"），但每个原始 item 必须被覆盖；不达标重试或 fallback。**Generator 自身负责生成跨类目"路由观察"**（如"调 GitHub API 用 mcp 不要写新 forge"），不另设用户配置文件 |
| **失败隔离** | 单 source ListItems 挂 → log Warn + 用空列表代替；**全部 source 失败时**保留上次 cache，不写空 catalog |
| **Generator 输出硬上限** | LLM 输出 maxTokens=2000 cap；超 cap 视为生成失败 |
| **重试 2 次（共 3 次 attempt）** | 单次 attempt 失败（坏 JSON / 漏 item / 输出爆大）→ 重试，最多 2 次重试，3 次全败再 mechanical |
| **Key 轮训跑真 LLM** | 每次 attempt 内部按所有 apikey 依次试**真 LLM 调用**（不只是 build client）；任一 key 真跑成功即用 |
| **失败即 mechanical + 写 lastFP** | 3 次 attempt + 全 key 都失败 → mechanical 顶上 + lastFP 照常更新。**用户不动东西就不再耗 LLM**；改了东西 fp 自然变 → LLM 重新有机会跑——**"用户活跃度"驱动重试**，无须后台 backoff |
| **不发 SSE** | catalog 是内部组件，对前端透明；UI 看 source 各自的 SSE 即可（详 §13）|
| **Cache 防损坏** | cold-start 加载 `.catalog.json` parse fail → 移到 `.bak` + 当作空 cache 走完整 build |

---

## 4. 领域接口

### CatalogSource port（`internal/domain/catalog/source.go`）

```go
// CatalogSource 是任何能被纳入 Capability Catalog 的能力提供方必须实现的 port。
// 由 catalog domain 定义，由各业务 app（forgeapp / skillapp / mcpapp / 未来）实现。
// 注：subagent 不实现此接口——Subagent tool 自身 description 已覆盖 subagent 类型，catalog 不重复。
//
// CatalogSource is the port that any capability provider must implement to participate
// in the Capability Catalog. Defined in the catalog domain, implemented by business apps.
//
// 接口极简——catalog 用轮询架构，无须 source 自报事件。catalog goroutine 每 1s 调一次
// ListItems 取最新数据，与上次 fingerprint 比较，变了才 regen。
type CatalogSource interface {
    // Name 此 source 的稳定标识，用于日志、generator 路由提示
    Name() string

    // Granularity 告诉 generator 怎么合并 items（PerItem 可合并 / PerServer 通常不合并）
    Granularity() Granularity

    // ListItems 返回当前全量 items；catalog 每 1 秒调一次
    // 失败应返 error，catalog 会用空列表代替此 source（继续处理其他 source）+ Warn
    // **重要**：必须返"当前真实状态"——半成品（如 MCP 正在 connect 的 server）就该不出现在结果里
    ListItems(ctx context.Context) ([]Item, error)
}

type Granularity int
const (
    PerItem       Granularity = iota  // forge / skill — 每条独立可合并
    PerServer                          // mcp — 每 server 一条不合并
    PerCollection                      // 未来 knowledge — 每集合一条
)
```

### Item

```go
type Item struct {
    Source      string  `json:"source"`        // 同 CatalogSource.Name()
    ID          string  `json:"id"`            // source 内唯一
    Name        string  `json:"name"`          // LLM-facing
    Description string  `json:"description"`   // per-item description（来源各异）
    Category    string  `json:"category,omitempty"`  // 可选 hint，generator 合并时用
}
```

### Catalog（生成的产物）

```go
type Catalog struct {
    Summary       string               `json:"summary"`        // 进 system prompt 的人类可读文本（含 LLM 自动生成的路由观察）
    Coverage      map[string][]string  `json:"coverage"`       // source → [item ID]，校验完整性用
    Fingerprint   string               `json:"fingerprint"`    // sha256 of sorted (source+name+description)；用于跳过无效 regen
    GeneratedAt   time.Time            `json:"generatedAt"`
    Version       int                  `json:"version"`        // 自增
    SourcesAt     map[string]time.Time `json:"sourcesAt"`      // source → 该 source 上次拉数据时间戳
    GeneratedBy   string               `json:"generatedBy"`    // "llm" / "mechanical-fallback"
}
```

**没有 `RoutingHints` 字段**——路由观察由 LLM generator 看着所有 item descriptions **直接写进 Summary 文本**。用户想影响路由 → 编辑源头（forge / skill / mcp 各自的 description），catalog 自动 reflect。

### Sentinel 错误

```go
var (
    ErrCoverageIncomplete = errors.New("catalog: generator output missing items")
    ErrGenerationFailed   = errors.New("catalog: LLM generation failed")
)
```

均不上抛 handler——catalog 内部消化（重试或 fallback）。

---

## 5. 数据存储

| 数据 | 位置 | 持久化 | 说明 |
|---|---|---|---|
| Catalog summary + coverage | `~/.forgify/.catalog.json` | ✅ | 派生 cache；删了能重建 |
| Catalog 进程内 cache | RAM | ❌ | chat runner 热路径读取，零 IO |
| Per-source items | 各自 source 内（DB / 文件 / 内存）| ✅ | 不在 catalog 这里持久化 |

### `.catalog.json` 完整 schema

```json
{
  "summary": "## Available capabilities\n- 5 forges (3 CSV / 2 sorting)...\n## Notes on choosing\n- For CSV processing prefer local forges...",
  "coverage": {
    "forge": ["f_abc", "f_def", "f_xyz", "f_111", "f_222"],
    "skill": ["pr-review", "deploy", "data-export"],
    "mcp": ["github", "postgres"]
  },
  "generatedAt": "2026-05-04T13:42:00Z",
  "version": 17,
  "sourcesAt": {
    "forge": "2026-05-04T13:42:00Z",
    "skill": "2026-05-04T13:42:00Z",
    "mcp": "2026-05-04T13:42:00Z"
  },
  "generatedBy": "llm"
}
```

---

## 6. Service 层（`internal/app/catalog/catalog.go`）

```go
type Service struct {
    sources    []catalogdomain.CatalogSource
    generator  *Generator
    cache      *atomic.Pointer[catalogdomain.Catalog]
    lastFP     atomic.Value       // string，上次 fingerprint
    busy       atomic.Bool        // 单 flight 守卫
    cachePath  string             // ~/.forgify/.catalog.json
    pollInterval time.Duration    // 默认 1s
    log        *zap.Logger
    mu         sync.Mutex         // 仅保护 RegisterSource / 启停
}

func (s *Service) RegisterSource(src catalogdomain.CatalogSource)
func (s *Service) Start(ctx context.Context) error
func (s *Service) Stop(ctx context.Context) error
func (s *Service) Refresh(ctx context.Context) error            // 强制立即 refresh（HTTP `:refresh` 用）
func (s *Service) GetForSystemPrompt() string                    // 热路径，从 cache 读，无 IO
func (s *Service) Get() *catalogdomain.Catalog                   // debug / API 用
```

**整套基础设施由 polling 自然消化**——没有 events bridge subscriber、没有 burst 合并器、没有并发互斥锁库；单 goroutine + ticker + atomic.Bool 完事。

### Start

```go
func (s *Service) Start(ctx context.Context) error {
    // 1. 加载磁盘 cache（cold start 瞬时给 chat runner 用）
    //    parse 失败 → 移 .bak + 空 cache
    if cached, err := loadFromDisk(s.cachePath); err == nil {
        s.cache.Store(cached)
        s.lastFP.Store(cached.Fingerprint)  // ← 不管 LLM 还是 mechanical 版本都复用
    } else if !os.IsNotExist(err) {
        s.log.Warn("catalog cache parse failed; backing up + starting empty",
            zap.Error(err))
        os.Rename(s.cachePath, s.cachePath+".bak")
        s.lastFP.Store("")
    } else {
        s.lastFP.Store("")
    }
    
    // 2. 启动 polling goroutine
    go s.pollLoop(ctx)
    
    return nil
}

// pollLoop 每 1 秒 tick 一次。ctx cancel 时退出。
func (s *Service) pollLoop(ctx context.Context) {
    ticker := time.NewTicker(s.pollInterval)
    defer ticker.Stop()
    
    // 启动时立即跑一次（不等第一个 tick）
    s.tryRefresh(ctx)
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.tryRefresh(ctx)
        }
    }
}

// tryRefresh 是 polling 的核心——atomic 单 flight + fingerprint 检查 + 调 Refresh。
func (s *Service) tryRefresh(ctx context.Context) {
    // 单 flight：上次还在跑就跳过
    if !s.busy.CompareAndSwap(false, true) {
        return
    }
    defer s.busy.Store(false)
    
    if err := s.Refresh(ctx); err != nil {
        s.log.Warn("catalog refresh skipped/failed; keeping previous cache",
            zap.Error(err))
    }
}
```

### Refresh — 拉数据 + fingerprint 检查 + regen

```go
func (s *Service) Refresh(ctx context.Context) error {
    items := []catalogdomain.Item{}
    sourcesAt := map[string]time.Time{}
    granularityMap := map[string]catalogdomain.Granularity{}
    failedCount := 0
    
    for _, src := range s.sources {
        srcItems, err := src.ListItems(ctx)
        if err != nil {
            s.log.Warn("catalog source failed; using empty",
                zap.String("source", src.Name()), zap.Error(err))
            failedCount++
            continue  // 单 source 挂不影响其他
        }
        items = append(items, srcItems...)
        sourcesAt[src.Name()] = time.Now()
        granularityMap[src.Name()] = src.Granularity()
    }
    
    // 全部 source 都挂了 → 保留上次 cache，不写空 catalog
    if failedCount == len(s.sources) && len(s.sources) > 0 {
        return fmt.Errorf("catalogapp.Refresh: all %d sources failed; keeping previous cache", len(s.sources))
    }
    
    // ⚡ fingerprint 检查 —— polling 的核心优化
    fp := fingerprint(items)
    if last, _ := s.lastFP.Load().(string); last == fp {
        return nil  // ~99% 的 tick 走这里（没变化）
    }
    
    // 真变了 → 调 LLM（失败走 mechanical fallback）
    catalog, err := s.generator.Generate(ctx, items, granularityMap)
    if err != nil {
        s.log.Warn("catalog generation failed; using mechanical fallback",
            zap.Error(err))
        catalog = mechanicalFallback(items)
    }
    catalog.Fingerprint = fp
    catalog.GeneratedAt = time.Now()
    catalog.Version = s.nextVersion()
    catalog.SourcesAt = sourcesAt
    
    // ⚡ 关键：lastFP 总是更新（无论 LLM 成功还是 mechanical fallback）
    // 哲学：失败时 mechanical 顶上让用户能用，lastFP 写进去 → 用户不动东西就不再耗 LLM；
    // 用户改了东西 fp 自然变 → LLM 重新有机会跑。"用户活跃度"驱动重试，无须后台 backoff。
    s.cache.Store(catalog)
    s.lastFP.Store(fp)
    if err := saveToDisk(s.cachePath, catalog); err != nil {
        s.log.Warn("catalog write to disk failed", zap.Error(err))
    }
    return nil
}

// fingerprint 仅哈希影响 catalog summary 的字段：source + name + description。
// 用户改 forge code / tags / etc. 都不会触发 LLM 调用——只有 name/description 改了才触发。
func fingerprint(items []catalogdomain.Item) string {
    sorted := append([]catalogdomain.Item(nil), items...)
    sort.Slice(sorted, func(i, j int) bool {
        if sorted[i].Source != sorted[j].Source {
            return sorted[i].Source < sorted[j].Source
        }
        return sorted[i].Name < sorted[j].Name
    })
    h := sha256.New()
    for _, it := range sorted {
        h.Write([]byte(it.Source + "\x00" + it.Name + "\x00" + it.Description + "\x00"))
    }
    return hex.EncodeToString(h.Sum(nil))
}
```

**MCP 加载中间态自动消化的细节**：
- t=0：mcpapp.ListItems() 返 `[]`（还没 server connect）→ fingerprint = hash(no items) → 与 lastFP 不同就 regen 一次
- t=1s：A 还在 connect → ListItems 仍 `[]` → fingerprint 不变 → 不 regen
- t=2s：A ready → ListItems 返 `[A]` → fingerprint 变 → regen 一次
- t=3s：B ready → ListItems 返 `[A, B]` → fingerprint 变 → regen 一次
- t=4s+：稳定 → fingerprint 不变 → 不 regen

启动期最多浪费 N+1 次 LLM 调用（N = MCP server 数）。每次 ~$0.001。**完全可接受**。

---

## 7. Generator（`internal/app/catalog/generator.go`）

### Prompt 设计

```go
const generatorPromptTemplate = `
You are generating a "Capability Catalog" summary that will be inserted into another LLM's system prompt.
The summary tells the other LLM what high-level capability categories are available, when to use each, and how to discover details.

CONSTRAINTS — ALL MANDATORY:
1. Coverage: every item below MUST be represented (directly named or grouped).
   You MUST output a "coverage" field listing every source ID you grouped/named.
2. Brevity: total summary <= 600 tokens. Prefer "5 file-processing tools" over listing 5 names.
3. Granularity rules:
   - source granularity=PerItem (forge, skill): grouping/merging allowed
   - source granularity=PerServer (mcp): one mention per server, do NOT merge
   - source granularity=PerCollection: one mention per collection
4. **Detect overlap and write routing observations inline**: If two items in different
   sources serve similar purposes (e.g., a forge that calls GitHub API + a github MCP server),
   add a "Notes on choosing" section to the summary telling the LLM which to prefer and why.
   Inferences should come from the item descriptions provided below.
5. End with: "If a task could fit multiple categories, you MAY call multiple search tools in parallel."

OUTPUT JSON:
{
  "summary": "...",  // includes "Notes on choosing" section when overlap detected
  "coverage": { "forge": [<all forge IDs>], "skill": [...], "mcp": [...] }
}

ITEMS:
%s
`
```

**关键设计**：路由提示**不是用户单独写的文件**，而是 generator 看着所有 item description**自动推断生成**的"Notes on choosing"段落，就在 summary 里。用户想影响路由 → 编辑 forge/skill/mcp 各自的 description（source 是 truth），下次 regen 自动 reflect。

### Coverage 校验 + 重试 2 次 + Key 轮训跑真 LLM + 输出上限

```go
const (
    generatorMaxOutputTokens = 2000  // 硬上限，防 LLM 输出爆大
    generatorMaxAttempts     = 3     // 初次 + 2 次重试
)

func (g *Generator) Generate(ctx context.Context, items []Item, gMap map[string]Granularity) (*Catalog, error) {
    sourceIDs := groupBySource(items)  // map[source][]ID
    
    for attempt := 0; attempt < generatorMaxAttempts; attempt++ {
        prompt := buildPrompt(items, gMap, attempt)
        // attempt > 0 时加："Previous attempt missed: [...]; you must include all of them this time"
        
        // ⚡ Key 轮训：每次 attempt 内部按所有 apikey 顺序试调真 LLM，首个成功的赢
        raw, err := g.callLLMWithKeyRotation(ctx, prompt)
        if err != nil {
            // 全 key 都不可用 → 不再 attempt（无 key 重试也没用）
            return nil, fmt.Errorf("catalog.Generate: no working LLM (attempt %d): %w", attempt, err)
        }
        if len(raw) > generatorMaxOutputTokens*5 {  // 字符数粗略上限（防 maxTokens 失效兜底）
            continue  // 输出爆大 → 重试
        }
        
        var parsed struct {
            Summary  string              `json:"summary"`
            Coverage map[string][]string `json:"coverage"`
        }
        if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil {
            continue  // JSON 坏 → 重试
        }
        
        missing := validateCoverage(sourceIDs, parsed.Coverage)
        if len(missing) > 0 {
            g.log.Warn("catalog generation missing items",
                zap.Strings("missing", missing), zap.Int("attempt", attempt))
            continue  // 漏 item → 重试
        }
        
        return &Catalog{
            Summary:     parsed.Summary,
            Coverage:    parsed.Coverage,
            GeneratedBy: "llm",
        }, nil
    }
    return nil, ErrCoverageIncomplete  // 3 次都失败 → 调用方走 mechanical fallback
}

// callLLMWithKeyRotation 在所有配置的 LLM key 上**依次跑真 LLM**，
// 单个 key 的 LLM 调用 fail（如 401 / 网络 / 模型不存在）→ 立即试下一个 key，
// 不只是 "build client" 阶段成功就罢手。首个 LLM 调用成功的 key 赢。
//
// 优先 chat 场景对应的 key；它失败再遍历其他 apikey。
func (g *Generator) callLLMWithKeyRotation(ctx context.Context, prompt string) (string, error) {
    opts := llm.GenerateOpts{MaxTokens: generatorMaxOutputTokens}
    
    // 先试 chat 场景
    if bundle, err := g.llm.ResolveForChat(ctx); err == nil {
        if raw, err := llm.Generate(ctx, bundle.Client, prompt, opts); err == nil {
            return raw, nil
        } else {
            g.log.Warn("chat-scenario LLM failed; trying other keys", zap.Error(err))
        }
    }
    
    // 遍历所有 apikey + 各自 default model 真跑 LLM
    for _, key := range g.keys.List(ctx) {
        client, err := g.factory.Build(llminfra.Config{Provider: key.Provider, ...})
        if err != nil { continue }
        if raw, err := llm.Generate(ctx, client, prompt, opts); err == nil {
            return raw, nil
        }
        // 这个 key 的 LLM 失败 → 试下一个
    }
    return "", errors.New("all configured LLM keys failed")
}
```

**重试 + 轮训关系图**：

```
Generate() 入口
  ├─ attempt 0
  │   └─ callLLMWithKeyRotation
  │       ├─ chat-scenario key → 真跑 LLM
  │       ├─ apikey 1 → 真跑 LLM    ← 任一成功立即返
  │       ├─ apikey 2 → 真跑 LLM
  │       └─ apikey N → 真跑 LLM
  │   └─ 拿到 LLM 输出 → JSON parse / coverage 校验
  │       ├─ 通过 → 返回，结束
  │       └─ 不通过 → attempt+1
  ├─ attempt 1（带 "上次漏了 X" 提示）→ 同上
  ├─ attempt 2（最后一次）→ 同上
  └─ 全失败 → return ErrCoverageIncomplete → 调用方 mechanical fallback
```

### Mechanical Fallback

```go
func mechanicalFallback(items []Item) *Catalog {
    // 极简：每 source 一段，全列名字 + description；不含 LLM 推断的路由观察
    var b strings.Builder
    coverage := map[string][]string{}
    for _, src := range groupBySourceFunc(items) {
        b.WriteString(fmt.Sprintf("\n### %s (%d)\n", src.Name, len(src.Items)))
        for _, it := range src.Items {
            b.WriteString(fmt.Sprintf("- %s: %s\n", it.Name, it.Description))
            coverage[src.Name] = append(coverage[src.Name], it.ID)
        }
    }
    b.WriteString("\nIf a task could fit multiple categories, you MAY call multiple search tools in parallel.\n")
    return &Catalog{
        Summary: b.String(),
        Coverage: coverage,
        GeneratedBy: "mechanical-fallback",
    }
}
```

**fallback 牺牲精炼 + 失去路由推断**，但保证完整覆盖——演示时哪怕 LLM 抽风也不会出现"AI 不知道某个 forge 存在"的尴尬。

---

## 8. 集成点

### 8.1 注入 chat runner（`internal/app/chat/runner.go`）

```go
// 当前 buildSystemPrompt 改造：
func (r *runner) buildSystemPrompt(ctx context.Context, conv *conversation, tools []toolapp.Tool) string {
    var b strings.Builder
    b.WriteString(basePrompt)
    
    // 新增：catalog 块
    if r.catalog != nil {
        catalogText := r.catalog.GetForSystemPrompt()
        if catalogText != "" {
            b.WriteString("\n\n")
            b.WriteString(catalogText)
        }
    }
    
    // 已有：tool 定义...
    return b.String()
}
```

`runner` struct 新加字段 `catalog catalogapp.SystemPromptProvider`（接口注入）。

### 8.2 SystemPromptProvider 接口

```go
// internal/domain/catalog/catalog.go
type SystemPromptProvider interface {
    GetForSystemPrompt() string
}
```

`Service` 实现此接口；通过 setter 注入 `chat runner`，避免 `chat` import `catalog` 具体实现。

### 8.3 main.go 装配

```go
// 1. 创建 service（无须 events bridge——polling 不订阅）
catalogService := catalogapp.NewService(llmResolver, log)

// 2. 注册 sources
catalogService.RegisterSource(forgeService.AsCatalogSource())
catalogService.RegisterSource(mcpService.AsCatalogSource())
catalogService.RegisterSource(skillService.AsCatalogSource())
// subagent 不注册——Subagent tool 自身 description 已覆盖 subagent 类型说明

// 3. 注入 chat
chatService.SetSystemPromptProvider(catalogService)

// 4. 启动（pollLoop 在自己 goroutine 起）
catalogService.Start(ctx)
```

**source 注册顺序无要求**——polling 不依赖 events bridge，注册完启动 polling 自己会把 source 都拉一遍。

---

## 9. HTTP API

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/catalog` | debug / 调试视图——返当前 cache 内容（含 summary + coverage）|
| `POST /api/v1/catalog:refresh` | 手动触发 refresh（绕过 1s polling 间隔，强制立即跑一次 tryRefresh）|

**:refresh** 主要给 testend 调试 / 用户在 UI 点"立即更新"用。

**没有 routing-hints 端点**——路由提示由 generator LLM-gen 时直接写进 summary，用户想影响路由 → 编辑源头 forge/skill/mcp 的 description。

---

## 10. 错误码

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `catalogdomain.ErrCoverageIncomplete` | (不到 handler) | — |
| `catalogdomain.ErrGenerationFailed` | (不到 handler) | — |

均内部消化。HTTP `:refresh` 失败时返通用 500 + 日志详情。

---

## 11. 测试覆盖 ✅

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| domain | `internal/domain/catalog/catalog_test.go` | 5 | Catalog JSON 全 7 字段 round-trip / Granularity String() + 枚举值 pin (PerItem=0 是新 source 安全默认) / Item JSON + Category omitempty / sentinel 唯一性 + 'catalog: ' 前缀审计 |
| app/catalog | `internal/app/catalog/catalog_test.go` | 18 | NewHasEmptyCache / RegisterSourceConcurrent (并发安全) / Refresh empty/nil-gen mech-fallback/wired-gen LLM-path/gen-error-fallback/all-sources-fail-keeps-cache/partial-failure-isolation / Fingerprint 短路 + 描述变 trigger / pollLoop FiresAtLeastOnce (20ms 间隔) / TryRefresh BusyGuard SkipsConcurrent (slowGenerator entered2=0) / Start LoadsExistingCache (Version 7 → nextVersion=8) / Start CorruptCacheMovedToBak / Refresh PersistsToDisk / Fingerprint stable shuffle + changes on description + ignores ID-only |
| app/catalog/generator | `internal/app/catalog/generator_test.go` | 8 | buildPrompt contains all items + granularity hints / first-attempt no retry hint / retry-attempt embeds missing IDs / groupSourceIDs / findMissing full coverage + partial + extras-ignored / NewLLMGenerator nil log OK |
| app/chat | `internal/app/chat/runner_test.go` | 5 | NilProvider skips catalog block / EmptyProviderText skips (boot window safety) / NonEmptyProvider injects 顺序 (intro → catalog → locale) / per-conv SystemPrompt 独立 / SetSystemPromptProvider 真 mutate |
| transport/handlers | `internal/transport/httpapi/handlers/catalog_test.go` | 4 | Get empty cache → null in envelope / Refresh builds + returns Catalog (asserts mech-fallback + Coverage + Summary + Fingerprint + Version 1) / Get after Refresh 返 cached / Refresh short-circuits when fingerprint unchanged |
| pipeline | `test/catalog/catalog_test.go` | 3 | AllSourcesCovered E2E (forge + skill 都进 Coverage + Summary + GetForSystemPrompt) / ForgeDescriptionChange triggers regen (Version + Fingerprint + Summary 都变) / NoLLMKey FallsBackToMechanical (mech-fallback + 第二次 no-op Refresh 短路防 per-tick LLM 重试) |

总计 40 单测 + 3 pipeline 场景全绿。

**实施时发现的关键 bug**：catalog 在后台 goroutine 跑（无 HTTP middleware ctx）→ 所有下游调用（source ListItems → repo 多租户查询；LLMGenerator → llmclient.Resolve → model picker）都会 fail 'reqctx: missing user id'。修复：Service.Refresh 入口注入 DefaultLocalUserID（单人 app 安全），让一处修复覆盖全 pipeline。

---

## 12. 与各 source domain 的关系

```
┌──────────────┐
│   catalog    │ ← domain 定义 CatalogSource port
│  (port 拥有方) │
└──────┬───────┘
       │ 被 implement
   ┌───┴────┬────────┬────────┬─────────┐
   ↓        ↓        ↓        ↓         ↓
┌──────┐┌──────┐┌─────┐┌─────────┐┌─────────┐
│forge ││skill ││ mcp ││future...│
└──────┘└──────┘└─────┘└─────────┘└─────────┘
   各 app 在自己包内提供 catalogsource.go 实现 port
   通过 svc.AsCatalogSource() 暴露给 main.go 注册

catalog 永远不 import 任何 source 的 app 包。
```

| Source | description 来源 | Granularity |
|---|---|---|
| **forge** | LLM-gen（forge 创建/编辑时即生成）| PerItem |
| **skill** | author 写在 SKILL.md frontmatter | PerItem |
| **mcp** | server 自报（tools/list 返回 description）| PerServer |

> **注**：CatalogSource 接口仅 `Name()` / `Granularity()` / `ListItems()` 三方法——catalog 改 polling 主动拉，无须事件订阅。

---

## 13. 演化方向

- **Knowledge / Workflow source**（Phase 5）：实现 CatalogSource 接口加进去；catalog 0 行修改
- **Catalog 自身分页**（如果某 category 真超过 50+ item）：generator 输出 "data-processing 类: 12 items, call list_forges(category='data') for full list"，二次嵌套 progressive disclosure
- **多 catalog（per-conversation）**：当前所有对话共享一个 catalog；未来用户可定制"这个对话只看 forge X / Y"
- **Plugin 动态注册**：plugin manager 在运行时调 RegisterSource 注册第三方 source；要求 RegisterSource thread-safe（已在设计内）
- **Catalog version diff**：UI 显示"自上次看以来 catalog 加了什么"，需要持久化历史版本（当前只 cache 最新）
