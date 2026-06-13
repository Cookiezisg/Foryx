// llmmock.go is the OpenAI-compatible fake model server driving the LLM face of black-box
// scenarios at zero token cost. It speaks the REAL wire (POST /chat/completions SSE stream +
// GET /models probe), so the backend's whole provider HTTP path — request building, stream
// parsing, tool-call assembly, usage accounting — is exercised, not bypassed. Turns are
// scripted PER MODEL ID (dialogue vs utility queues never race), and every request is
// captured as a PromptDump: what the model actually saw on the wire IS the experience audit.
//
// llmmock.go 是 OpenAI 兼容的假模型 server，以零 token 驱动黑盒场景的 LLM 面。它讲真线缆
// （POST /chat/completions SSE 流 + GET /models 探测），后端整条 provider HTTP 链——请求构造、
// 流解析、tool-call 组装、usage 记账——全被压到，而非绕过。脚本按 MODEL ID 排队（dialogue 与
// utility 队列互不抢帧）；每个请求捕获为 PromptDump：模型在线缆上真看到了什么，本身就是体验审计。
package harness

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockToolCall is one scripted tool invocation. Args should carry the framework-standard
// fields a real LLM would self-report (summary / danger / execution_group) alongside the
// tool's own arguments.
//
// MockToolCall 是一次脚本化工具调用。Args 应像真 LLM 一样自报框架标准字段（summary /
// danger / execution_group）+ 工具自身参数。
type MockToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// LLMTurn is one scripted completion. Zero-value fields are defaulted: empty turn → a
// plain "ok." text reply with nominal usage.
//
// LLMTurn 是一次脚本化补全。零值字段有默认：空 turn → 一句 "ok." 文本 + 名义 usage。
type LLMTurn struct {
	Text             string
	Reasoning        string
	ToolCalls        []MockToolCall
	PromptTokens     int // default 100
	CompletionTokens int // default 10
	StallMS          int // flush the first text chunk, then stall this long (cancel scenarios)
	Status           int // non-zero → respond with this HTTP status + OpenAI error envelope
}

// PromptDump is one captured request — the model's-eye view of the conversation.
//
// PromptDump 是一次捕获的请求——模型视角的对话。
type PromptDump struct {
	Model    string
	System   string
	Messages []DumpMsg
	Tools    []string
	Raw      json.RawMessage
}

// DumpMsg is one wire message in a captured request (content flattened to string
// best-effort; multimodal arrays stay in Raw).
//
// DumpMsg 是捕获请求里的一条线缆消息（content 尽力拍平成 string；多模态数组留在 Raw）。
type DumpMsg struct {
	Role       string
	Content    string
	ToolCallID string
	ToolNames  []string // assistant tool_calls names
}

// HasMessage reports whether any wire message of the role contains the substring.
//
// HasMessage 报告某 role 的线缆消息是否含子串。
func (d *PromptDump) HasMessage(role, substr string) bool {
	for _, m := range d.Messages {
		if m.Role == role && strings.Contains(m.Content, substr) {
			return true
		}
	}
	return false
}

// LLMMock is the scripted fake provider. Start with NewLLMMock; point an apikey's baseUrl
// at URL().
//
// LLMMock 是脚本化假供应商。NewLLMMock 启动；apikey 的 baseUrl 指向 URL()。
type LLMMock struct {
	t   *testing.T
	srv *httptest.Server

	mu     sync.Mutex
	queues map[string][]LLMTurn
	dumps  []PromptDump
}

