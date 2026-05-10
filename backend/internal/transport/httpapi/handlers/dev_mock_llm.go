// dev_mock_llm.go — /dev/mock-llm/* endpoints (TE-4b). The HTTP
// surface that lets testend's Mock LLM tab push canned scripts into
// the singleton llminfra.MockClient + inspect what the chat runner
// most recently sent to the LLM. Only registered when --dev is on
// (router.go gates on Deps.Dev) AND llmFactory is wired.
//
// Endpoints (all under /dev/mock-llm/):
//   POST   /scripts        push 1+ scripts (JSON body)
//   GET    /queue          current queue depth + script previews
//   DELETE /scripts        clear queue → returns count dropped
//   GET    /last-prompt    Request payload from most recent Stream call
//                          (system prompt + messages + tool defs that
//                          chat runner actually sent to the LLM)
//
// dev_mock_llm.go ——/dev/mock-llm/* 端点（TE-4b）。给 testend 的
// Mock LLM tab 用的 HTTP 面：推预设脚本进 llminfra.MockClient 单例 +
// 检查 chat runner 最近发了啥给 LLM。仅 --dev 启动时注册。
package handlers

import (
	"errors"
	"fmt"
	"net/http"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// mockScriptInput is the JSON-friendly shape of a single script. Events
// match llminfra.StreamEvent field names but accept JSON-camelCase for
// readability in the testend editor (stream events use Go field names
// internally; converted at parse time).
//
// mockScriptInput 是单 script 的 JSON 友好形状。Events 字段对应
// llminfra.StreamEvent，接 JSON-camelCase 让 testend 编辑器写起来顺手
// （stream events 内部用 Go 字段名；解析时转）。
type mockScriptInput struct {
	// Events to emit through the iterator, in order.
	// 按顺序通过迭代器发的 events。
	Events []mockEventInput `json:"events"`

	// ErrAfter, when set, replaces the entire script with a single
	// EventError carrying this message. Lets users exercise error paths
	// without crafting an event sequence.
	//
	// ErrAfter 设了把整段 script 替换为单个 EventError 携此消息。让用户
	// 不用编排事件即可触错误路径。
	ErrAfter string `json:"errAfter,omitempty"`
}

// mockEventInput is one StreamEvent in the JSON-input form.
//
// mockEventInput 是 JSON-input 形式的一个 StreamEvent。
type mockEventInput struct {
	Type         string `json:"type"`         // "text" | "reasoning" | "tool_start" | "tool_delta" | "finish" | "error"
	Delta        string `json:"delta,omitempty"`
	ToolIndex    int    `json:"toolIndex,omitempty"`
	ToolID       string `json:"toolId,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	ArgsDelta    string `json:"argsDelta,omitempty"`
	FinishReason string `json:"finishReason,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
	Error        string `json:"error,omitempty"` // EventError text
}

func (m *mockEventInput) toStreamEvent() (llminfra.StreamEvent, error) {
	ev := llminfra.StreamEvent{
		Delta:        m.Delta,
		ToolIndex:    m.ToolIndex,
		ToolID:       m.ToolID,
		ToolName:     m.ToolName,
		ArgsDelta:    m.ArgsDelta,
		FinishReason: m.FinishReason,
		InputTokens:  m.InputTokens,
		OutputTokens: m.OutputTokens,
	}
	switch m.Type {
	case "text":
		ev.Type = llminfra.EventText
	case "reasoning":
		ev.Type = llminfra.EventReasoning
	case "tool_start":
		ev.Type = llminfra.EventToolStart
	case "tool_delta":
		ev.Type = llminfra.EventToolDelta
	case "finish":
		ev.Type = llminfra.EventFinish
	case "error":
		ev.Type = llminfra.EventError
		if m.Error != "" {
			ev.Err = errors.New(m.Error)
		}
	default:
		return ev, fmt.Errorf("handlers.toStreamEvent: unknown event type %q (want text/reasoning/tool_start/tool_delta/finish/error)", m.Type)
	}
	return ev, nil
}

// MockLLMPushScripts handles POST /dev/mock-llm/scripts.
// Body: {"scripts": [<mockScriptInput>...]}
//
// Each script is enqueued in order; consecutive Stream() calls will
// pop them in push order. Per-script ErrAfter shortcuts the events
// array (whole script becomes a single EventError).
//
// MockLLMPushScripts 处理 POST /dev/mock-llm/scripts。
// Body：{"scripts": [<mockScriptInput>...]}
// 每段 script 按序入队；连续 Stream() 按 push 顺序弹。每段的 ErrAfter
// 短路 events 数组（整段变单个 EventError）。
func (h *DevHandler) MockLLMPushScripts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scripts []mockScriptInput `json:"scripts"`
	}
	if err := decodeJSON(r, &body); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if len(body.Scripts) == 0 {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"no scripts in payload (expect {scripts: [...]})", nil)
		return
	}

	mock := h.llmFactory.Mock()
	pushed := 0
	for i, in := range body.Scripts {
		s := llminfra.MockScript{}
		if in.ErrAfter != "" {
			s.ErrAfter = errors.New(in.ErrAfter)
		} else {
			for j, eIn := range in.Events {
				ev, err := eIn.toStreamEvent()
				if err != nil {
					responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
						fmt.Sprintf("script[%d].events[%d]: %s", i, j, err.Error()), nil)
					return
				}
				s.Events = append(s.Events, ev)
			}
		}
		mock.PushScript(s)
		pushed++
	}

	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"pushed":     pushed,
		"queueDepth": mock.QueueDepth(),
	})
}

