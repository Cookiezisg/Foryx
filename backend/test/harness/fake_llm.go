//go:build pipeline

package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// FakeLLMServer speaks OpenAI-compatible streaming chat completions; scripts FIFO.
//
// FakeLLMServer 说 OpenAI 兼容流式 chat completions；脚本 FIFO 消费。
type FakeLLMServer struct {
	t      *testing.T
	server *httptest.Server

	mu           sync.Mutex
	queue        []Script
	dflt         *Script
	calls        int
	modelsStatus int

	lastSystemPrompt string
	lastMessages     []FakeLLMMessage
}

// FakeLLMMessage is one role/content pair captured from a request.
//
// FakeLLMMessage 是请求中一个 role/content 对。
type FakeLLMMessage struct {
	Role    string
	Content string
}

// Script describes what one streaming completion call should emit.
//
// Script 描述一次流式 completion 调用应发出什么。
type Script struct {
	HTTPStatus   int
	Actions      []ChunkAction
	FinishReason string
	InputTokens  int
	OutputTokens int
}

// ChunkAction is one step in a Script; Kind ∈ {text, reasoning, tool_call_start, tool_call_delta, delay}.
//
// ChunkAction 是 Script 一步；Kind 取 text/reasoning/tool_call_start/tool_call_delta/delay。
type ChunkAction struct {
	Kind    string
	Content string
	ToolID  string
	Name    string
	Index   int
	Delay   time.Duration
}

// NewFakeLLMServer starts a fake OpenAI-compatible server; cleanup via t.Cleanup.
//
// NewFakeLLMServer 启动 fake OpenAI 兼容 server，清理挂 t.Cleanup。
func NewFakeLLMServer(t *testing.T) *FakeLLMServer {
	t.Helper()
	f := &FakeLLMServer{t: t, modelsStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", f.handle)
	mux.HandleFunc("GET /v1/models", f.handleModels)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// SetModelsStatus overrides GET /v1/models response status (default 200).
//
// SetModelsStatus 覆盖 GET /v1/models 响应状态（默认 200）。
func (f *FakeLLMServer) SetModelsStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modelsStatus = status
}

// URL returns the OpenAI-compatible base URL for WithFakeLLMBaseURL.
//
// URL 返回供 WithFakeLLMBaseURL 用的 OpenAI 兼容 base URL。
func (f *FakeLLMServer) URL() string { return f.server.URL + "/v1" }

// PushScript enqueues one script; scripts are popped FIFO.
//
// PushScript 入队一条脚本，FIFO 弹出。
func (f *FakeLLMServer) PushScript(s Script) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, s)
}

// PushDefault sets the fallback script for when the queue is empty.
//
// PushDefault 设置队列空时的兜底脚本。
func (f *FakeLLMServer) PushDefault(s Script) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := s
	f.dflt = &cp
}

// CallCount returns the total completions requests received.
//
// CallCount 返回收到的 completions 请求总数。
func (f *FakeLLMServer) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// LastSystemPrompt returns the system message from the most recent request, or empty.
//
// LastSystemPrompt 返最近一次请求的 system 消息，无则返空。
func (f *FakeLLMServer) LastSystemPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastSystemPrompt
}

// LastMessages returns a copy of the ordered Messages from the most recent request.
//
// LastMessages 返最近一次请求的有序 Messages 拷贝。
func (f *FakeLLMServer) LastMessages() []FakeLLMMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeLLMMessage, len(f.lastMessages))
	copy(out, f.lastMessages)
	return out
}