// NewLLMMock starts the fake provider on a loopback port and registers cleanup.
//
// NewLLMMock 在回环端口启动假供应商并注册清理。
func NewLLMMock(t *testing.T) *LLMMock {
	t.Helper()
	m := &LLMMock{t: t, queues: map[string][]LLMTurn{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /models", m.handleModels)
	mux.HandleFunc("POST /chat/completions", m.handleCompletions)
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// URL is the base URL to put on the apikey (provider openai appends /chat/completions).
//
// URL 是放进 apikey 的 base URL（openai provider 自行拼 /chat/completions）。
func (m *LLMMock) URL() string { return m.srv.URL }

// Enqueue scripts the next turns for one model id (FIFO). An exhausted queue serves the
// default turn — scenarios fail on content, not on hangs.
//
// Enqueue 给一个 model id 排下一批 turn（FIFO）。队列耗尽即发默认 turn——场景在内容上失败、
// 不在挂起上失败。
func (m *LLMMock) Enqueue(model string, turns ...LLMTurn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queues[model] = append(m.queues[model], turns...)
}

// Clear drops any unserved turns for one model — sub-scenarios within a test must not
// poison each other with leftovers (e.g. spare scripted failures).
//
// Clear 丢弃某 model 未消费的 turn——同测试内的子场景不得拿残留（如多排的故障帧）毒到彼此。
func (m *LLMMock) Clear(model string) {
	m.mu.Lock()
	delete(m.queues, model)
	m.mu.Unlock()
}

// Dumps returns a copy of every captured request so far.
//
// Dumps 返回至今捕获的全部请求副本。
func (m *LLMMock) Dumps() []PromptDump {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PromptDump, len(m.dumps))
	copy(out, m.dumps)
	return out
}

// DumpsFor returns the captured requests addressed to one model id.
//
// DumpsFor 返回发给某 model id 的捕获请求。
func (m *LLMMock) DumpsFor(model string) []PromptDump {
	var out []PromptDump
	for _, d := range m.Dumps() {
		if d.Model == model {
			out = append(out, d)
		}
	}
	return out
}

// WaitDumps polls until at least n requests hit the given model id.
//
// WaitDumps 轮询直到某 model id 至少收到 n 个请求。
func (m *LLMMock) WaitDumps(t *testing.T, model string, n, timeoutMS int) []PromptDump {
	t.Helper()
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for time.Now().Before(deadline) {
		if ds := m.DumpsFor(model); len(ds) >= n {
			return ds
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("llmmock: model %s never received %d requests (got %d)", model, n, len(m.DumpsFor(model)))
	return nil
}

func (m *LLMMock) handleModels(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Catalog-known ids so capability probing works; scenarios may still use any id.
	// 用目录认识的 id 使能力探测可用；场景仍可用任意 id。
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]string{
			{"id": "gpt-4o"}, {"id": "mock-dialogue"}, {"id": "mock-utility"}, {"id": "mock-agent"},
		},
	})
}

// handleCompletions captures the dump, pops the model's next scripted turn, and streams it
// back in OpenAI SSE chunks (or one non-streaming body when stream=false).
//
// handleCompletions 捕获 dump、弹出该 model 的下一个脚本 turn、按 OpenAI SSE chunk 流回
// （stream=false 时单体返回）。
func (m *LLMMock) handleCompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string          `json:"model"`
		Stream   bool            `json:"stream"`
		Messages json.RawMessage `json:"messages"`
		Tools    []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	raw := json.RawMessage{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		http.Error(w, "bad shape", http.StatusBadRequest)
		return
	}

	dump := PromptDump{Model: req.Model, Raw: raw}
	for _, t := range req.Tools {
		dump.Tools = append(dump.Tools, t.Function.Name)
	}
	var msgs []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	_ = json.Unmarshal(req.Messages, &msgs)
	for _, mm := range msgs {
		dm := DumpMsg{Role: mm.Role, ToolCallID: mm.ToolCallID}
		var s string
		if json.Unmarshal(mm.Content, &s) == nil {
			dm.Content = s
		} else {
			dm.Content = string(mm.Content) // multimodal array — keep raw JSON text. 多模态数组——留原始 JSON 文本。
		}
		for _, tc := range mm.ToolCalls {
			dm.ToolNames = append(dm.ToolNames, tc.Function.Name)
		}
		if mm.Role == "system" && dump.System == "" {
			dump.System = dm.Content
			continue
		}
		dump.Messages = append(dump.Messages, dm)
	}

	m.mu.Lock()
	q := m.queues[req.Model]
	var turn LLMTurn
	if len(q) > 0 {
		turn, m.queues[req.Model] = q[0], q[1:]
	} else {
		turn = LLMTurn{Text: "ok."}
	}
	m.dumps = append(m.dumps, dump)
	m.mu.Unlock()

	if turn.PromptTokens == 0 {
		turn.PromptTokens = 100
	}
	if turn.CompletionTokens == 0 {
		turn.CompletionTokens = 10
	}
	if turn.Status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(turn.Status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "scripted provider failure", "type": "mock_error"},
		})
		return
	}
	if req.Stream {
		m.streamTurn(w, turn)
		return
	}
	m.plainTurn(w, turn)
}

// streamTurn emits the turn as OpenAI SSE chunks: reasoning → text (2 chunks) → tool_calls
// → finish_reason → usage → [DONE].
//
// streamTurn 把 turn 按 OpenAI SSE chunk 发出：reasoning → text（两片）→ tool_calls →
// finish_reason → usage → [DONE]。
func (m *LLMMock) streamTurn(w http.ResponseWriter, turn LLMTurn) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	emit := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	delta := func(d map[string]any) map[string]any {
		return map[string]any{"choices": []map[string]any{{"delta": d}}}
	}

	if turn.Reasoning != "" {
		emit(delta(map[string]any{"reasoning_content": turn.Reasoning}))
	}
	if turn.Text != "" {
		half := len(turn.Text) / 2
		emit(delta(map[string]any{"content": turn.Text[:half]}))
		if turn.StallMS > 0 {
			time.Sleep(time.Duration(turn.StallMS) * time.Millisecond)
		}
		emit(delta(map[string]any{"content": turn.Text[half:]}))
	} else if turn.StallMS > 0 {
		time.Sleep(time.Duration(turn.StallMS) * time.Millisecond)
	}
	finish := "stop"
	if len(turn.ToolCalls) > 0 {
		finish = "tool_calls"
		for i, tc := range turn.ToolCalls {
			args, _ := json.Marshal(tc.Args)
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", i+1)
			}
			emit(delta(map[string]any{"tool_calls": []map[string]any{{
				"index": i, "id": id,
				"function": map[string]any{"name": tc.Name, "arguments": string(args)},
			}}}))
		}
	}
	emit(map[string]any{"choices": []map[string]any{{"delta": map[string]any{}, "finish_reason": finish}}})
	emit(map[string]any{"choices": []map[string]any{}, "usage": map[string]int{
		"prompt_tokens": turn.PromptTokens, "completion_tokens": turn.CompletionTokens,
	}})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// plainTurn emits the turn as one non-streaming completion body.
//
// plainTurn 把 turn 作为单条非流式补全返回。
func (m *LLMMock) plainTurn(w http.ResponseWriter, turn LLMTurn) {
	finish := "stop"
	var calls []map[string]any
	for i, tc := range turn.ToolCalls {
		args, _ := json.Marshal(tc.Args)
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		calls = append(calls, map[string]any{
			"index": i, "id": id, "type": "function",
			"function": map[string]any{"name": tc.Name, "arguments": string(args)},
		})
	}
	if len(calls) > 0 {
		finish = "tool_calls"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"role": "assistant", "content": turn.Text,
				"reasoning_content": turn.Reasoning, "tool_calls": calls,
			},
			"finish_reason": finish,
		}},
		"usage": map[string]int{
			"prompt_tokens": turn.PromptTokens, "completion_tokens": turn.CompletionTokens,
		},
	})
}
