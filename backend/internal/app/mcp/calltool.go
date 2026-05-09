// calltool.go — Service.CallTool, Search, HealthCheck. The hot-path
// methods that the LLM (via search_mcp / call_mcp tools) and the UI
// "Test Connection" button drive at runtime.
//
// CallTool integrates the §5.7 timeout precedence + §5.6 health
// tracking (consecutive-failure → degraded transition + auto-heal).
// Search uses the LLM-ranking pattern from forge.search (mcp.md §6
// mode A). HealthCheck probes via tools/list without mutating any
// ServerStatus counter (so test-connection clicks don't accidentally
// trigger degraded).
//
// calltool.go ——Service.CallTool / Search / HealthCheck。LLM（经
// search_mcp / call_mcp）与 UI "Test Connection" 按钮在运行时驱动的热路径。
//
// CallTool 整合 §5.7 超时 precedence + §5.6 健康追踪（连续失败 → degraded
// + 自愈）。Search 沿用 forge.search LLM-ranking 模式（mcp.md §6 mode A）。
// HealthCheck 经 tools/list 探针，不改任何 ServerStatus 计数（防 test-
// connection 点击触发 degraded）。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// CallTool routes one tool/call to the named server. Computes the per-
// call timeout from §5.7 precedence (ServerConfig.TimeoutSec >
// RegistryEntry.DefaultTimeoutSec > 30s default), wraps the parent ctx
// in a deadline, invokes the Client, then records success/failure into
// ServerStatus counters and triggers degraded/recovery transitions per
// §5.6.
//
// CallTool 把一次 tool/call 路由到 named server。按 §5.7 precedence 算 per-
// call 超时（ServerConfig.TimeoutSec > RegistryEntry.DefaultTimeoutSec
// > 30s），把父 ctx 包成 deadline，调 Client，然后把成功/失败记到
// ServerStatus counter，按 §5.6 触发 degraded/恢复转换。
func (s *Service) CallTool(ctx context.Context, server, tool string, args json.RawMessage) (string, error) {
	s.mu.RLock()
	client, hasClient := s.clients[server]
	state := s.states[server]
	cfg := s.configs[server]
	s.mu.RUnlock()

	if state == nil {
		return "", fmt.Errorf("mcpapp.CallTool: %w: %q", mcpdomain.ErrServerNotFound, server)
	}
	if !hasClient || !mcpdomain.IsCallable(state.Status) {
		return "", fmt.Errorf("mcpapp.CallTool %s: %w (status=%s)",
			server, mcpdomain.ErrServerNotConnected, state.Status)
	}
	// Validate the tool exists on this server before dispatching — gives
	// LLM a precise ErrToolNotFound rather than the server's generic
	// "method not found" RPC error.
	// 派发前校验 tool 存在——给 LLM 精确的 ErrToolNotFound 而非 server 通用
	// "method not found" RPC 错。
	if !toolExists(state.Tools, tool) {
		return "", fmt.Errorf("mcpapp.CallTool %s/%s: %w",
			server, tool, mcpdomain.ErrToolNotFound)
	}

	timeout := s.resolveCallTimeout(cfg)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := client.CallTool(cctx, tool, args)
	s.recordCallResult(ctx, server, err)
	return result, err
}

