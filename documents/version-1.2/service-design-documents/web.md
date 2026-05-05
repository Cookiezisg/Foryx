# Web Tools — V1.2 详设计

**Phase**：5（System Tool 第二代 web 批次）
**状态**：✅ 实现完成（2026-05-04，W1-W4）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §S18 — Tool 接口规约
- [`./chat.md`](./chat.md) §4.4 — 系统工具完整目录
- [`./model.md`](./model.md) — `web_summary` scenario 定义
- 实现包：`backend/internal/app/tool/web/`

---

## 1. 一句话

LLM 上网两件套：**WebFetch**（抓 URL → LLM 摘要 → 返答案）+ **WebSearch**（3 层 fallback 公网搜索，零用户配置）。两者共享 SSRF 守卫（拒 loopback / 私网 / link-local + **逐跳重定向校验**）+ 30 秒墙钟。WebFetch 走 `web_summary` 模型场景（未配则透明 fallback 到 chat 场景）。

---

## 2. 端到端推演（设计原则 #5）

### WebFetch 路径

```
触发源：LLM 调 WebFetch(url, prompt)
  → ValidateInput: url + prompt 非空 / scheme ∈ {http,https}
  → Execute:
      url.Parse → guardHostname(host)            // SSRF 守卫（DNS rebinding 防御）
      fetchContent(ctx, url):
        Tier 1: fetchViaJina(jinaEndpoint+url) → 干净 markdown
        Tier 2 (Jina 失败 / 非 ctx 取消): fetchDirect(url) → 原始 HTML
        // fetchClient 带 CheckRedirect = ssrfCheckRedirect → 每跳重新校验
      content 截到 1 MiB
      summarise(ctx, url, prompt, content):
        llmclient.ResolveForWebSummary → bundle (web_summary 场景，不通则 chat fallback)
        Generate(prompt + 内容片段)
      → tool_result：摘要文本
```

### WebSearch 路径

```
触发源：LLM 调 WebSearch(query)
  → ValidateInput: query 非空 / limit ≥0
  → 3 层 fallback ladder:
      Tier 1: SearXNG 公共池（随机洗牌 + JSON 解析） — 任一实例返非空即用
      Tier 2: Bing HTML 抓（www.bing.com） — html visitor 解析 b_algo li
      Tier 3: Bing CN HTML 抓（cn.bing.com） — 大陆兜底
      每后端 10 秒墙钟；任一 tier 返非空 results 即终止
  → JSON: {query, source(searxng|bing|bing_cn), results[{title,url,snippet}], truncated}
```

**端到端跨 domain 依赖**：
- `pkg/llmclient.ResolveForWebSummary`（仅 WebFetch）：解析 `web_summary` model scenario，未配则 fallback `PickForChat`
- `domain/model.ScenarioWebSummary` + `ModelPicker.PickForWebSummary`（W1 模型层支持）
- `domain/apikey.KeyProvider`（解析 API key）
- `infra/llm.Factory.Build`（构造 LLM client）
- 第三方：`golang.org/x/net/html`（Bing 解析，唯一第三方依赖）
- env: `JINA_API_KEY`（可选，提速率档）/ `FORGIFY_SEARXNG_INSTANCES`（可选 SearXNG 实例池覆盖）
- 无 DB / SSE / HTTP API

---

