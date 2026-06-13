package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
	logtailpkg "github.com/sunweilin/forgify/backend/internal/pkg/logtail"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CallTool routes a tool/call to the server's connected client with a per-call timeout, updates
// health counters (3 consecutive failures → degraded; a success → back to ready), records one
// mcp_calls audit row (C4 — MCP joins fn/hd/ag in having a durable execution log), and tees the
// server's progress notifications onto the entities run terminal scoped to the server (SSE-C).
// serverID is the mcp_ id. triggeredBy is the execution body (chat/agent/workflow/manual); ""
// derives it from ctx (subagent → agent, else chat) — the dynamic chat tool passes "".
//
// CallTool 用 per-call 超时把 tool/call 路由到 server 的已连接 client、更新健康计数（连续 3 失败 →
// degraded；一次成功 → 恢复 ready）、记一行 mcp_calls 审计（C4——MCP 与 fn/hd/ag 一样有耐久执行日志）、
// 并把 server 进度通知 tee 到该 server scope 的 entities run 终端（SSE-C）。serverID 是 mcp_ id。
// triggeredBy 是执行体（chat/agent/workflow/manual）；"" 从 ctx 推（subagent → agent，否则 chat）——
// chat 动态工具传 ""。
func (s *Service) CallTool(ctx context.Context, serverID, tool string, args json.RawMessage, triggeredBy string) (string, error) {
	s.mu.RLock()
	client := s.clients[serverID]
	st := s.states[serverID]
	s.mu.RUnlock()

	if st == nil {
		return "", fmt.Errorf("mcpapp.CallTool: %w: %q", mcpdomain.ErrServerNotFound, serverID)
	}
	if client == nil || !mcpdomain.IsCallable(st.Status) {
		return "", fmt.Errorf("mcpapp.CallTool %s: %w (status=%s)", st.Name, mcpdomain.ErrServerNotConnected, st.Status)
	}

	cctx, cancel := context.WithTimeout(ctx, time.Duration(limitspkg.Current().Timeout.MCPCallSec)*time.Second)
	defer cancel()

	// Tee progress notifications to the entities run terminal (entity panel, all callers) and the
	// call's capped logtail (persisted on the mcp_calls row), on top of any sink the caller already
	// put in ctx (the dynamic tool's chat sink). Lazy node — a server that emits no progress opens
	// nothing on the terminal; the logtail just stays empty.
	//
	// 把进度通知 tee 到 entities run 终端（实体面板，全 caller）+ 本次调用的限长 logtail（随 mcp_calls
	// 行落盘），叠在调用方已放 ctx 的 sink（动态工具的 chat sink）之上。懒节点——不发进度的 server 终端
	// 不开任何帧；logtail 保持空。
	runTerm := entitystreamapp.New(ctx, s.entities, streamdomain.Scope{Kind: streamdomain.KindMCP, ID: serverID}, entitystreamapp.NodeRun, streamdomain.JSONContent(map[string]any{"tool": tool}))
	logs := logtailpkg.New(logtailpkg.DefaultCap)
	prev := mcpinfra.ProgressFrom(cctx)
	cctx = mcpinfra.WithProgress(cctx, func(line string) {
		if prev != nil {
			prev(line)
		}
		_, _ = runTerm.Write([]byte(line))
		_, _ = logs.Write([]byte(line))
	})

	startedAt := time.Now().UTC()
	result, err := client.CallTool(cctx, tool, args)
	endedAt := time.Now().UTC()
	if err != nil {
		runTerm.Close("error", nil)
		// A failed call appends the server's stderr tail (MCP's log channel) — the progress stream
		// rarely explains a crash, the stderr usually does. Server-level, so annotated as such.
		// 失败的调用补上 server stderr 尾（MCP 的日志通道）——progress 流很少解释崩溃，stderr 通常能。
		// 它是 server 级的，故标注说明。
		if tail := stderrTail(client.StderrTail(), stderrTailCap); tail != "" {
			fmt.Fprintf(logs, "\n--- server stderr tail (server-level, may predate this call) ---\n%s", tail)
		}
	} else {
		runTerm.Close("completed", nil)
	}

	s.recordResult(serverID, err)
	s.recordCall(ctx, serverID, tool, args, triggeredBy, result, logs.String(), err, cctx.Err(), startedAt, endedAt)
	return result, err
}

// stderrTailCap bounds how much server stderr a failed call carries — enough for a
// traceback, not the whole ring.
//
// stderrTailCap 限定失败调用携带多少 server stderr——够一个 traceback、不是整个 ring。
const stderrTailCap = 8 * 1024

// stderrTail keeps the last capBytes of s.
//
// stderrTail 保留 s 的末 capBytes。
func stderrTail(s string, capBytes int) string {
	if len(s) <= capBytes {
		return s
	}
	return s[len(s)-capBytes:]
}