// MockLLMQueue handles GET /dev/mock-llm/queue.
// Returns the current queue depth + a per-script preview (event count,
// first event type as a quick visual cue, errAfter when set). Useful
// for testend to show "12 scripts pending: text, tool_start, finish, ...".
//
// MockLLMQueue 处理 GET /dev/mock-llm/queue。返当前队列深度 + per-
// script 概览（event 数 / 首 event 类型作快速视觉提示 / errAfter）。
// 让 testend 显示"队列 12 个：text、tool_start、finish、..."。
func (h *DevHandler) MockLLMQueue(w http.ResponseWriter, r *http.Request) {
	mock := h.llmFactory.Mock()
	queue := mock.Queue()
	previews := make([]map[string]any, 0, len(queue))
	for _, s := range queue {
		p := map[string]any{
			"eventCount": len(s.Events),
		}
		if s.ErrAfter != nil {
			p["errAfter"] = s.ErrAfter.Error()
		}
		if len(s.Events) > 0 {
			p["firstType"] = string(s.Events[0].Type)
		}
		previews = append(previews, p)
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"depth":     len(queue),
		"callCount": mock.CallCount(),
		"previews":  previews,
	})
}

// MockLLMClear handles DELETE /dev/mock-llm/scripts.
// Drops all queued scripts; returns count dropped so the caller knows.
//
// MockLLMClear 处理 DELETE /dev/mock-llm/scripts。
// 丢全部 queued scripts；返丢的数让调用方知道。
func (h *DevHandler) MockLLMClear(w http.ResponseWriter, r *http.Request) {
	dropped := h.llmFactory.Mock().Clear()
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"dropped": dropped,
	})
}

// MockLLMLastPrompt handles GET /dev/mock-llm/last-prompt.
// Returns the most recent Stream() call's Request payload — system
// prompt (with catalog block, locale hint, etc.) + messages array +
// tool defs sent to the LLM. THE single most useful endpoint for
// debugging "why didn't the LLM see X?" questions.
//
// MockLLMLastPrompt 处理 GET /dev/mock-llm/last-prompt。返最近一次
// Stream() 调用的 Request 载荷——含 catalog 块/locale hint 等的 system
// prompt + messages 数组 + 发给 LLM 的 tool defs。debug "LLM 怎么没
// 看到 X" 类问题最有用的端点。
func (h *DevHandler) MockLLMLastPrompt(w http.ResponseWriter, r *http.Request) {
	req := h.llmFactory.Mock().LastRequest()
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"modelId":  req.ModelID,
		"baseURL":  req.BaseURL,
		"system":   req.System,
		"messages": req.Messages,
		"tools":    req.Tools,
	})
}

// LLMTrace handles GET /dev/llm-trace?conversationId=xxx (TE-5a).
// Returns the recorder's per-conversation traces (each trace = one
// Stream() call: full Request + every StreamEvent + final text +
// elapsed + error). Without a query param returns the list of
// conversation IDs that have traces (lets the Wire tab populate a
// dropdown). The recorder works for ALL providers (mock + real) when
// enabled in --dev mode, not just mock — same wrapper.
//
// LLMTrace 处理 GET /dev/llm-trace?conversationId=xxx（TE-5a）。返
// recorder 的 per-conversation traces（每条 trace = 一次 Stream() 调用:
// 完整 Request + 每个 StreamEvent + 最终文字 + 耗时 + error）。无
// query param 返有 trace 的对话 ID 列表（让 Wire tab 填 dropdown）。
// recorder 在 --dev mode 下对所有 provider（mock + real）都生效，不
// 只 mock——同 wrapper。
func (h *DevHandler) LLMTrace(w http.ResponseWriter, r *http.Request) {
	tracer := h.llmFactory.Tracer()
	if tracer == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "TRACER_DISABLED",
			"LLM trace recorder not enabled (only available in --dev)", nil)
		return
	}
	convID := r.URL.Query().Get("conversationId")
	if convID == "" {
		responsehttpapi.Success(w, http.StatusOK, map[string]any{
			"conversations": tracer.Conversations(),
		})
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"conversationId": convID,
		"traces":         tracer.TracesFor(convID),
	})
}
