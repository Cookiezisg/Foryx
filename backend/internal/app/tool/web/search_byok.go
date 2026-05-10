// search_byok.go — BYOK search backend clients (Brave / Serper / Tavily /
// Bocha). Each function receives (ctx, baseURL, key, query, limit) and
// returns []searchResult; HTTP plumbing + auth headers + JSON parsing all
// inline. WebSearch.Execute iterates these in apikeydomain
// .SearchProviderPriority order, skipping providers without configured
// keys, falling through on per-provider error.
//
// search_byok.go ——BYOK 搜索后端 client（Brave / Serper / Tavily / Bocha）。
// 每函数收 (ctx, baseURL, key, query, limit) 返 []searchResult；HTTP plumbing
// + 认证头 + JSON 解析全内联。WebSearch.Execute 按 apikeydomain
// .SearchProviderPriority 顺序遍历，跳过未配 key 的，per-provider 错误降级。
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// searchBrave queries Brave Search API. Endpoint: GET {baseURL}/web/search
// ?q={query}&count={limit}. Auth: X-Subscription-Token. Response shape:
// {"web":{"results":[{"title","url","description"}]}}.
//
// searchBrave 调 Brave Search API。响应形如
// {"web":{"results":[{"title","url","description"}]}}。
func (t *WebSearch) searchBrave(ctx context.Context, baseURL, key, query string, limit int) ([]searchResult, error) {
	u := fmt.Sprintf("%s/web/search?q=%s&count=%d",
		baseURL, url.QueryEscape(query), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("brave: build: %w", err)
	}
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("Accept", "application/json")
	body, err := t.doSearchHTTP(req, "brave")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("brave: parse: %w", err)
	}
	out := make([]searchResult, 0, len(resp.Web.Results))
	for _, r := range resp.Web.Results {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return out, nil
}

// searchSerper queries Serper.dev (Google search results proxy).
// Endpoint: POST {baseURL}/search. Auth: X-API-KEY. Response shape:
// {"organic":[{"title","link","snippet"}]}.
//
// searchSerper 调 Serper.dev（Google 结果代理）。响应形如
// {"organic":[{"title","link","snippet"}]}。
func (t *WebSearch) searchSerper(ctx context.Context, baseURL, key, query string, limit int) ([]searchResult, error) {
	payload, _ := json.Marshal(map[string]any{"q": query, "num": limit})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("serper: build: %w", err)
	}
	req.Header.Set("X-API-KEY", key)
	req.Header.Set("Content-Type", "application/json")
	body, err := t.doSearchHTTP(req, "serper")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("serper: parse: %w", err)
	}
	out := make([]searchResult, 0, len(resp.Organic))
	for _, r := range resp.Organic {
		out = append(out, searchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return out, nil
}

// searchTavily queries Tavily (AI-agent-tuned search).
// Endpoint: POST {baseURL}/search. Auth: api_key in JSON body.
// Response shape: {"results":[{"title","url","content"}]}.
//
// searchTavily 调 Tavily（agent 调优搜索）。响应形如
// {"results":[{"title","url","content"}]}。
func (t *WebSearch) searchTavily(ctx context.Context, baseURL, key, query string, limit int) ([]searchResult, error) {
	payload, _ := json.Marshal(map[string]any{
		"api_key":     key,
		"query":       query,
		"max_results": limit,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("tavily: build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := t.doSearchHTTP(req, "tavily")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tavily: parse: %w", err)
	}
	out := make([]searchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}

// searchBocha queries Bocha (博查) — China-native search API. Endpoint:
// POST {baseURL}/web-search. Auth: Authorization Bearer header. Response
// shape: {"data":{"webPages":{"value":[{"name","url","snippet"}]}}}.
//
// searchBocha 调博查——国产搜索 API。响应形如
// {"data":{"webPages":{"value":[{"name","url","snippet"}]}}}。
func (t *WebSearch) searchBocha(ctx context.Context, baseURL, key, query string, limit int) ([]searchResult, error) {
	payload, _ := json.Marshal(map[string]any{"query": query, "count": limit})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/web-search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("bocha: build: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	body, err := t.doSearchHTTP(req, "bocha")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			WebPages struct {
				Value []struct {
					Name    string `json:"name"`
					URL     string `json:"url"`
					Snippet string `json:"snippet"`
				} `json:"value"`
			} `json:"webPages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bocha: parse: %w", err)
	}
	out := make([]searchResult, 0, len(resp.Data.WebPages.Value))
	for _, r := range resp.Data.WebPages.Value {
		out = append(out, searchResult{Title: r.Name, URL: r.URL, Snippet: r.Snippet})
	}
	return out, nil
}

// doSearchHTTP is the shared HTTP send + status check used by all 4 BYOK
// clients. Returns body bytes on 2xx; named-provider error otherwise so
// the routing log shows which backend failed.
//
// doSearchHTTP 是 4 个 BYOK client 共用的 HTTP 发送 + 状态检查。2xx 返 body；
// 否则返带 provider 名的错误，让路由日志看清是哪一家挂。
func (t *WebSearch) doSearchHTTP(req *http.Request, provider string) ([]byte, error) {
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: connection: %w", provider, err)
	}
	defer resp.Body.Close()
	body := make([]byte, 0, 64*1024)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
			if len(body) > 256*1024 {
				body = body[:256*1024]
				break
			}
		}
		if rerr != nil {
			break
		}
	}
	if resp.StatusCode/100 != 2 {
		// Wrap with status-classified sentinel so errors.Is can drive
		// downstream behaviour (e.g. markInvalidIfAuthErr → flip apikey
		// status). Previously err.Error() string-matched "HTTP 401" /
		// "HTTP 403" — fragile and breaks if format changes.
		// 用状态分类 sentinel 包装让 errors.Is 驱动下游（如 markInvalidIfAuthErr
		// → 翻 apikey 状态）。原来 err.Error() string match "HTTP 401"
		// 易碎，格式变就破。
		var sentinel error
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			sentinel = ErrAuthFailed
		case http.StatusTooManyRequests:
			sentinel = ErrRateLimited
		default:
			sentinel = ErrUpstreamHTTP
		}
		return nil, fmt.Errorf("%s: HTTP %d: %w: %s", provider, resp.StatusCode, sentinel, snippet(body, 200))
	}
	return body, nil
}

// snippet trims body bytes for inclusion in error messages.
//
// snippet 截 body 字节用于错误消息。
func snippet(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}
