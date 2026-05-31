# 04 — Web Tools 深挖

> 02-tools-deep 系列第四篇。
> ✅ **WebFetch** — Claude Code 标准 tool（v2.1.14 起描述长期稳定）
> ✅ **WebSearch** — Claude Code 标准 tool（v2.1.120）；**仅美国可用**

## 信息源

- **主源**：
  - [`tool-description-webfetch.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-webfetch.md) — ccVersion **2.1.14**（长期稳定）
  - [`tool-description-websearch.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-websearch.md) — ccVersion **2.1.120**
- **副源**：[code.claude.com tools-reference](https://code.claude.com/docs/en/tools-reference)
- **写作日期**：2026-05-03

---

## WebFetch

### 1. Description 原文（Piebald v2.1.14）

> - Fetches content from a specified URL and processes it using an AI model
> - Takes a URL and a prompt as input
> - Fetches the URL content, converts HTML to markdown
> - Processes the content with the prompt using a small, fast model
> - Returns the model's response about the content
> - Use this tool when you need to retrieve and analyze web content
>
> Usage notes:
>   - IMPORTANT: If an MCP-provided web fetch tool is available, prefer using that tool instead of this one, as it may have fewer restrictions.
>   - The URL must be a fully-formed valid URL
>   - HTTP URLs will be automatically upgraded to HTTPS
>   - The prompt should describe what information you want to extract from the page
>   - This tool is read-only and does not modify any files
>   - Results may be summarized if the content is very large
>   - Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL
>   - When a URL redirects to a different host, the tool will inform you and provide the redirect URL in a special format. You should then make a new WebFetch request with the redirect URL to fetch the content.
>   - For GitHub URLs, prefer using the gh CLI via Bash instead (e.g., gh pr view, gh issue view, gh api).

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["url", "prompt"],
  "properties": {
    "url": {
      "type": "string",
      "format": "uri",
      "description": "The URL to fetch content from"
    },
    "prompt": {
      "type": "string",
      "description": "The prompt to run on the fetched content"
    }
  }
}
```

✅ 字段名 (`url` / `prompt`) 来自 wong2 gist + 官方 docs。

**注意**：schema 极简——仅 2 字段。所有"行为"都在描述里 + framework 兜底。

### 3. 算法行为

**Pipeline** ✅
```
LLM 提交 {url, prompt}
  ↓
1. URL 校验：必须是合法 URL；HTTP → HTTPS upgrade
  ↓
2. Cache 查询：URL 是否在 15 分钟缓存内？命中则直接拿 markdown
  ↓ (miss)
3. HTTP GET：拿 HTML（带 User-Agent、follow redirects 但跨 host 中止）
  ↓
4. HTML → Markdown 转换（自带库 / Jina Reader 等）
  ↓
5. 小模型调用：以 prompt 为 instruction，markdown 为 context，请求摘要 / 提取
  ↓