## 3. 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| WebFetch 抓取策略 | **两段：Jina r.jina.ai → 直 GET fallback** | Jina 把任意网页转干净 markdown（免费层无 key），直 GET 兜底 Jina 限流 / down |
| WebFetch 摘要 LLM | **新增 `web_summary` scenario**，未配 fallback `chat` 场景 | 抓到的网页可能很长（1MB cap），用户可指定省钱模型（如 4o-mini）；不强制配置（透明 fallback）|
| WebSearch BYOK 策略 | **不要 BYOK**——3 层公共后端 fallback | 单用户本地不强制配 Tavily / Bing API key；维护风险换易用性（决策 D8，详 02-tools-deep/04-web.md）|
| WebSearch SearXNG 池 | 精选 4 实例 + 随机洗牌；env `FORGIFY_SEARXNG_INSTANCES` 覆盖 | 公共池常变；用户可指自家实例稳定运行；随机分散负载 |
| WebSearch Bing 解析 | **html visitor**（`x/net/html`）非 regex | Bing 偶尔加包装 div / 改属性顺序；regex 容易失效；visitor 跟随 `<li class="b_algo">` 子树更稳健 |
| Bing snippet fallback | b_caption 缺失时取 `<li>` 内首个 `<p>` | 实测 Bing 偶尔丢 b_caption 包装 |
| SSRF 守卫策略 | **解析所有 IP，任一禁区即拒**（DNS rebinding 防御）+ **逐跳重定向校验** | 单纯入口校验会被 302→localhost 绕过（**Tool 自检 batch 1 修的真 bug**）；现 `fetchClient.CheckRedirect = ssrfCheckRedirect` 每跳重跑 |
| 重定向跳数上限 | 10 | Go 默认值；超过即 `stopped after 10 redirects` |
| 单请求 byte cap | 1 MiB | 几乎覆盖所有文章型页面；摘要 LLM 的 token 成本可控 |
| 单后端超时 | WebFetch 30s / WebSearch 10s × 3 后端 | 30s 单 fetch 给慢博客留空间；search 3 × 10s = 30s 最坏（适配 chat tool 预算）|
| User-Agent | Chrome 桌面 UA | Bing 对空 UA / curl UA 返更少结果 / 403 |
| Image / PDF 抓取 | **v1 不实现**——仅 markdown / 文本 | description 不写未实现内容 |

---

## 4. 工具规约

### 4.1 WebFetch（`backend/internal/app/tool/web/fetch.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `url` | string | ✅ | 绝对 http/https URL |
| `prompt` | string | ✅ | 摘要指令（"概括要点"/"列出 API"等）|

**返回**：摘要 LLM 的回答字符串。

**特殊情况**：
- Validate 失败 → Go err（chat 转 tool_result）
- SSRF 拒 → `Refusing to fetch loopback address: 127.0.0.1`（或 private/link-local/unspecified/multicast 对应文案）
- 重定向到禁区 → `Failed to fetch <url>: redirect blocked: Refusing to fetch loopback...`
- 超时 / 网络错 → `Failed to fetch <url>: <err>`
- 双 tier 都失败 → 同上（最后一个 err）
- 空 body → `Fetched <url> but body was empty.`
- 摘要 LLM 失败 → `Summarisation failed (<err>). Raw content (first 4 KB):\n\n<truncated>`（兜底返原文 4KB 让 LLM 不至于完全没信息）

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=false`（**网络工具不碰文件系统**）

**ValidateInput** sentinels：
- `ErrEmptyURL` / `ErrEmptyPrompt`
- `ErrUnsupportedScheme` — 仅允许 http/https（拒 file:// / ftp:// / gopher://，扩大 SSRF 攻击面）

### 4.2 WebSearch（`search.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `query` | string | ✅ | 搜索词 |
| `limit` | number | | 默认 10；硬上限 30 |

**返回**（JSON）：
```json
{
  "query": "golang",
  "source": "searxng",
  "results": [
    {"title": "Go", "url": "https://go.dev", "snippet": "Build simple..."}
  ],
  "truncated": false
}
```