// Search returns at most topK ToolDef matching query. When the total
// connected-server tool count is ≤ topK we skip the LLM call entirely
// and return everything (mcp.md §6 says "少时直接全返"). Otherwise we
// build a ranking prompt à la forge.search mode A and parse the LLM's
// ordered ID list.
//
// Search 返最多 topK 个匹配 query 的 ToolDef。connected server 总工具数
// ≤ topK 时跳过 LLM 直接全返（mcp.md §6 "少时直接全返"）。否则构造
// forge.search 模式 A 的排序 prompt 并解析 LLM 排序 ID 列表。
func (s *Service) Search(ctx context.Context, query string, topK int) ([]mcpdomain.ToolDef, error) {
	if topK <= 0 {
		topK = 5
	}
	all := s.ListTools(ctx)
	if len(all) == 0 {
		return []mcpdomain.ToolDef{}, nil
	}
	if len(all) <= topK {
		return all, nil
	}

	prompt := buildRankingPrompt(query, all, topK)

	// Surface this internal LLM rerank as a progress block under the
	// caller's tool_call (chat invokes this from search_mcp_marketplace
	// or similar). emitter is no-op outside chat — always safe.
	//
	// 把内部 LLM rerank 作为 progress block 挂调用方 tool_call 下（chat
	// 经 search_mcp_marketplace 等调）；chat 外 emitter no-op，永远安全。
	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress,
		map[string]any{"stage": "rerank", "tool": "search_mcp_marketplace", "candidates": len(all)})

	bundle, err := llmclientpkg.Resolve(ctx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return nil, fmt.Errorf("mcpapp.Search: resolve LLM: %w", err)
	}
	resp, err := llminfra.Generate(ctx, bundle.Client, llminfra.Request{
		ModelID: bundle.ModelID,
		Key:     bundle.Key,
		BaseURL: bundle.BaseURL,
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return nil, fmt.Errorf("mcpapp.Search: llm: %w", err)
	}
	em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	indices, err := parseRankedIndices(resp, len(all))
	if err != nil {
		// Ranking parse failure → return error to caller (LLM). Returning
		// alpha-order top K like the previous implementation did was
		// misleading — for a search query like "PDF reader" an alpha-
		// ordered list starting with "ai-coder" is unrelated and could
		// trick the LLM into recommending wrong tools. Better to fail
		// loudly so LLM can retry / refine the query (consistent with
		// the post-2026-05-08 屎山拯救计划 #4 search_mcp_marketplace pattern).
		//
		// 排序解析失败 → 返错给调用方（LLM）。原实现返字母序前 K 是误导
		// ——搜 "PDF reader" 拿到字母序首位 "ai-coder" 跟 query 无关，可能
		// 骗 LLM 推错工具。明显失败让 LLM 自重试/精化（与 2026-05-08 后
		// 屎山拯救计划 #4 search_mcp_marketplace 模式一致）。
		s.log.Warn("mcp search rank parse failed",
			zap.String("query", query),
			zap.String("response_snippet", trimResp(resp, 200)),
			zap.Error(err))
		return nil, fmt.Errorf("mcpapp.Search: ranking failed; LLM should retry or refine query: %w", err)
	}

	out := make([]mcpdomain.ToolDef, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(all) {
			continue
		}
		out = append(out, all[idx])
		if len(out) >= topK {
			break
		}
	}
	return out, nil
}

// HealthCheck probes the server with a tools/list call and times the
// RTT. Does NOT mutate ServerStatus — UI test-connection clicks
// shouldn't accidentally trip the degraded transition. 10s timeout.
//
// HealthCheck 用 tools/list 探针 + 测 RTT。不改 ServerStatus——UI
// test-connection 点击不该误触 degraded 转换。10s 超时。
func (s *Service) HealthCheck(ctx context.Context, name string) (*mcpdomain.HealthResult, error) {
	s.mu.RLock()
	client, hasClient := s.clients[name]
	state := s.states[name]
	s.mu.RUnlock()

	if state == nil {
		return nil, fmt.Errorf("mcpapp.HealthCheck: %w: %q", mcpdomain.ErrServerNotFound, name)
	}
	res := &mcpdomain.HealthResult{
		ServerName: name,
		CheckedAt:  time.Now().UTC(),
	}
	if !hasClient {
		res.Healthy = false
		res.Error = "server not connected"
		return res, nil
	}

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	tools, err := client.ListTools(cctx)
	res.LatencyMs = int(time.Since(start).Milliseconds())
	if err != nil {
		res.Healthy = false
		res.Error = err.Error()
		return res, nil
	}
	res.Healthy = true
	res.ToolCount = len(tools)
	return res, nil
}

// ── recordCallResult (internal) ──────────────────────────────────────