6. 返回小模型的输出（不是原始 markdown）
```

**关键设计原则** ✅

- **独立 context window**——主对话 LLM 看不到 HTML / markdown 全文，只看到小模型的最终输出。**这避免了 web 内容污染主 context budget**——一个 50KB 的 HTML 不会消耗 50KB 主 token
- **小模型 = Haiku-tier**（CC 内部）；Forgify 可用 chat model 或单独配 "small" 场景
- **15 分钟 cache** 按 URL 键值
- **跨 host 重定向不自动跟**——返特殊提示要 LLM 主动重发到新 URL（防 SSRF）
- **HTTP auto upgrade HTTPS**——不接受裸 HTTP

**重定向提示形式**（推测）
```
Redirect detected: <original_url> → <new_url> (different host)
You must call WebFetch again with the new URL to fetch the content.
```

**MCP web fetch 优先**——CC 主动让位给 MCP-provided web fetch tool（如果用户装了的话），因为 MCP 实现可能有更松的限制。Forgify 现在没 MCP，等 Phase 5 上 mcpserver domain 后这条就生效。

**GitHub 例外**——CC 明确建议 GitHub URL 用 `gh` CLI 而非 WebFetch，因为 `gh` 能拿 issue body / PR diff 等结构化数据。Forgify 暂无 `gh` 集成，按 URL 走即可。

**并发**
- `IsReadOnly() = true`
- 并发安全：**是**（不同 URL 互不影响；同 URL 命中 cache）—— LLM 同 turn 多个 WebFetch 应放同 `execution_group`

### 4. 已知 bugs / edge cases

- **缓存粒度只到 URL** —— `?param=A` vs `?param=B` 视为不同 URL，OK；但同 URL 的 prompt 不同时**不区分**——第二次 prompt 不重抓 HTML 是正确的（省成本），但每次仍重新调小模型
- **JS-render 页面**——CC 的 fetcher 是纯 HTTP GET，不执行 JS。SPA / 重 JS 站点（React app shell）只能拿到空骨架。CC 没文档化此限制
- **大文件**：HTML > 1MB 时小模型 input 可能超限；CC "Results may be summarized" 是模糊兜底
- **rate limiting**：网站 429 会怎样？CC 推测是直接 surface 错误给 LLM，让 LLM 决定重试

### 5. 输出格式给 LLM

成功：
```
<小模型基于 prompt 总结的内容；可以是任意结构，因为是 free-form LLM 输出>
```

跨 host 重定向：
```
Redirect to different host detected. Please retry with: https://new-host.com/path
```

错误：
```
Failed to fetch <url>: <reason>
```

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口

```go
type WebFetch struct {
    httpClient *http.Client
    converter  HTMLToMarkdown        // 接口：HTML → Markdown
    summarizer Summarizer            // 接口：(prompt, content) → string
    cache      *URLCache             // 15-min TTL，URL → markdown
}

func (t *WebFetch) Name() string                  { return "WebFetch" }
func (t *WebFetch) IsReadOnly() bool              { return true }
func (t *WebFetch) NeedsReadFirst() bool          { return false }
func (t *WebFetch) RequiresWorkspace() bool       { return false }
```

#### 6.2 HTML → Markdown 选择

| 选项 | 优 | 劣 |
|---|---|---|
| **Jina Reader** (`https://r.jina.ai/<URL>`) | 一行 HTTP；外部服务擦掉 ad / nav / sidebar；免费配额够用 | 依赖外部服务（可能被墙 / rate limit）；隐私（请求过 jina）|
| **`html-to-markdown` 库**（Go 包） | 完全本地；无外部依赖 | 需自己写 boilerplate 清理（剥 nav / ad / footer）；JS-render 页面同样吃瘪 |
| **`goquery` + 自定 parser** | 最灵活 | 工作量大 |

**建议**：v1 用 **Jina Reader 优先 + html-to-markdown fallback**。Jina Reader 在内地访问受限时自动降级。

#### 6.3 Summarizer：新增 `web_summary` scenario

CC 用"小、快模型"（Haiku）。Forgify 走 **scenario-based model 配置**——在 `model` domain 加新 scenario `"web_summary"`，让用户单独配便宜快模。

**为何不复用 chat 模型**：
- 每次 WebFetch 烧主 chat model 的 token——如果用户配的是 reasoning 模型（o1 / DeepSeek-R1），WebFetch 会慢且贵
- Forgify 的 model domain 本身设计就是"按场景配不同 model"——`web_summary` 是这个方向的自然扩展
- 推荐配置：Haiku / Gemini Flash / DeepSeek-V3 等便宜快模

**实现要点**：

```go
// model domain：scenarios 白名单加一项
const ScenarioWebSummary = "web_summary"

// modelapp.Service.PickForScenario(ctx, scenario string) 已有泛化接口
// （chat 用 "chat"；test_case 用 "test_case_generation"）
```