// recordCall writes one terminal mcp_calls row (best-effort, on a detached ctx that keeps workspace
// so a cancelled call still persists). Mirrors handlerapp.recordCall.
//
// recordCall 写一行终态 mcp_calls（best-effort，用保留 workspace 的 detached ctx，使被取消的调用仍落账）。
// 对标 handlerapp.recordCall。
func (s *Service) recordCall(ctx context.Context, serverID, tool string, args json.RawMessage, triggeredBy, result, logs string, callErr, runCtxErr error, startedAt, endedAt time.Time) {
	status := mcpdomain.CallStatusOK
	errMsg := ""
	if callErr != nil {
		status = mcpdomain.CallStatusFailed
		errMsg = callErr.Error()
		if errors.Is(runCtxErr, context.DeadlineExceeded) {
			status = mcpdomain.CallStatusTimeout
		} else if errors.Is(runCtxErr, context.Canceled) {
			status = mcpdomain.CallStatusCancelled
		}
	}
	if !mcpdomain.IsValidCallTrigger(triggeredBy) {
		triggeredBy = mcpdomain.CallTriggeredByChat
		if _, inSub := reqctxpkg.GetSubagentID(ctx); inSub {
			triggeredBy = mcpdomain.CallTriggeredByAgent
		}
	}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	flowrunID, _ := reqctxpkg.GetFlowrunID(ctx)
	flowrunNodeID, _ := reqctxpkg.GetFlowrunNodeID(ctx)

	call := &mcpdomain.Call{
		ID:             idgenpkg.New("mcl"),
		ServerID:       serverID,
		Tool:           tool,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          args,
		Output:         result,
		ErrorMessage:   errMsg,
		Logs:           logs,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		FlowrunID:      flowrunID,
		FlowrunNodeID:  flowrunNodeID,
	}
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	detached := reqctxpkg.Detached(wsID)
	if err := s.repo.SaveCall(detached, call); err != nil {
		s.log.Warn("mcpapp.recordCall: save failed (best-effort)",
			zap.String("serverId", serverID), zap.String("tool", tool), zap.Error(err))
	}
}

// SearchCallsResult is the paged call-log + ok/failed rollup (mirrors handlerapp).
//
// SearchCallsResult 是分页调用日志 + ok/失败汇总（对标 handlerapp）。
type SearchCallsResult struct {
	Calls      []*mcpdomain.Call        `json:"calls"`
	NextCursor string                   `json:"nextCursor,omitempty"`
	HasMore    bool                     `json:"hasMore"`
	Aggregates mcpdomain.CallAggregates `json:"aggregates"`
}

// SearchCalls runs a paginated call-log query with the ok/failed rollup (the entity
// panel's run history + status badge — same surface every executable kind has).
//
// SearchCalls 跑分页调用日志查询 + ok/失败汇总（实体面板运行历史 + 状态徽标——与其它
// 可执行体同面）。
func (s *Service) SearchCalls(ctx context.Context, filter mcpdomain.CallFilter) (*SearchCallsResult, error) {
	rows, next, err := s.repo.ListCalls(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.SearchCalls: %w", err)
	}
	agg, err := s.repo.ComputeCallAggregates(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.SearchCalls: aggregates: %w", err)
	}
	return &SearchCallsResult{Calls: rows, NextCursor: next, HasMore: next != "", Aggregates: agg}, nil
}

// ListCalls pages a server's call log (the entity panel's run history).
//
// ListCalls 分页 server 的调用日志（实体面板的运行历史）。
func (s *Service) ListCalls(ctx context.Context, filter mcpdomain.CallFilter) ([]*mcpdomain.Call, string, error) {
	return s.repo.ListCalls(ctx, filter)
}

// GetCall returns one call-log record (AI :triage reads it to diagnose a failed mcp invocation).
//
// GetCall 返回一条调用日志记录（AI :triage 读它诊断一次失败的 mcp 调用）。
func (s *Service) GetCall(ctx context.Context, id string) (*mcpdomain.Call, error) {
	return s.repo.GetCall(ctx, id)
}

// recordResult bumps per-server health counters; consecutive failures/successes flip
// degraded/ready. Holds s.mu.
//
// recordResult 更新 per-server 健康计数；连续失败/成功翻转 degraded/ready。持 s.mu。
func (s *Service) recordResult(id string, callErr error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.states[id]
	if st == nil {
		return
	}
	st.TotalCalls++
	if callErr != nil {
		st.TotalFailures++
		st.ConsecutiveFailures++
		st.LastError = callErr.Error()
		st.LastErrorAt = &now
		if st.ConsecutiveFailures >= mcpdomain.DegradedThreshold && st.Status == mcpdomain.StatusReady {
			st.Status = mcpdomain.StatusDegraded
		}
	} else {
		st.ConsecutiveFailures = 0
		if st.Status == mcpdomain.StatusDegraded {
			st.Status = mcpdomain.StatusReady
		}
	}
}