// recordCallResult updates the per-server health counters after each
// CallTool. Increments TotalCalls; on err: increments TotalFailures +
// ConsecutiveFailures, sets LastError; if ConsecutiveFailures hits
// degradedThreshold (3) and current status is ready, transitions to
// degraded + publishes SSE. On success: clears ConsecutiveFailures, sets
// LastSuccessAt; if status was degraded, transitions back to ready +
// publishes SSE (auto-heal).
//
// recordCallResult 每次 CallTool 后更新 per-server 健康 counter。增
// TotalCalls；err：增 TotalFailures + ConsecutiveFailures、设 LastError；
// ConsecutiveFailures 达 degradedThreshold (3) 且当前 ready → degraded +
// 发 SSE。成功：清 ConsecutiveFailures、设 LastSuccessAt；前为 degraded
// → ready + 发 SSE（自愈）。
func (s *Service) recordCallResult(_ context.Context, name string, err error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[name]
	if state == nil {
		return
	}
	state.TotalCalls++
	if err != nil {
		state.TotalFailures++
		state.ConsecutiveFailures++
		state.LastError = err.Error()
		state.LastErrorAt = &now
		if state.ConsecutiveFailures >= degradedThreshold && state.Status == mcpdomain.StatusReady {
			state.Status = mcpdomain.StatusDegraded
		}
	} else {
		state.ConsecutiveFailures = 0
		state.LastSuccessAt = &now
		if state.Status == mcpdomain.StatusDegraded {
			state.Status = mcpdomain.StatusReady
		}
	}
}

// resolveCallTimeout walks the §5.7 precedence chain.
//
// resolveCallTimeout 走 §5.7 precedence 链。registry 端点的 lookup 不能在
// 热路径里调（每次 CallTool 都拉远程 marketplace 不现实）——只看 ServerConfig
// .TimeoutSec，它在 install 时已从 RegistryEntry.DefaultTimeoutSec 复制过来。
func (s *Service) resolveCallTimeout(cfg mcpdomain.ServerConfig) time.Duration {
	if cfg.TimeoutSec > 0 {
		return time.Duration(cfg.TimeoutSec) * time.Second
	}
	return defaultCallTimeout
}

// ── ranking helpers ──────────────────────────────────────────────────

// buildRankingPrompt assembles the LLM ranking request: numbered tool
// catalog + the user query + JSON output spec. Uses 0-based indexes
// instead of full server/tool names so the LLM's response stays compact
// (and we don't burn tokens on long names like
// "mcp__github__create_pull_request_with_files").
//
// buildRankingPrompt 装 LLM 排序请求：编号 tool 目录 + 用户 query + JSON
// 输出规范。用 0-based index 而非完整 server/tool 名让 LLM 响应紧凑（不
// 在 "mcp__github__create_pull_request_with_files" 这种长名上烧 token）。
func buildRankingPrompt(query string, all []mcpdomain.ToolDef, topK int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nAvailable MCP tools:\n", query)
	for i, t := range all {
		desc := t.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		fmt.Fprintf(&sb, "%d. [%s] %s — %s\n", i, t.ServerName, t.Name, desc)
	}
	fmt.Fprintf(&sb, "\nReturn the indices of the %d most relevant tools as a JSON array, "+
		"most relevant first: [3, 7, 1, ...]\n"+
		"Respond with valid JSON only, no surrounding prose.", topK)
	return sb.String()
}

// parseRankedIndices extracts the LLM-emitted index array. Tolerates
// markdown fencing / surrounding prose via llmparsepkg.ExtractJSON.
// Validates each index is within [0, total).
//
// parseRankedIndices 提取 LLM 发的 index 数组。经 llmparsepkg.ExtractJSON
// 容忍 markdown 围栏 / 前后散文。校验每个 index 在 [0, total)。
func parseRankedIndices(resp string, total int) ([]int, error) {
	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		return nil, fmt.Errorf("mcpapp.parseRankedIndices: no JSON in response: %q", trimResp(resp, 200))
	}
	var raw []int
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("mcpapp.parseRankedIndices: parse JSON: %w", err)
	}
	out := make([]int, 0, len(raw))
	for _, idx := range raw {
		if idx >= 0 && idx < total {
			out = append(out, idx)
		}
	}
	return out, nil
}

// toolExists is the per-server membership check used by CallTool.
//
// toolExists 是 CallTool 用的 per-server 成员检查。
func toolExists(tools []mcpdomain.ToolDef, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func trimResp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}