WebFetch tool 通过 `picker.PickForScenario(ctx, "web_summary")` 拿模型；用户没配该 scenario 时**降级到 chat scenario**：

```go
bc, err := llmclientpkg.ResolveScenario(ctx, picker, keys, factory, modeldomain.ScenarioWebSummary)
if errors.Is(err, modeldomain.ErrScenarioNotConfigured) {
    // 优雅降级
    bc, err = llmclientpkg.ResolveScenario(ctx, picker, keys, factory, modeldomain.ScenarioChat)
}
if err != nil {
    return fmt.Sprintf("Cannot summarize fetched content: no model configured (%v)", err), nil
}
```

UI 提示：用户进 model 配置页时，能看到 `web_summary` 是 "推荐配 Haiku-tier 的便宜模型；不配会降级到 chat 模型，可能慢"。

#### 6.4 Cache 实现

```go
type URLCache struct {
    mu      sync.RWMutex
    entries map[string]cacheEntry
    ttl     time.Duration
}

type cacheEntry struct {
    markdown  string
    fetchedAt time.Time
}

func (c *URLCache) Get(url string) (string, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    e, ok := c.entries[url]
    if !ok || time.Since(e.fetchedAt) > c.ttl {
        return "", false
    }
    return e.markdown, true
}

func (c *URLCache) Put(url, markdown string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.entries[url] = cacheEntry{markdown: markdown, fetchedAt: time.Now()}
}

// goroutine: 每 5 分钟扫一遍，清过期项
func (c *URLCache) gcLoop(ctx context.Context) {
    t := time.NewTicker(5 * time.Minute)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-t.C:
            c.mu.Lock()
            for url, e := range c.entries {
                if time.Since(e.fetchedAt) > c.ttl {
                    delete(c.entries, url)
                }
            }
            c.mu.Unlock()
        }
    }
}
```

挂 chat.Service 上，进程级共享一份。15 分钟 TTL 同 CC。

#### 6.5 关键代码片段

