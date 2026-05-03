// anthropic.go — Anthropic native streaming client (/v1/messages).
// Uses content_block_start/delta/stop events which map cleanly to our Block
// model and give explicit signals when each tool call's arguments are complete.
//
// anthropic.go — Anthropic 原生流式客户端（/v1/messages）。
// content_block_start/delta/stop 事件与我们的 Block 模型天然对应，
// 并在每个 tool call arguments 完整时给出明确信号。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"strings"
)

const (
	anthropicVersion          = "2023-06-01"
	anthropicMessagesPath     = "/v1/messages"
	anthropicDefaultMaxTokens = 8096
)

// anthropicClient implements Client for the Anthropic native API.
//
// anthropicClient 为 Anthropic 原生 API 实现 Client 接口。
type anthropicClient struct {
	http *http.Client
}

func newAnthropicClient() *anthropicClient {
	return &anthropicClient{http: newOpenAIClient().http}
}

// Stream sends a streaming /v1/messages request to the Anthropic API.
//
// Stream 向 Anthropic API 发起流式 /v1/messages 请求。
func (c *anthropicClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		body, err := buildAnthropicBody(req)
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: build body: %w", err)})
			return
		}

		httpReq, err := http.NewRequestWithContext(
			ctx, http.MethodPost, req.BaseURL+anthropicMessagesPath, bytes.NewReader(body))
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: new request: %w", err)})
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", req.Key)
		httpReq.Header.Set("anthropic-version", anthropicVersion)

		resp, err := c.http.Do(httpReq)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: do: %w", err)})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			yield(StreamEvent{Type: EventError, Err: classifyHTTPError(resp.StatusCode, raw)})
			return
		}

		parseAnthropicSSE(ctx, resp.Body, yield)
	}
}

// ── SSE parser ────────────────────────────────────────────────────────────────

// parseAnthropicSSE reads the Anthropic named-event SSE stream.
// Anthropic uses "event: <name>\ndata: <json>" pairs separated by blank lines.
//
// parseAnthropicSSE 读取 Anthropic 命名事件 SSE 流。
// 格式为 "event: <name>\ndata: <json>" 对，由空行分隔。
func parseAnthropicSSE(ctx context.Context, body io.Reader, yield func(StreamEvent) bool) {
	scanner := bufio.NewScanner(body)
	var eventName string
	var inputTokens, outputTokens int

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()

		if name, ok := strings.CutPrefix(line, "event: "); ok {
			eventName = name
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}

		switch eventName {
		case "message_start":
			var e anthropicMsgStart
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: parse message_start: %w", err)})
				return
			}
			if e.Message.Usage != nil {
				inputTokens = e.Message.Usage.InputTokens
			}

		case "content_block_start":
			var e anthropicBlockStart
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: parse content_block_start: %w", err)})
				return
			}
			if e.ContentBlock.Type == "tool_use" {
				if !yield(StreamEvent{
					Type:      EventToolStart,
					ToolIndex: e.Index,
					ToolID:    e.ContentBlock.ID,
					ToolName:  e.ContentBlock.Name,
				}) {
					return
				}
			}

		case "content_block_delta":
			var e anthropicBlockDelta
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: parse content_block_delta: %w", err)})
				return
			}
			if !emitAnthropicDelta(e, yield) {
				return
			}

		case "message_delta":
			var e anthropicMsgDelta
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: parse message_delta: %w", err)})
				return
			}
			if e.Usage != nil {
				outputTokens = e.Usage.OutputTokens
			}
			if e.Delta.StopReason != "" {
				if !yield(StreamEvent{
					Type:         EventFinish,
					FinishReason: e.Delta.StopReason,
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
				}) {
					return
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: scan: %w", err)})
	}
}

func emitAnthropicDelta(e anthropicBlockDelta, yield func(StreamEvent) bool) bool {
	switch e.Delta.Type {
	case "text_delta":
		if e.Delta.Text != "" {
			return yield(StreamEvent{Type: EventText, Delta: e.Delta.Text})
		}
	case "thinking_delta":
		if e.Delta.Thinking != "" {
			return yield(StreamEvent{Type: EventReasoning, Delta: e.Delta.Thinking})
		}
	case "input_json_delta":
		if e.Delta.PartialJSON != "" {
			return yield(StreamEvent{
				Type:      EventToolDelta,
				ToolIndex: e.Index,
				ArgsDelta: e.Delta.PartialJSON,
			})
		}
	}
	return true
}

// ── Request builder ───────────────────────────────────────────────────────────

func buildAnthropicBody(req Request) ([]byte, error) {
	msgs, err := toAnthropicMsgs(req.Messages)
	if err != nil {
		return nil, err
	}
	body := anthropicRequest{
		Model:     req.ModelID,
		MaxTokens: anthropicDefaultMaxTokens,
		Messages:  msgs,
		Stream:    true,
	}
	if req.System != "" {
		body.System = req.System
	}
	if len(req.Tools) > 0 {
		body.Tools = toAnthropicTools(req.Tools)
	}
	return json.Marshal(body)
}

