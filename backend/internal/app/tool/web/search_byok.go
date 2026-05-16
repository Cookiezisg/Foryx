package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// searchBrave queries Brave Search API; auth via X-Subscription-Token header.
//
// searchBrave 调 Brave Search API；认证走 X-Subscription-Token。
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

// searchSerper queries Serper.dev (Google results proxy); auth via X-API-KEY header.
//
// searchSerper 调 Serper.dev（Google 结果代理）；认证走 X-API-KEY。
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

// searchTavily queries Tavily; auth via api_key in JSON body.
//
// searchTavily 调 Tavily；认证用 api_key JSON body 字段。
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

// searchBocha queries Bocha (博查); auth via Authorization Bearer header.
//
// searchBocha 调博查；认证走 Authorization Bearer。
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

// doSearchHTTP is shared HTTP send + status check; 2xx returns body, otherwise provider-named error.
//
// doSearchHTTP 共享 HTTP 发送 + 状态检查；2xx 返 body，否则返带 provider 名的错误。
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

func snippet(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}