- `source`：`"searxng"` / `"bing"` / `"bing_cn"` —— 让 LLM 知道走的哪 tier
- `truncated`：`true` 表示原始结果数 > limit
- 全部 tier 失败 → `All search backends failed. Last error: <err>`
- 全部 tier 零结果 → `No results for "<query>" across SearXNG, Bing, and Bing CN.`

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=false`

**ValidateInput** sentinels：
- `ErrEmptyQuery` — query 缺 / 空 / 仅空白
- limit < 0 → `errors.New("limit must be non-negative")`

### 4.3 WebTools 工厂

```go
// app/tool/web/web.go
func WebTools(picker modeldomain.ModelPicker, keys apikeydomain.KeyProvider, factory *llminfra.Factory) []toolapp.Tool {
    return []toolapp.Tool{
        newWebFetch(picker, keys, factory),  // WebFetch 需要摘要 LLM 的依赖
        newWebSearch(),                       // WebSearch 自包含（10s client + SearXNG 池）
    }
}
```

调用方按 §S13 嵌套子包别名规则导入为 `webtool`。

---

## 5. 实现要点

### 5.1 SSRF 守卫（`guardHostname` + `classifyIP`）

**两层防御**：

```go
func guardHostname(host string) string {
    // (1) 字面 loopback 名拒
    if host ∈ {"localhost", "ip6-localhost", "ip6-loopback"} { return reject }

    // (2) 裸 IP 字面：classifyIP 检测
    if ip := net.ParseIP(host); ip != nil {
        return classifyIP(ip)  // loopback / private / link-local / unspecified / multicast
    }

    // (3) 域名：解析所有 IP，任一禁区即拒（DNS rebinding 策略级防御）
    ips, _ := net.LookupIP(host)
    for _, ip := range ips {
        if reason := classifyIP(ip); reason != "" { return reason }
    }
    return ""  // safe
}
```

**已知局限**：不绑定 IP 到 TCP 连接（pinning 需要自定义 Dialer，复杂度大）。带"公网 + 私网双答案"的恶意域名挡住了；高速 DNS 翻转攻击（请求时翻转 IP）理论上仍可能。

### 5.2 重定向逐跳校验（**Tool 自检 batch 1 加固**）

```go
var fetchClient = &http.Client{
    Timeout:       fetchTimeout,
    CheckRedirect: ssrfCheckRedirect,
}

func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
    if len(via) >= 10 { return errors.New("stopped after 10 redirects") }
    if reason := guardHostname(req.URL.Hostname()); reason != "" {
        return fmt.Errorf("redirect blocked: %s", reason)
    }
    return nil
}
```

**为啥重要**：`http.Client` 默认跟随 302/301 不做任何安全校验。修复前公网 URL → 302 → `http://localhost` 能绕过入口的 `guardHostname`。Tool 自检 batch 1 加 CheckRedirect 后每跳重跑（详 fetch_test.go 4 个回归测试）。

### 5.3 摘要 LLM 解析（`llmclient.ResolveForWebSummary`）

```go
// pkg/llmclient/llmclient.go
func ResolveForWebSummary(ctx, picker, keys, factory) (Bundle, error) {
    // 优先 web_summary scenario
    bundle, err := resolveScenario(ctx, picker.PickForWebSummary, keys, factory)
    if err == nil { return bundle, nil }
    if !errors.Is(err, model.ErrNotConfigured) { return bundle, err }

    // 透明 fallback 到 chat 场景
    return Resolve(ctx, picker, keys, factory)
}
```

**为啥透明 fallback**：用户首次用 WebFetch 时大概率没专门配 web_summary。报错会破坏 UX。fallback 让"开箱即用"成立；用户**之后**配 web_summary（更便宜的 4o-mini）就自动切。

### 5.4 SearXNG 池随机洗牌

```go
pool := append([]string(nil), t.instances...)
rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
for _, base := range pool {
    if ctx.Err() != nil { return nil, ctx.Err() }
    results, err := t.querySearXNG(ctx, base, query)
    if err == nil && len(results) > 0 { return results, nil }
}
```

**为啥洗牌**：顺序遍历会一直打第一个实例；公共实例都是志愿者跑的，分散负载是公民义务。

### 5.5 Bing HTML 解析（`search_bing.go`）

```go
// 跟随 <li class="b_algo"> 子树（不下钻已找到的 result block）
walkBing(doc, &out)

// 每块提取：
//   <h2><a href> → URL + Title
//   <div class="b_caption"><p> → Snippet
//   缺 b_caption 时 fallback：<li> 内首个 <p>
```

**`hasClass` / `findFirstByTag` / `findFirstByClass` / `textOf` / `collapseSpaces`** 是本地小 helper（不引 `x/net/html/atom` 等更重依赖）。

`collapseSpaces` 用 `strings.Fields(s) + strings.Join(" ")` 把 `\n`/tab/多空白压成单空格——Bing snippet 常含怪空白。

---

## 6. 安全边界