func (f *FakeLLMServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(raw, &req) == nil {
			msgs := make([]FakeLLMMessage, 0, len(req.Messages))
			var sys string
			for _, m := range req.Messages {
				msgs = append(msgs, FakeLLMMessage{Role: m.Role, Content: m.Content})
				if m.Role == "system" && sys == "" {
					sys = m.Content
				}
			}
			f.mu.Lock()
			f.lastMessages = msgs
			if sys != "" {
				f.lastSystemPrompt = sys
			}
			f.mu.Unlock()
		}
	}

	f.mu.Lock()
	var (
		script Script
		ok     bool
	)
	if len(f.queue) > 0 {
		script = f.queue[0]
		f.queue = f.queue[1:]
		ok = true
	} else if f.dflt != nil {
		script = *f.dflt
		ok = true
	}
	if ok {
		f.calls++
	}
	f.mu.Unlock()

	if !ok {
		f.t.Errorf("FakeLLMServer: request received but no script in queue and no default set")
		http.Error(w,
			`{"error":{"message":"no script configured","type":"test_error"}}`,
			http.StatusInternalServerError)
		return
	}

	if script.HTTPStatus != 0 {
		http.Error(w,
			`{"error":{"message":"fake provider error","type":"test_error"}}`,
			script.HTTPStatus)
		return
	}

	flusher, isFlusher := w.(http.Flusher)
	if !isFlusher {
		f.t.Errorf("FakeLLMServer: ResponseWriter does not implement http.Flusher")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	for _, action := range script.Actions {
		switch action.Kind {
		case "delay":
			flusher.Flush()
			time.Sleep(action.Delay)
		case "text":
			f.emitSSE(w, flusher, fakeChunk{
				Choices: []fakeChoice{{Delta: fakeDelta{Content: action.Content}}},
			})
		case "reasoning":
			f.emitSSE(w, flusher, fakeChunk{
				Choices: []fakeChoice{{Delta: fakeDelta{ReasoningContent: action.Content}}},
			})
		case "tool_call_start":
			f.emitSSE(w, flusher, fakeChunk{
				Choices: []fakeChoice{{Delta: fakeDelta{
					ToolCalls: []fakeToolCall{{
						Index:    action.Index,
						ID:       action.ToolID,
						Function: fakeFuncDelta{Name: action.Name},
					}},
				}}},
			})
		case "tool_call_delta":
			f.emitSSE(w, flusher, fakeChunk{
				Choices: []fakeChoice{{Delta: fakeDelta{
					ToolCalls: []fakeToolCall{{
						Index:    action.Index,
						Function: fakeFuncDelta{Arguments: action.Content},
					}},
				}}},
			})
		}
	}

	fr := script.FinishReason
	if fr == "" {
		fr = "stop"
	}
	f.emitSSE(w, flusher, fakeChunk{
		Choices: []fakeChoice{{FinishReason: fr}},
		Usage: &fakeUsage{
			PromptTokens:     script.InputTokens,
			CompletionTokens: script.OutputTokens,
		},
	})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (f *FakeLLMServer) emitSSE(w http.ResponseWriter, fl http.Flusher, chunk fakeChunk) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fl.Flush()
}

// handleModels serves GET /v1/models; status from SetModelsStatus (default 200).
//
// handleModels 提供 GET /v1/models；状态由 SetModelsStatus 控制（默认 200）。
func (f *FakeLLMServer) handleModels(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	status := f.modelsStatus
	f.mu.Unlock()

	if status != http.StatusOK {
		http.Error(w,
			`{"error":{"message":"invalid API key","type":"authentication_error"}}`,
			status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"data":[{"id":"fake-model-1"},{"id":"fake-model-2"}]}`)
}

type fakeChunk struct {
	Choices []fakeChoice `json:"choices"`
	Usage   *fakeUsage   `json:"usage,omitempty"`
}

type fakeChoice struct {
	Delta        fakeDelta `json:"delta"`
	FinishReason string    `json:"finish_reason,omitempty"`
}

type fakeDelta struct {
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []fakeToolCall `json:"tool_calls,omitempty"`
}

type fakeToolCall struct {
	Index    int           `json:"index"`
	ID       string        `json:"id,omitempty"`
	Function fakeFuncDelta `json:"function"`
}

type fakeFuncDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type fakeUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