```go
func (t *WebFetch) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        URL    string `json:"url"`
        Prompt string `json:"prompt"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("WebFetch.Execute: %w", err)
    }

    u, err := url.Parse(args.URL)
    if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
        return fmt.Sprintf("Invalid URL: %s", args.URL), nil
    }
    // HTTP → HTTPS 自动升级
    if u.Scheme == "http" {
        u.Scheme = "https"
        args.URL = u.String()
    }

    // 1. cache 查
    md, ok := t.cache.Get(args.URL)
    if !ok {
        // 2. 抓 HTML（不跟跨 host redirect）
        originalHost := u.Host
        client := &http.Client{
            CheckRedirect: func(req *http.Request, via []*http.Request) error {
                if req.URL.Host != originalHost {
                    return fmt.Errorf("CROSS_HOST_REDIRECT: %s", req.URL.String())
                }
                return nil
            },
            Timeout: 30 * time.Second,
        }
        resp, err := client.Get(args.URL)
        if err != nil {
            // 跨 host 重定向特殊处理
            if strings.Contains(err.Error(), "CROSS_HOST_REDIRECT:") {
                redirectURL := strings.Split(err.Error(), "CROSS_HOST_REDIRECT: ")[1]
                return fmt.Sprintf(
                    "Redirect to different host detected. Please retry WebFetch with: %s",
                    redirectURL,
                ), nil
            }
            return fmt.Sprintf("Failed to fetch %s: %v", args.URL, err), nil
        }
        defer resp.Body.Close()
        if resp.StatusCode >= 400 {
            return fmt.Sprintf("HTTP %d for %s", resp.StatusCode, args.URL), nil
        }
        body, _ := io.ReadAll(resp.Body)

        // 3. HTML → Markdown
        md, err = t.converter.Convert(string(body))
        if err != nil {
            return fmt.Sprintf("HTML conversion failed: %v", err), nil
        }
        t.cache.Put(args.URL, md)
    }

    // 4. 小模型摘要
    summary, err := t.summarizer.Summarize(ctx, args.Prompt, md)
    if err != nil {
        return fmt.Sprintf("Summarization failed: %v", err), nil
    }
    return summary, nil
}
```

#### 6.6 Validate / Permission

| Sentinel | 触发 |
|---|---|
| `ErrInvalidURL` | URL 不是合法 http(s) URL |
| `ErrEmptyPrompt` | prompt == "" |

permission：默认 Allow（read-only 操作；用户隐私意识用本地启停 WebFetch 控制）。

#### 6.7 测试要点

- 普通 https URL → 抓 + 摘要
- HTTP URL → 自动升 HTTPS 后抓
- 同 URL 二次调用 → 命中 cache（验证 cache 工作）
- 不同 prompt 同 URL → cache 命中 但每次重摘要
- 跨 host 重定向 → 提示 LLM 重 fetch
- 404 → 错误消息
- 超 30s timeout → 错误
- HTML 解析失败 → 错误（content-type text/plain 等）
- prompt 空 → ErrEmptyPrompt

---

## WebSearch

### 1. Description 原文（Piebald v2.1.120）

> - Allows Claude to search the web and use the results to inform responses
> - Provides up-to-date information for current events and recent data
> - Returns search result information formatted as search result blocks, including links as markdown hyperlinks
> - Use this tool for accessing information beyond Claude's knowledge cutoff
> - Searches are performed automatically within a single API call
>
> CRITICAL REQUIREMENT - You MUST follow this:
>   - After answering the user's question, you MUST include a "Sources:" section at the end of your response
>   - In the Sources section, list all relevant URLs from the search results as markdown hyperlinks: `[Title](URL)`
>   - This is MANDATORY - never skip including sources in your response
>   - Example format:
>
>     [Your answer here]
>
>     Sources:
>     - [Source Title 1](https://example.com/1)
>     - [Source Title 2](https://example.com/2)
>
> Usage notes:
>   - Domain filtering is supported to include or block specific websites
>   - Web search is only available in the US
>
> IMPORTANT - Use the correct year in search queries:
>   - The current month is **${CURRENT_MONTH_YEAR}**. You MUST use this year when searching for recent information, documentation, or current events.
>   - Example: If the user asks for "latest React docs", search for "React documentation" with the current year, NOT last year

#### 占位符

| 占位符 | 值 |
|---|---|
| `CURRENT_MONTH_YEAR` | 当前年月，如 "May 2026"——CC 在 system prompt 注入时实时填 |

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["query"],
  "properties": {
    "query": {
      "type": "string",
      "description": "The search query"
    },
    "allowed_domains": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Restrict results to these domains (e.g. [\"docs.python.org\", \"go.dev\"])"
    },
    "blocked_domains": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Exclude results from these domains"
    }
  }
}
```

✅ 来自 wong2 gist + 官方 docs；3 字段。

### 3. 算法行为

**搜索源** ⚠️
- CC 内部用某个搜索 API（推测 Bing or Google）；具体 provider 未公开
- 仅美国可用——大概率 Anthropic-hosted 走自家代理 + IP 限制

**单次调用** ✅
- "Searches are performed automatically within a single API call"——意思是 LLM 一次 tool call = 一次 search engine query，不在内部多轮 refine

**Sources 强制约定** ✅
- description 显式 MANDATORY：返回结果后**必须**在响应末尾加 `Sources:` 段
- 这是 **prompt-side enforcement**——framework 不强制 / 不验证；纯靠 LLM 自觉
- 设计意图：让用户能验证信息来源，避免 LLM 编造

**当前年份注入** ✅
- description 含 `${CURRENT_MONTH_YEAR}` 实时填——避免 LLM 用过期年份搜（"latest React docs 2024" → 应该 "latest React docs 2026"）
- 这是 **anti-hallucination prompt engineering**——LLM 不知道当前日期，强行注入