| 防线 | 覆盖 | 局限 |
|---|---|---|
| **Schema scheme 校验** | 拒 file:// / ftp:// / gopher:// | data: / blob: 也不允许（默认拒非 http/https）|
| **SSRF guardHostname** | loopback / 私网 RFC1918 / link-local / unspecified / multicast | 不 pin IP 到 TCP 连接（可被高速 DNS 翻转攻击；非威胁模型核心）|
| **CheckRedirect 逐跳** | 防 302→localhost 绕过（**batch 1 修的真 bug**）| 跳数硬上限 10 |
| **byte cap 1 MiB** | 防摘要 LLM token 爆炸 + 内存 OOM | 大文章长尾被截断（LLM 看到部分内容也能摘要）|
| **30 秒 fetchTimeout** | 防慢服务器把 ReAct 循环卡分钟 | 真大文件下载会被砍 |
| **web_summary fallback** | 用户没配也能用 | 用 chat 模型摘要可能贵——可以日志提示用户配 web_summary 节省 |
| **WebSearch 公共后端** | 零用户配置 | SearXNG 实例可能 down / Bing 反爬 → 3 层 fallback 提高可用性 |
| **JINA_API_KEY 是 env 不是 BYOK config** | 不强制；用户想升速率档自己设 | 没设走免费层（够用）|

---

## 7. 测试覆盖

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| WebFetch | `backend/internal/app/tool/web/fetch_test.go` | 24 | identity / 静态 metadata / schema / Validate × 5 / classifyIP 公网 + 5 类禁区 / guardHostname 名 + IP / Jina 优先 + 直 GET fallback / 双失败 / 取消 / byte cap / Execute SSRF short-circuit × 2 / **CheckRedirect 拒 loopback / 拒私网 / 公网通过 / 10 跳上限**（batch 1 加固）/ truncate / buildSummaryPrompt |
| WebSearch | `search_test.go` | 21 | identity / 静态 metadata / schema / Validate × 2 / normalize × 2 / env 覆盖 + fallback / 3 tier 端到端 × 4 / limit truncated / ctx cancel / Bing HTML 解析 + b_caption fallback + 空 doc / 测试 helper |
| Pipeline | `backend/test/web/` | 2 场景 | LLM ↔ tool 端到端：WebFetchBlocksLoopback / WebSearchRejectsEmptyQuery（11s）|

合计 **45 单测 + 2 pipeline 场景**。

---

## 8. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **model** | WebFetch 依赖 `web_summary` scenario；W1 加 `domain/model.ScenarioWebSummary` 常量 + `ModelPicker.PickForWebSummary` 接口方法 |
| **apikey** | 同 chat：通过 `KeyProvider.ResolveCredentials` 拿摘要 LLM 的凭据 |
| **infra/llm** | WebFetch 用 `llminfra.Factory.Build` 构造摘要 client，`Generate` helper 跑非流式调用 |
| **pkg/llmclient** | 新增 `ResolveForWebSummary(ctx, picker, keys, factory)` 三段舞 + 透明 fallback chat |
| **chat** | 通过 ReAct loop 调度；WebFetch / WebSearch 都 IsReadOnly=true，可同 execution_group 并行 |
| **events / SSE** | 无 — 结果通过 chat.message tool_result block 推流 |
| **errmap** | 无登记 — 错误以友好字符串返 LLM |

---

## 9. 演化方向

- **WebFetch 缓存**：CC 有 15min cache（独立 context 里跑摘要避免主对话污染）；Forgify v1 不做（cache invalidation 复杂）
- **WebFetch 多模态**：未来支持图片识别（OCR / 视觉模型）；当前仅文本 / markdown
- **WebSearch BYOK 升级**：用户希望更稳的话，加 Tavily / Brave Search API 作为 Tier 0（在 SearXNG 之前），有 API key 才启用
- **WebSearch 缓存 + dedup**：相同 query 短期内用缓存
- **Bing 反爬升级时**：Tier 2/3 失效需替换（DuckDuckGo HTML 抓 / Searx 自建 / 等）
- **SSRF IP-pinning**：自定义 Dialer 把解析到的 IP 钉到 TCP 连接，关上高速 DNS 翻转的洞（需写 `net.Dialer.Resolver` + 自定义解析器）
- **Image fetch + Vision 摘要**：URL 是图片时走 Vision 模型摘要
