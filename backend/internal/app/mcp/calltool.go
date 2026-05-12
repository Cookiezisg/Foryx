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
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CallTool routes one tool/call to the named server. Computes the per-
// call timeout from §5.7 precedence (ServerConfig.TimeoutSec > 30s
// default), wraps the parent ctx in a deadline, invokes the Client,
// then records success/failure into ServerStatus counters and triggers
// degraded/recovery transitions per §5.6.
//
// CallTool 把一次 tool/call 路由到 named server。按 §5.7 precedence 算 per-
// call 超时（ServerConfig.TimeoutSec > 30s），把父 ctx 包成 deadline，调
// Client，然后把成功/失败记到 ServerStatus counter，按 §5.6 触发
// degraded/恢复转换。
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

	startedAt := time.Now().UTC()
	result, err := client.CallTool(cctx, tool, args)
	endedAt := time.Now().UTC()
	s.recordCallResult(server, err)
	// D22 call log (terminal write, detached ctx per §S9 — fire-and-forget).
	// D22 call log 终态写 detached ctx (fire-and-forget,失败不挂主调用)。
	s.recordCallLog(ctx, server, tool, state, args, result, err, startedAt, endedAt)
	return result, err
}

// recordCallLog persists one mcp_calls row (D22). Best-effort — failure
// logs but doesn't fail the CallTool path. Uses a detached ctx + user
// stamp so caller-cancel doesn't lose the audit row (§S9).
//
// recordCallLog 写 mcp_calls 一行(D22)best-effort;detached ctx + user
// stamp 防 caller-cancel 丢 audit。
func (s *Service) recordCallLog(ctx context.Context, server, tool string, state *mcpdomain.ServerStatus, args json.RawMessage, result string, callErr error, startedAt, endedAt time.Time) {
	s.mu.RLock()
	repo := s.callRepo
	s.mu.RUnlock()
	if repo == nil {
		return
	}
	uid, _ := reqctxpkg.RequireUserID(ctx)
	if uid == "" {
		uid = reqctxpkg.DefaultLocalUserID
	}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)

	status := mcpdomain.CallStatusOK
	errCode := ""
	errMsg := ""
	if callErr != nil {
		switch {
		case errors.Is(callErr, context.Canceled):
			status = mcpdomain.CallStatusCancelled
			errCode = "CTX_CANCELLED"
		case errors.Is(callErr, context.DeadlineExceeded):
			status = mcpdomain.CallStatusTimeout
			errCode = "MCP_TOOL_CALL_TIMEOUT"
		default:
			status = mcpdomain.CallStatusFailed
			errCode = "MCP_TOOL_CALL_FAILED"
		}
		errMsg = callErr.Error()
	}

	triggeredBy := mcpdomain.TriggeredByChat
	if toolCallID == "" && convID == "" {
		triggeredBy = mcpdomain.TriggeredByHTTP
	}

	var inputMap map[string]any
	_ = json.Unmarshal(args, &inputMap)

	var output any
	if result != "" {
		_ = json.Unmarshal([]byte(result), &output)
		if output == nil {
			output = result
		}
	}

	// V1: ServerStatus doesn't carry server's self-reported version yet
	// (initialize-response field unexposed in mcpinfra Client); leave
	// empty until that lands. _ = state keeps the param meaningful.
	// V1:ServerStatus 暂未携 server 自报 version;mcpinfra Client 暴露后再填。
	serverVersion := ""
	_ = state

	row := &mcpdomain.Call{
		ID:             idgenpkg.New("mcl"),
		UserID:         uid,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          inputMap,
		Output:         output,
		ErrorCode:      errCode,
		ErrorMessage:   errMsg,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		ServerName:     server,
		ToolName:       tool,
		ServerVersion:  serverVersion,
	}

	detached := reqctxpkg.SetUserID(context.Background(), uid)
	if err := repo.SaveCall(detached, row); err != nil {
		s.log.Warn("recordCallLog: save failed",
			zap.String("server", server),
			zap.String("tool", tool),
			zap.Error(err))
	}
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
	// caller's tool_call (chat invokes this from search_mcp_tools).
	// emitter is no-op outside chat — always safe.
	//
	// 把内部 LLM rerank 作为 progress block 挂调用方 tool_call 下（chat
	// 经 search_mcp_tools 调）；chat 外 emitter no-op，永远安全。
	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress,
		map[string]any{"stage": "rerank", "tool": "search_mcp_tools", "candidates": len(all)})

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
		// Ranking parse failure → return error to caller (LLM). Alpha-
		// order fallback would be misleading — for "PDF reader" an
		// alpha-ordered list starting with "ai-coder" tricks the LLM
		// into recommending wrong tools. Fail loud so LLM retries /
		// refines.
		//
		// 排序解析失败 → 返错给调用方（LLM）。字母序兜底是误导——搜
		// "PDF reader" 拿到字母序首位 "ai-coder" 与 query 无关，骗 LLM 推错
		// 工具。明显失败让 LLM 自重试/精化。
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
// CallTool. Increments TotalCalls; on err: bumps TotalFailures +
// ConsecutiveFailures, sets LastError; ≥ degradedThreshold (3) consecutive
// while ready → degraded (in-memory only — frontend sees this on next
// ListServers / health-check poll, no notification). On success: clears
// ConsecutiveFailures, sets LastSuccessAt; degraded → ready auto-heal
// (also in-memory only). Per mcp.md §5.6 通知边界.
//
// recordCallResult 每次 CallTool 后更新 per-server 健康 counter。增
// TotalCalls；err：增 TotalFailures + ConsecutiveFailures、设 LastError；
// 连续失败 ≥ degradedThreshold (3) 且当前 ready → degraded（仅内存——
// 不主动推；前端下次 ListServers / health-check 轮询时看到）。成功：清
// ConsecutiveFailures、设 LastSuccessAt；前为 degraded → ready 自愈（同样
// 仅内存）。详 mcp.md §5.6 通知边界。
func (s *Service) recordCallResult(name string, err error) {
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

// resolveCallTimeout walks the §5.7 precedence chain: per-server
// ServerConfig.TimeoutSec when > 0 wins; otherwise defaultCallTimeout.
//
// resolveCallTimeout 走 §5.7 precedence 链：per-server
// ServerConfig.TimeoutSec > 0 时优先；否则回 defaultCallTimeout。
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