**Domain filtering** ✅
- 支持 allow / block list；SearXNG/Tavily/Brave 等 API 都原生支持；本地 Bing scrape fallback 在结果上做后置过滤

**并发**
- `IsReadOnly() = true`
- 并发安全：**是**——LLM 同 turn 多个 WebSearch 应放同 `execution_group`

### 4. 已知 edge cases

- **US-only 限制**：不在美国的用户用不了，是 CC 的 1A 限制
- **Sources 段被 LLM 跳过**：prompt-side 约束本质上是 best-effort，弱模型可能漏（特别是 quantized 4o-mini 之类）
- **当前年份 stale**：如果 description 注入是构建期 hard-code 而非运行时，实际跑起来会过期。CC 推测是运行时填入

### 5. 输出格式给 LLM

CC 的输出是"search result blocks"，推测形如：
```
[1] How React Hooks Work (https://react.dev/learn/hooks)
    React Hooks let you use state and other React features without writing a class. ...

[2] React useState Hook Tutorial (https://react.dev/reference/react/useState)
    The useState hook is one of the most commonly used React hooks. ...

[3] ...
```

每条带 title / URL / snippet。LLM 自己决定深 fetch 哪个（用 WebFetch）。

### 6. Forgify Go 实现要点

#### 6.1 Backend 策略：3 层 fallback（**全免费、全开源、零配置**）

CC 自带搜索 + 仅美国 + SaaS 代付——Forgify 是本地 app 学不了。但完全 BYOK 用户体验差。最终方案：**多层 fallback，质量降级但永远有响应**。

```
LLM call WebSearch
  ↓
Primary: SearXNG 公共实例池（轮 5 个 well-known 实例，随机起点）
  ↓ 全部失败 / timeout
Fallback 1: 直接 HTTP GET https://www.bing.com/search?q=... 解析 HTML
  ↓ 失败（GFW / captcha）
Fallback 2: 直接 HTTP GET https://cn.bing.com/search?q=... 解析 HTML（覆盖中国用户）
  ↓ 全失败
返回友好错误："WebSearch backends all unavailable; please try WebFetch with a specific URL"
```

| 层 | Backend | 优 | 劣 |
|---|---|---|---|
| Primary | SearXNG 公共实例池 | 开源；aggregator 质量好；零配置 | 个别实例不稳；中国部分访问受限 |
| Fallback 1 | Bing HTML scraping (international) | 完全 stdlib HTTP；质量一般但够用 | 触发 captcha 后挂；中国 GFW |
| Fallback 2 | Bing China HTML scraping | 中国可访问；GFW 友好 | 内容审查；少量结果被过滤 |

**SearXNG 实例池（hardcoded，每次 shuffle 起点轮询）**：
```go
var searxInstances = []string{
    "https://searx.be",
    "https://priv.au",
    "https://searx.tiekoetter.com",
    "https://paulgo.io",
    "https://northboot.xyz",
}
```
未来可在 settings 加用户自定义 URL 覆盖（v2 advanced feature，v1 不做）。

#### 6.2 不需要 apikey domain

WebSearch v1 完全无 BYOK——不动 apikey/providers.go，不要 Tavily / Brave / Google CSE 配置。这是跟 LLM 模型 / 未来 advanced backend 的关键分界。

#### 6.3 Tool 接口

```go
type WebSearch struct {
    httpClient *http.Client
}

func (t *WebSearch) Name() string                  { return "WebSearch" }
func (t *WebSearch) IsReadOnly() bool              { return true }
func (t *WebSearch) NeedsReadFirst() bool          { return false }
func (t *WebSearch) RequiresWorkspace() bool       { return false }
```

#### 6.4 关键代码片段（3 层 fallback 实现）

