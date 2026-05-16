# Capability Catalog — V1.2 详设计

**Phase**：Phase 4 准备件（提前到位）
**状态**：✅ D8 全部交付（2026-05-06）：domain types + 3 sentinels + Service{Start/Stop/Refresh/RegisterSource/SetGenerator/SetPollInterval/GetForSystemPrompt/Get} + LLMGenerator（单次 attempt + mechanical fallback，屎山拯救计划 #7）+ pollLoop 1s + atomic.Bool 单 flight + fingerprint dedup + atomic disk cache + 3 CatalogSource（forge/skill/mcp）+ chat runner SystemPromptProvider 注入 + 2 HTTP endpoints + 3 离线 pipeline 场景 + `catalog` notification per cache fingerprint change
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 无新表（.catalog.json + memory）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — catalog ×3（`ErrAllSourcesFailed` 接 errmap 503；其余 2 个内部消化）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — `catalog` entity-state 通知（每次 cache fingerprint 变化 publish 一次）
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
| **失败隔离** | 单 source ListItems 挂 → log Warn + 用空列表代替；**全部 source 失败时**保留上次 cache + 抛 `ErrAllSourcesFailed`，不写空 catalog |
| **Generator 输出硬上限** | LLM 输出 maxTokens=2000 cap；超 cap 视为生成失败 |
| **单次 attempt + mechanical fallback**（屎山拯救计划 #7，2026-05-08）| 现代 LLM 首次成功率 ~99%，重试基本不工作；外部 1s 轮询本身就是天然重试。删掉 retry loop / coverage 校验 / missing hint。失败即 mechanical 兜底 |
| **失败即 mechanical + 写 lastFP** | LLM attempt 失败 → mechanical 顶上 + lastFP 照常更新。**用户不动东西就不再耗 LLM**；改了东西 fp 自然变 → LLM 重新有机会跑——**"用户活跃度"驱动重试**，无须后台 backoff |
| **每次 fingerprint 变化 publish notification** | cache 真更新后 publish `catalog` notification（type="catalog", id="*", data 含 fingerprint/version/generatedAt）；同 fingerprint 短路时不 publish。详 §10 |
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

// 注：本接口**不**含 `EventTopics() []string` 方法——catalog 完全靠 1s 轮询拉数据，不订阅事件。

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
    Coverage      map[string][]string  `json:"coverage"`       // source → [item ID]（function/handler/skill/mcp；workflow 不入,因 workflow 是用户触发不是 LLM 意图匹配的能力）
    Fingerprint   string               `json:"fingerprint"`    // sha256 of sorted (source+name+description)；用于跳过无效 regen
    GeneratedAt   time.Time            `json:"generatedAt"`
    Version       int                  `json:"version"`        // 自增
    SourcesAt     map[string]time.Time `json:"sourcesAt"`      // source → 该 source 上次拉数据时间戳
    GeneratedBy   string               `json:"generatedBy"`    // "llm" / "mechanical-fallback"
}
```

**没有 `RoutingHints` 字段**——路由观察由 LLM generator 看着所有 item descriptions **直接写进 Summary 文本**。用户想影响路由 → 编辑源头（forge / skill / mcp 各自的 description），catalog 自动 reflect。

### Sentinel 错误（3 个）

```go
var (
    ErrCoverageIncomplete = errors.New("catalog: generator output missing items")
    ErrGenerationFailed   = errors.New("catalog: LLM generation failed")
    ErrAllSourcesFailed   = errors.New("catalog: all sources failed")
)
```

`ErrCoverageIncomplete` / `ErrGenerationFailed` 均内部消化——LLM attempt 失败 → mechanicalFallback 顶上，不上抛 handler。

`ErrAllSourcesFailed` 在 `Service.Refresh` 里：当全部 source `ListItems` 都报错时返此 sentinel，**保留上次 cache 不写空**。HTTP `:refresh` 端点遇此返 503（详 §10）。

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
    generator  Generator                          // interface; nil → mechanical 兜底
    cache      *atomic.Pointer[catalogdomain.Catalog]
    lastFP     atomic.Value       // string，上次 fingerprint
    busy       atomic.Bool        // 单 flight 守卫
    cachePath  string             // ~/.forgify/.catalog.json
    pollInterval time.Duration    // 默认 1s
    notif      notificationspkg.Publisher  // 每次 cache fingerprint 变化 publish "catalog" 通知
    log        *zap.Logger
    sourcesMu  sync.RWMutex       // 保护 sources slice（RegisterSource）
    versionMu  sync.Mutex         // 保护 version 自增
    stopOnce   sync.Once
    pollDone   chan struct{}
}

func (s *Service) RegisterSource(src catalogdomain.CatalogSource)
func (s *Service) SetGenerator(g Generator)                       // 后置注入（main.go 装配序需要）
func (s *Service) SetPollInterval(d time.Duration)                 // 测试注入
func (s *Service) Start(ctx context.Context) error
func (s *Service) Stop()                                            // 幂等；stopOnce + 阻塞等 pollDone
func (s *Service) Refresh(ctx context.Context) error                // 强制立即 refresh（HTTP `:refresh` 用）
func (s *Service) GetForSystemPrompt() string                       // 热路径，从 cache 读，无 IO
func (s *Service) Get() *catalogdomain.Catalog                      // debug / API 用
```

