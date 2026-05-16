package handlers

import (
	"errors"
	"fmt"
	"net/http"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// mockScriptInput is the JSON-friendly shape of a single mock script.
//
// mockScriptInput 是单 script 的 JSON 友好形状。
type mockScriptInput struct {
	Events   []mockEventInput `json:"events"`
	ErrAfter string           `json:"errAfter,omitempty"`
}

// mockEventInput is one StreamEvent in JSON-input form.
//
// mockEventInput 是 JSON-input 形式的一个 StreamEvent。
type mockEventInput struct {
	Type         string `json:"type"`
	Delta        string `json:"delta,omitempty"`
	ToolIndex    int    `json:"toolIndex,omitempty"`
	ToolID       string `json:"toolId,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	ArgsDelta    string `json:"argsDelta,omitempty"`
	FinishReason string `json:"finishReason,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
	Error        string `json:"error,omitempty"`
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

// MockLLMPushScripts enqueues 1+ scripts onto the mock client queue.
//
// MockLLMPushScripts 把 1+ 段 script 按序入队到 mock client。
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

// MockLLMQueue returns queue depth + per-script previews.
//
// MockLLMQueue 返队列深度 + per-script 概览。
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

// MockLLMClear drops all queued mock scripts and returns the count dropped.
//
// MockLLMClear 丢全部 queued scripts,返丢的数。
func (h *DevHandler) MockLLMClear(w http.ResponseWriter, r *http.Request) {
	dropped := h.llmFactory.Mock().Clear()
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"dropped": dropped,
	})
}

// MockLLMLastPrompt returns the most recent Stream() call's Request payload.
//
// MockLLMLastPrompt 返最近一次 Stream() 调用的 Request 载荷。
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

// LLMTrace returns recorder traces; no conversationId returns the conv ID list.
//
// LLMTrace 返 recorder traces;无 conversationId 返 trace 对话 ID 列表。
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