```go
import "github.com/PuerkitoBio/goquery"

var searxInstances = []string{
    "https://searx.be",
    "https://priv.au",
    "https://searx.tiekoetter.com",
    "https://paulgo.io",
    "https://northboot.xyz",
}

type searchResult struct {
    Title   string
    URL     string
    Snippet string
}

func (t *WebSearch) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        Query           string   `json:"query"`
        AllowedDomains  []string `json:"allowed_domains"`
        BlockedDomains  []string `json:"blocked_domains"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("WebSearch.Execute: %w", err)
    }
    if args.Query == "" {
        return "Query is empty.", nil
    }

    // ── Layer 1: SearXNG pool ────────────────────────────────────────────
    if results, err := t.searchSearXNG(ctx, args.Query); err == nil && len(results) > 0 {
        return formatResults(results, args.AllowedDomains, args.BlockedDomains, ""), nil
    }

    // ── Layer 2: Bing international scraping ─────────────────────────────
    if results, err := t.scrapeBing(ctx, "https://www.bing.com/search", args.Query); err == nil && len(results) > 0 {
        return formatResults(results, args.AllowedDomains, args.BlockedDomains,
            "[Note: degraded backend (Bing scraping); SearXNG pool unreachable]"), nil
    }

    // ── Layer 3: Bing China scraping ─────────────────────────────────────
    if results, err := t.scrapeBing(ctx, "https://cn.bing.com/search", args.Query); err == nil && len(results) > 0 {
        return formatResults(results, args.AllowedDomains, args.BlockedDomains,
            "[Note: last-resort backend (Bing CN); previous backends unreachable]"), nil
    }

    return "WebSearch backends all unavailable. Please try WebFetch with a specific URL.", nil
}

