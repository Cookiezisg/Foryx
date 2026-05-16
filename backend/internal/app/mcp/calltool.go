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

// CallTool routes a tool/call with per-call timeout and updates health counters.
//
// CallTool 用 per-call 超时路由 tool/call，并更新健康计数。
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
	s.recordCallLog(ctx, server, tool, state, args, result, err, startedAt, endedAt)
	return result, err
}

// recordCallLog persists one mcp_calls row via detached ctx (§S9); best-effort.
//
// recordCallLog 用 detached ctx 写入 mcp_calls 一行，best-effort。
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

// Search returns up to topK tools; total ≤ topK skips the LLM.
//
// Search 返回最多 topK 个工具；总数 ≤ topK 时跳过 LLM 直接全返。
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

// HealthCheck probes with tools/list; does NOT mutate ServerStatus.
//
// HealthCheck 用 tools/list 探测；不修改 ServerStatus 计数。
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

// recordCallResult bumps per-server health counters; consecutive failures flip degraded/ready.
//
// recordCallResult 更新 per-server 健康计数；连续失败/成功触发 degraded/ready 转换。
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

func (s *Service) resolveCallTimeout(cfg mcpdomain.ServerConfig) time.Duration {
	if cfg.TimeoutSec > 0 {
		return time.Duration(cfg.TimeoutSec) * time.Second
	}
	return defaultCallTimeout
}

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