**整套基础设施由 polling 自然消化**——没有 events bridge subscriber、没有 burst 合并器、没有并发互斥锁库；单 goroutine + ticker + atomic.Bool 完事。

**对外通知的唯一通道是 notifications**：cache 真更新时 publish 一条 `catalog` 通知，前端按需订阅。catalog 自身不订阅任何事件流。

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
    
    // 全部 source 都挂了 → 保留上次 cache，不写空 catalog；上抛 ErrAllSourcesFailed
    if failedCount == len(s.sources) && len(s.sources) > 0 {
        return fmt.Errorf("catalogapp.Refresh: %w (%d sources)", ErrAllSourcesFailed, len(s.sources))
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

    // 推 notification（cache 真更新了才 publish；同 fp 短路那条路径不到这里）
    if s.notif != nil {
        s.notif.Publish(ctx, "catalog", "*", map[string]any{
            "fingerprint": catalog.Fingerprint,
            "version":     catalog.Version,
            "generatedAt": catalog.GeneratedAt,
        })
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
1. Brevity: total summary <= 600 tokens. Prefer "5 file-processing tools" over listing 5 names.
2. Granularity rules:
   - source granularity=PerItem (forge, skill): grouping/merging allowed
   - source granularity=PerServer (mcp): one mention per server, do NOT merge
   - source granularity=PerCollection: one mention per collection
3. **Detect overlap and write routing observations inline**: If two items in different
   sources serve similar purposes (e.g., a forge that calls GitHub API + a github MCP server),
   add a "Notes on choosing" section to the summary telling the LLM which to prefer and why.
   Inferences should come from the item descriptions provided below.
4. End with: "If a task could fit multiple categories, you MAY call multiple search tools in parallel."

OUTPUT JSON:
{
  "summary": "..."   // includes "Notes on choosing" section when overlap detected
}

ITEMS:
%s
`
```

**关键设计**：
- 路由提示**不是用户单独写的文件**，而是 generator 看着所有 item description**自动推断生成**的"Notes on choosing"段落，就在 summary 里。用户想影响路由 → 编辑 forge/skill/mcp 各自的 description（source 是 truth），下次 regen 自动 reflect。
- **LLM 只输出 `summary`，不输出 `coverage`**（2026-05 #13 修）：coverage 是机械"item 按 source name 分组"的派生数据——让 LLM 输出它纯属浪费 token + 还可能漏。改成**后端 `computeCoverage(items)` 按 `item.Source` 名字 group**生成，100% 完整 + 永不漏。LLM 专心写 summary 这件它擅长的事。

### 单次 attempt + mechanical fallback（屎山拯救计划 #7，2026-05-08）

> **历史**：原设计是 "3-attempt retry + coverage 校验 + missing-id hint 重写"。后来发现：
> 1. 现代 LLM（DeepSeek/GPT-4o/Claude）做这种 "读列表 + 写总结" 任务**首次成功率 ~99%**，重试基本不工作
> 2. 真失败的原因（网络挂 / key 失效）**重试 3 次也救不了**——同 client/key 同样失败
> 3. catalog 本身有 1s 轮询，**外部已经是天然重试**（用户活动一变，下次 polling 自动再生成）
> 4. 失败时 mechanicalFallback 已经兜底——LLM 写得糙一点 ≠ 系统挂
>
> 所以 **2026-05-08 删 retry loop + missing hint + coverage 校验**，回到单次设计。

```go
const generatorOutputCharCap = 10 * 1024  // 防御 cap，超之视畸形

func (g *LLMGenerator) Generate(ctx context.Context, items []Item, gMap map[string]Granularity) (*Catalog, error) {
    if len(items) == 0 {
        return mechanicalFallback(items, gMap), nil
    }
    bundle, err := llmclient.Resolve(ctx, g.picker, g.keys, g.factory)
    if err != nil { return nil, fmt.Errorf("%w: resolve LLM: %v", ErrGenerationFailed, err) }

    raw, err := llm.Generate(ctx, bundle.Client, prompt)
    if err != nil { return nil, fmt.Errorf("%w: %v", ErrGenerationFailed, err) }
    if len(raw) > generatorOutputCharCap { return nil, fmt.Errorf("%w: output exceeded cap", ErrGenerationFailed) }

    jsonStr, ok := llmparse.ExtractJSON(raw)
    if !ok { return nil, fmt.Errorf("%w: no JSON", ErrGenerationFailed) }
    
    var parsed struct {
        Summary string `json:"summary"`
    }
    if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
        return nil, fmt.Errorf("%w: parse JSON: %v", ErrGenerationFailed, err)
    }
    if strings.TrimSpace(parsed.Summary) == "" {
        return nil, fmt.Errorf("%w: empty Summary", ErrGenerationFailed)
    }
    coverage := computeCoverage(items)  // 后端按 item.Source 名字 group,100% 完整
    return &Catalog{Summary: parsed.Summary, Coverage: coverage, GeneratedBy: "llm"}, nil
}

// computeCoverage groups items by their Source name. Replaces the
// previous LLM-output coverage (2026-05 #13): mechanical grouping is
// 100% complete and never drops items, freeing the LLM to focus on
// the summary text.
//
// computeCoverage 按 item.Source 名字分组,2026-05 #13 替代 LLM 输出的
// coverage:机械 group 永不漏 + 释放 LLM token 给 summary 本身。
func computeCoverage(items []Item) map[string][]string {
    out := make(map[string][]string)
    for _, it := range items {
        if it.Source == "" { continue }
        out[it.Source] = append(out[it.Source], it.ID)
    }
    for k := range out { sort.Strings(out[k]) }
    return out
}
```

任何错误 → ErrGenerationFailed → Service.Refresh 切 mechanicalFallback。**Coverage 不再依赖 LLM 输出**——`computeCoverage` 是确定性 source-name keyed group，永不漏 item。

**已删的旧代码**（屎山拯救计划 #7，保留作历史记录）：

- `generatorMaxAttempts` 常量（值=3）
- `for attempt := 0; attempt < generatorMaxAttempts; attempt++` 重试循环
- `findMissing` / `groupSourceIDs` helpers + 它们的 6 个测试
- `buildPrompt` 的 `missingHint []string` 入参 + "previous attempt missed: [...]" prompt 段
- `callLLMWithKeyRotation` 多 key 轮训（V1 也根本没真接，是占位）
- `ErrCoverageIncomplete` 触发路径（sentinel 自身保留，将来可能再用）

```go
// 已删 — 历史片段
for attempt := 0; attempt < generatorMaxAttempts; attempt++ {
    prompt := buildPrompt(items, gMap, missingHint)
    raw, err := callLLMWithKeyRotation(ctx, prompt)
    // ... missing := validateCoverage(sourceIDs, parsed.Coverage)
    if len(missing) > 0 { missingHint = missing; continue }
    return ..., nil
}
return nil, ErrCoverageIncomplete
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
```

**单次调用流图**（屎山拯救计划 #7 后）：

```
Generate() 入口
  ├─ items 空 → mechanicalFallback 直返
  ├─ 解 LLM bundle 失败 → ErrGenerationFailed
  ├─ llm.Generate 调用：
  │   ├─ 成功 + Summary 非空 → 返 Catalog{GeneratedBy:"llm"}
  │   ├─ 输出 > 10KB → ErrGenerationFailed
  │   ├─ JSON 解析失败 → ErrGenerationFailed
  │   ├─ Summary 空 → ErrGenerationFailed
  │   └─ 传输失败 → ErrGenerationFailed
  └─ 任何 ErrGenerationFailed → 调用方 Service.Refresh 切 mechanicalFallback
```

下次 polling tick（最长 1 秒）→ 用户活动一变 → fingerprint 变 → 自然重试。**外部已是天然重试机制**。

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
// 1. 创建 service（注入 notifications.Publisher 用于 cache 变化通知；不订阅事件流）
catalogService := catalogapp.NewService(notifPublisher, log)

// 2. 后置注入 generator（生成器自身依赖 model picker / keys / llm factory）
catalogService.SetGenerator(catalogapp.NewLLMGenerator(modelPicker, keyProvider, llmFactory, log))

// 3. 注册 sources
catalogService.RegisterSource(forgeService.AsCatalogSource())
catalogService.RegisterSource(mcpService.AsCatalogSource())
catalogService.RegisterSource(skillService.AsCatalogSource())
// subagent 不注册——Subagent tool 自身 description 已覆盖 subagent 类型说明

// 4. 注入 chat
chatService.SetSystemPromptProvider(catalogService)

// 5. 启动（pollLoop 在自己 goroutine 起）
catalogService.Start(ctx)
```

**source 注册顺序无要求**——polling 自己会把 source 都拉一遍；注册完后启动 polling 即可。

---

## 9. HTTP API

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/catalog` | debug / 调试视图——返当前 cache 内容（含 summary + coverage）|
| `POST /api/v1/catalog:refresh` | 手动触发 refresh（绕过 1s polling 间隔，强制立即跑一次 tryRefresh）|

**:refresh** 主要给 testend 调试 / 用户在 UI 点"立即更新"用。

**没有 routing-hints 端点**——路由提示由 generator LLM-gen 时直接写进 summary，用户想影响路由 → 编辑源头 forge/skill/mcp 的 description。

---

## 10. 错误码 + Notifications

### 10.1 Sentinel → HTTP 映射

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `catalogdomain.ErrCoverageIncomplete` | (不到 handler) | — | LLM attempt 内部消化，触发 mechanicalFallback |
| `catalogdomain.ErrGenerationFailed` | (不到 handler) | — | 同上，内部消化 |
| `catalogdomain.ErrAllSourcesFailed` | 503 | `CATALOG_ALL_SOURCES_FAILED` | `Service.Refresh` 全部 source 报错时触发；HTTP `:refresh` 端点上抛 503，errmap 已登记 |

`ErrCoverageIncomplete` / `ErrGenerationFailed` 永远不到 handler。`ErrAllSourcesFailed` 是 catalog 唯一直达 errmap 的 sentinel——表达"派生 cache 拉数据全失败"的对外失败语义。

### 10.2 Notifications

每次 `Service.Refresh` 真更新 cache（fingerprint 变化）时 publish 一条 `catalog` 通知：

```json
{
  "type": "catalog",
  "id":   "*",
  "data": {
    "fingerprint": "<sha256-hex>",
    "version":     17,
    "generatedAt": "2026-05-09T13:42:00Z"
  }
}
```

- `id="*"`：catalog 是单例，没有 per-entity ID
- 同 fingerprint 短路时**不** publish（这是 99% 的 tick）
- 前端接收后可调 `GET /api/v1/catalog` 拉最新内容

详 [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) 的 notifications 协议章。

---

## 11. 测试覆盖 ✅

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| domain | `internal/domain/catalog/catalog_test.go` | 5 | Catalog JSON 全 7 字段 round-trip / Granularity String() + 枚举值 pin (PerItem=0 是新 source 安全默认) / Item JSON + Category omitempty / sentinel 唯一性 + 'catalog: ' 前缀审计 |
| app/catalog | `internal/app/catalog/catalog_test.go` | 18 | NewHasEmptyCache / RegisterSourceConcurrent (并发安全) / Refresh empty/nil-gen mech-fallback/wired-gen LLM-path/gen-error-fallback/all-sources-fail-keeps-cache/partial-failure-isolation / Fingerprint 短路 + 描述变 trigger / pollLoop FiresAtLeastOnce (20ms 间隔) / TryRefresh BusyGuard SkipsConcurrent (slowGenerator entered2=0) / Start LoadsExistingCache (Version 7 → nextVersion=8) / Start CorruptCacheMovedToBak / Refresh PersistsToDisk / Fingerprint stable shuffle + changes on description + ignores ID-only |
| app/catalog/generator | `internal/app/catalog/generator_test.go` | 8 | buildPrompt contains all items + granularity hints / NewLLMGenerator nil log OK / 单次 attempt JSON parse 路径（屎山拯救计划 #7 后保留 8 条；retry-loop / missing-hint / coverage 校验相关测试已删） |
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
