package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrMCPSearchUnavailable signals "no MCP search server connected" so caller falls through.
//
// ErrMCPSearchUnavailable 表示无连接的 MCP 搜索 server；调用方降级。
var ErrMCPSearchUnavailable = errors.New("mcp search server unavailable")

// MCPSearchRouter delegates a query to a connected MCP search server.
//
// MCPSearchRouter 委派 query 给已连接的 MCP 搜索 server。
type MCPSearchRouter interface {
	// CallSearchTool sends query to the MCP server; returns ErrMCPSearchUnavailable when no server configured.
	//
	// CallSearchTool 把 query 发给 MCP server；未配置/未连接返 ErrMCPSearchUnavailable。
	CallSearchTool(ctx context.Context, query string, limit int) (string, error)
}

// runMCPSearch invokes the router and parses results from {"results":[...]} or a bare array.
//
// runMCPSearch 调 router 并解析 {"results":[...]} 或裸数组结果。
func (t *WebSearch) runMCPSearch(ctx context.Context, query string, limit int) ([]searchResult, error) {
	if t.mcpRouter == nil {
		return nil, ErrMCPSearchUnavailable
	}
	raw, err := t.mcpRouter.CallSearchTool(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	results, perr := parseMCPSearchResults(raw)
	if perr != nil {
		return nil, fmt.Errorf("mcp: parse: %w", perr)
	}
	return results, nil
}

// parseMCPSearchResults handles two known MCP shapes with best-effort field-name union.
//
// parseMCPSearchResults 处理 MCP 两种已知 shape，字段名按 best-effort 取并集。
func parseMCPSearchResults(raw string) ([]searchResult, error) {
	type item struct {
		Title       string `json:"title"`
		Name        string `json:"name"`
		URL         string `json:"url"`
		Link        string `json:"link"`
		Snippet     string `json:"snippet"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}

	pickOne := func(it item) searchResult {
		title := it.Title
		if title == "" {
			title = it.Name
		}
		u := it.URL
		if u == "" {
			u = it.Link
		}
		snip := it.Snippet
		if snip == "" {
			snip = it.Description
		}
		if snip == "" {
			snip = it.Content
		}
		return searchResult{Title: title, URL: u, Snippet: snip}
	}

	var keyed struct {
		Results []item `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &keyed); err == nil && len(keyed.Results) > 0 {
		out := make([]searchResult, 0, len(keyed.Results))
		for _, it := range keyed.Results {
			out = append(out, pickOne(it))
		}
		return out, nil
	}

	var bare []item
	if err := json.Unmarshal([]byte(raw), &bare); err == nil && len(bare) > 0 {
		out := make([]searchResult, 0, len(bare))
		for _, it := range bare {
			out = append(out, pickOne(it))
		}
		return out, nil
	}

	if raw != "" {
		return []searchResult{{Title: "MCP search result", Snippet: raw}}, nil
	}
	return nil, fmt.Errorf("empty MCP response")
}