// SearXNG: 轮 5 实例，随机起点
func (t *WebSearch) searchSearXNG(ctx context.Context, query string) ([]searchResult, error) {
    perm := rand.Perm(len(searxInstances))
    for _, idx := range perm {
        base := searxInstances[idx]
        u := fmt.Sprintf("%s/search?q=%s&format=json", base, url.QueryEscape(query))
        req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
        req.Header.Set("User-Agent", "Forgify/1.0 (+https://forgify.local)")
        resp, err := t.httpClient.Do(req)
        if err != nil { continue }
        if resp.StatusCode >= 400 { resp.Body.Close(); continue }
        var out struct {
            Results []struct {
                URL     string `json:"url"`
                Title   string `json:"title"`
                Content string `json:"content"`
            } `json:"results"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
            resp.Body.Close(); continue
        }
        resp.Body.Close()
        if len(out.Results) == 0 { continue }
        results := make([]searchResult, 0, len(out.Results))
        for _, r := range out.Results {
            results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
        }
        return results, nil
    }
    return nil, fmt.Errorf("all SearXNG instances failed")
}

// Bing scrape：解析 .b_algo 列表
func (t *WebSearch) scrapeBing(ctx context.Context, baseURL, query string) ([]searchResult, error) {
    u := fmt.Sprintf("%s?q=%s", baseURL, url.QueryEscape(query))
    req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36")
    resp, err := t.httpClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 { return nil, fmt.Errorf("HTTP %d", resp.StatusCode) }

    doc, err := goquery.NewDocumentFromReader(resp.Body)
    if err != nil { return nil, err }

    var results []searchResult
    doc.Find("li.b_algo").Each(func(i int, s *goquery.Selection) {
        title := strings.TrimSpace(s.Find("h2 a").Text())
        href, _ := s.Find("h2 a").Attr("href")
        snippet := strings.TrimSpace(s.Find(".b_caption p").Text())
        if title != "" && href != "" {
            results = append(results, searchResult{Title: title, URL: href, Snippet: snippet})
        }
    })
    if len(results) == 0 {
        return nil, fmt.Errorf("no results parsed from Bing HTML")
    }
    return results, nil
}

// 格式化 + allow/block domain 过滤
func formatResults(results []searchResult, allow, block []string, footer string) string {
    var sb strings.Builder
    n := 0
    for _, r := range results {
        if !domainPasses(r.URL, allow, block) { continue }
        n++
        fmt.Fprintf(&sb, "[%d] %s (%s)\n    %s\n\n", n, r.Title, r.URL, truncate(r.Snippet, 300))
        if n >= 10 { break }
    }
    if n == 0 {
        return "No results."
    }
    if footer != "" {
        sb.WriteString(footer)
    }
    return sb.String()
}

func domainPasses(rawURL string, allow, block []string) bool {
    u, err := url.Parse(rawURL)
    if err != nil { return false }
    host := u.Hostname()
    if len(allow) > 0 {
        ok := false
        for _, d := range allow {
            if strings.HasSuffix(host, d) { ok = true; break }
        }
        if !ok { return false }
    }
    for _, d := range block {
        if strings.HasSuffix(host, d) { return false }
    }
    return true
}
```

#### 6.5 Description 复刻

**Sources 后置约定**和**当前年份强调**两条都要复刻。当前年份用 Forgify 启动时拿 `time.Now().Format("January 2006")`，每次 buildSystemPrompt 时注入到 description——保证每次 chat req 都是新鲜的。

#### 6.6 Validate / Permission

| Sentinel | 触发 |
|---|---|
| `ErrEmptyQuery` | query == "" |

无 `ErrNoSearchProvider`——不用 BYOK 就不存在"没配 provider"的情形；最坏返"backends all unavailable"。

#### 6.7 测试要点

- 普通 query 命中 SearXNG → 正常返结果
- allow_domains 限定（仅 docs.python.org）→ 过滤
- block_domains 排除 → 过滤
- 0 results 友好消息
- mock SearXNG 全 timeout → 走 Bing fallback
- mock SearXNG + Bing 全 timeout → 走 Bing CN fallback
- mock 所有 backend timeout → 友好错误消息（不 panic）
- 模拟 Bing 返 HTML 结构变化（`b_algo` 失效）→ parser 不崩，fallback 下一层
- 真集成测试（`-tags=integration` opt-in）：跑一次真 SearXNG 验证

---

## 跨 tool 共享

- 都 `IsReadOnly = true`——LLM 把 WebFetch + WebSearch 放同 `execution_group` 即并行
- 都不需要 workspace 约束（网络资源不在文件系统域）
- WebFetch 的 cache 是进程级共享；不属于 AgentState
- WebSearch v1 不需要 apikey domain（无 BYOK；3 层 fallback 全免费开源）

---

## 总结：本批实施估时

| 工具 | 估时 | 难点 |
|---|---|---|
| WebFetch | 0.5 天 | Jina Reader 集成 + Summarizer 接口 + cache + 跨 host redirect 处理 |
| WebSearch | 0.6 天 | 3 层 fallback 编排（SearXNG 池 / Bing scrape / Bing CN scrape）+ goquery HTML 解析 + domain 后置过滤 |

**合计 ~0.9 天**。

---

## 信任度总结

- ✅ **多源确认**：WebFetch description 原文 + 2 字段 schema + 15min cache + HTTPS upgrade + 跨 host redirect 行为；WebSearch description 原文 + 3 字段 schema + Sources 后置 + 当前年份注入 + US-only
- ⚠️ **单源 / 推测**：CC 内部小模型确切是哪个（Haiku 推测）/ WebSearch 后端搜索引擎（Bing/Google 推测）/ HTML→Markdown 用什么库（Anthropic 内部）/ search result block 的精确 wire format
- ❌ **无法验证**：cache 实现的精确数据结构 / 当前年份注入的具体时机（启动期 vs 每次 turn）

deep-dive 期间 ⚠️ 项不影响 Forgify 实现，因为我们用自己的 Jina Reader（WebFetch）/ SearXNG 池+Bing scrape fallback（WebSearch）替代——只要保持 description 一致，LLM 行为就一致。