// toAnthropicMsgs converts LLMMessages to Anthropic format.
// Consecutive RoleTool messages are grouped into a single user message with
// tool_result content blocks, as required by the Anthropic API.
//
// toAnthropicMsgs 把 LLMMessage 列表转为 Anthropic 格式。
// 连续的 RoleTool 消息被合并为一条 user 消息的 tool_result content blocks，
// 这是 Anthropic API 的要求。
func toAnthropicMsgs(msgs []LLMMessage) ([]anthropicMessage, error) {
	var out []anthropicMessage
	for i := 0; i < len(msgs); {
		m := msgs[i]
		if m.Role == RoleTool {
			// Collect all consecutive tool results into one user message.
			// 把连续的 tool result 合并进一条 user 消息。
			var blocks []anthropicContent
			for i < len(msgs) && msgs[i].Role == RoleTool {
				blocks = append(blocks, anthropicContent{
					Type:      "tool_result",
					ToolUseID: msgs[i].ToolCallID,
					Content:   msgs[i].Content,
				})
				i++
			}
			out = append(out, anthropicMessage{Role: "user", Content: blocks})
		} else {
			am, err := toAnthropicMsg(m)
			if err != nil {
				return nil, err
			}
			out = append(out, am)
			i++
		}
	}
	return out, nil
}

func toAnthropicMsg(m LLMMessage) (anthropicMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildAnthropicUserMsg(m), nil
	case RoleAssistant:
		return buildAnthropicAssistantMsg(m), nil
	default:
		return anthropicMessage{}, fmt.Errorf("llm/anthropic: unexpected role %q in toAnthropicMsg", m.Role)
	}
}

func buildAnthropicUserMsg(m LLMMessage) anthropicMessage {
	if len(m.Parts) == 0 {
		return anthropicMessage{
			Role:    "user",
			Content: []anthropicContent{{Type: "text", Text: m.Content}},
		}
	}
	blocks := make([]anthropicContent, 0, len(m.Parts))
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			blocks = append(blocks, anthropicContent{Type: "text", Text: p.Text})
		case "image_url":
			blocks = append(blocks, anthropicContent{
				Type: "image",
				Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: extractMediaType(p.ImageURL),
					Data:      extractBase64Data(p.ImageURL),
				},
			})
		}
	}
	return anthropicMessage{Role: "user", Content: blocks}
}

func buildAnthropicAssistantMsg(m LLMMessage) anthropicMessage {
	var blocks []anthropicContent
	if m.ReasoningContent != "" {
		blocks = append(blocks, anthropicContent{
			Type:     "thinking",
			Thinking: m.ReasoningContent,
		})
	}
	for _, tc := range m.ToolCalls {
		// History tool-call arguments came from an earlier LLM turn that we
		// persisted; bad JSON here means upstream stored garbage. Fall back
		// to "{}" so Anthropic's tool_use schema is satisfied (input is
		// required JSON), but log loudly so the corruption is traceable.
		//
		// 历史 tool-call 的 arguments 来自更早 LLM 轮次落库的内容；这里 JSON
		// 不合法说明上游存了脏数据。回退 "{}" 满足 Anthropic tool_use schema
		// 必填的要求，同时高声记录以便溯源。
		input := json.RawMessage("{}")
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
				slog.Warn("llm/anthropic: history tool-call arguments are malformed JSON, falling back to {}",
					"tool_call_id", tc.ID, "tool_name", tc.Name, "raw", tc.Arguments, "err", err)
				input = json.RawMessage("{}")
			}
		}
		blocks = append(blocks, anthropicContent{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	if m.Content != "" {
		blocks = append(blocks, anthropicContent{Type: "text", Text: m.Content})
	}
	return anthropicMessage{Role: "assistant", Content: blocks}
}

func toAnthropicTools(defs []ToolDef) []anthropicTool {
	out := make([]anthropicTool, len(defs))
	for i, d := range defs {
		out[i] = anthropicTool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.Parameters,
		}
	}
	return out
}

// extractMediaType pulls the MIME type from a base64 data URL.
// Falls back to "image/jpeg" if the URL is not a data URL.
//
// extractMediaType 从 base64 data URL 提取 MIME 类型，非 data URL 时兜底为 image/jpeg。
func extractMediaType(dataURL string) string {
	if !strings.HasPrefix(dataURL, "data:") {
		return "image/jpeg"
	}
	rest := strings.TrimPrefix(dataURL, "data:")
	if idx := strings.Index(rest, ";"); idx > 0 {
		return rest[:idx]
	}
	return "image/jpeg"
}

// extractBase64Data strips the data URL prefix and returns only the base64 payload.
//
// extractBase64Data 去掉 data URL 前缀，仅返回 base64 数据。
func extractBase64Data(dataURL string) string {
	if _, data, ok := strings.Cut(dataURL, ","); ok {
		return data
	}
	return dataURL
}

// ── Wire types ────────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type      string                `json:"type"`
	Text      string                `json:"text,omitempty"`
	Thinking  string                `json:"thinking,omitempty"`
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Input     json.RawMessage       `json:"input,omitempty"`
	ToolUseID string                `json:"tool_use_id,omitempty"`
	Content   string                `json:"content,omitempty"`
	Source    *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ── SSE event structs ─────────────────────────────────────────────────────────

type anthropicMsgStart struct {
	Message struct {
		Usage *struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthropicBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"content_block"`
}

type anthropicBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

type anthropicMsgDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
