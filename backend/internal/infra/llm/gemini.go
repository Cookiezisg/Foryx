package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"strings"

	modelcapspkg "github.com/sunweilin/forgify/backend/internal/pkg/modelcaps"
)

const geminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// geminiProvider speaks Google's native generateContent dialect: contents/parts
// messages, x-goog-api-key auth, model-in-path streaming, thinkingConfig for
// reasoning. Unlike the OpenAI-compat surface, native carries reasoning text
// back (thought:true parts) plus the thoughtSignature that Gemini-3 multi-turn
// tool loops require — that readback + round-trip is why this provider exists.
//
// geminiProvider 讲 Google 原生 generateContent 方言：contents/parts 消息、
// x-goog-api-key 鉴权、model 在 URL 路径、thinkingConfig 控推理。原生面能读回
// 推理文本（thought:true parts）+ Gemini-3 多轮工具循环必需的 thoughtSignature
// ——这正是它取代 OpenAI-compat 写-only 面的理由。
type geminiProvider struct{}

func newGeminiProvider() *geminiProvider { return &geminiProvider{} }

func (p *geminiProvider) Name() string           { return "google" }
func (p *geminiProvider) DefaultBaseURL() string { return geminiDefaultBaseURL }

// BuildRequest encodes a Request into a native generateContent HTTP request.
// The model lives in the URL PATH (base + /models/{model}:streamGenerateContent
// ?alt=sse), not the body — Gemini's per-method REST shape. Auth is the
// x-goog-api-key header. systemInstruction, contents, tools.functionDeclarations
// and generationConfig.thinkingConfig are mapped from the provider-neutral
// Request per 03 §5.
//
// BuildRequest 把 Request 编码为原生 generateContent HTTP 请求。model 在 URL
// 路径（base + /models/{model}:streamGenerateContent?alt=sse）而非 body——
// 这是 Gemini 的 per-method REST 形态。鉴权用 x-goog-api-key 头。按 03 §5 映射
// systemInstruction / contents / tools / thinkingConfig。
func (p *geminiProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	// TE-25: sanitize tool_call ↔ tool_result pairing before mapping; an orphan
	// functionResponse with no matching functionCall makes Gemini 400.
	// TE-25：映射前先配对 sanitize；孤儿 functionResponse 会让 Gemini 400。
	req.Messages = SanitizeMessages(req.Messages)

	body := geminiRequest{
		Contents: toGeminiContents(req.Messages),
	}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.System}},
		}
	}
	if len(req.Tools) > 0 {
		body.Tools = []geminiTool{{FunctionDeclarations: toGeminiFunctionDeclarations(req.Tools)}}
	}
	// Always send maxOutputTokens = the model's real cap. Gemini's default is a
	// truncating ~8192 (and thinking counts against the same budget), so omitting
	// it silently caps long generations. Unknown models use a generous modelcaps fallback.
	//
	// 始终发 maxOutputTokens = 模型真实上限。Gemini 默认 ~8192（且 thinking 计入同一
	// 预算）会静默截断长输出，省略即被腰斩。未知模型用 modelcaps 宽松兜底。
	gc := &geminiGenerationConfig{}
	if maxOut := modelcapspkg.Lookup("google", req.ModelID).MaxOutput; maxOut > 0 {
		gc.MaxOutputTokens = &maxOut
	}
	gc.ThinkingConfig = encodeGeminiThinking(req.ModelID, req.Thinking)
	if gc.MaxOutputTokens != nil || gc.ThinkingConfig != nil {
		body.GenerationConfig = gc
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.google: marshal body: %w", err)
	}

	method := "streamGenerateContent?alt=sse"
	if req.DisableStream {
		method = "generateContent"
	}
	url := fmt.Sprintf("%s/models/%s:%s", req.BaseURL, req.ModelID, method)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.google: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", req.Key)
	return httpReq, nil
}

// ParseStream reads native generateContent SSE chunks and yields StreamEvents.
// Each data: line is a full GenerateContentResponse chunk. A thought:true part
// → EventReasoning (carrying thoughtSignature on Signature, like Anthropic); a
// plain text part → EventText; a functionCall part → EventToolStart +
// EventToolDelta (Gemini sends the COMPLETE call, not deltas, so the whole
// args object is one EventToolDelta). usageMetadata + finishReason → EventFinish.
//
// ParseStream 读原生 generateContent SSE chunk 并 yield StreamEvent。每条 data:
// 是一个完整 GenerateContentResponse chunk。thought:true part→EventReasoning
//（Signature 带 thoughtSignature，仿 Anthropic）；text part→EventText；
// functionCall part→EventToolStart+EventToolDelta（Gemini 发完整调用非增量，
// 整个 args 一次性 emit）。usageMetadata + finishReason→EventFinish。
func (p *geminiProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseGeminiNonStreaming(resp.Body, yield)
			return
		}
		// toolIdx assigns a stream-local index per functionCall part; Gemini does
		// not number tool calls, so we count emission order.
		// toolIdx 给每个 functionCall part 分配流内序号；Gemini 不编号工具调用，按出现顺序计数。
		toolIdx := 0
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk geminiResponse
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.google: malformed SSE chunk: %w", err)})
				return false
			}
			return emitGeminiChunk(chunk, &toolIdx, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.google: scan: %w", scanErr)})
		}
	}
}

// emitGeminiChunk converts one GenerateContentResponse chunk to StreamEvents.
// Walks candidates[0].content.parts in order; emits a single EventFinish at the
// end of any chunk that carries finishReason and/or usageMetadata.
//
// emitGeminiChunk 把一个 GenerateContentResponse chunk 转为 StreamEvent 序列。
// 按序遍历 candidates[0].content.parts；带 finishReason / usageMetadata 的 chunk
// 末尾 emit 一个 EventFinish。
func emitGeminiChunk(chunk geminiResponse, toolIdx *int, yield func(StreamEvent) bool) bool {
	var cand *geminiCandidate
	if len(chunk.Candidates) > 0 {
		cand = &chunk.Candidates[0]
	}

	if cand != nil {
		for _, part := range cand.Content.Parts {
			switch {
			case part.FunctionCall != nil:
				if !emitGeminiFunctionCall(*part.FunctionCall, toolIdx, yield) {
					return false
				}
			case part.Thought:
				// Reasoning part: emit text (if any) then signature (if any) so the
				// consumer stores both. A signature-only thought part still round-trips.
				// 推理 part：先 emit 文本再 emit 签名，让消费者两者都存；纯签名 part 也能回传。
				if part.Text != "" {
					if !yield(StreamEvent{Type: EventReasoning, Delta: part.Text}) {
						return false
					}
				}
				if part.ThoughtSignature != "" {
					if !yield(StreamEvent{Type: EventReasoning, Signature: part.ThoughtSignature}) {
						return false
					}
				}
			case part.Text != "":
				if !yield(StreamEvent{Type: EventText, Delta: part.Text}) {
					return false
				}
			}
		}
	}

	// Finish: Gemini puts finishReason on the candidate and usageMetadata at the
	// top level; both can arrive on the same final chunk.
	// finish：finishReason 在 candidate 上、usageMetadata 在顶层；常同在末尾 chunk。
	hasFinish := cand != nil && cand.FinishReason != ""
	if hasFinish || chunk.UsageMetadata != nil {
		ev := StreamEvent{Type: EventFinish}
		if cand != nil {
			ev.FinishReason = cand.FinishReason
		}
		if chunk.UsageMetadata != nil {
			ev.InputTokens = chunk.UsageMetadata.PromptTokenCount
			// Native splits visible-output vs thinking tokens; sum both into
			// OutputTokens so accounting matches the other providers' totals.
			// 原生把可见输出与思考 token 分列；合计为 OutputTokens，与其他家口径一致。
			ev.OutputTokens = chunk.UsageMetadata.CandidatesTokenCount + chunk.UsageMetadata.ThoughtsTokenCount
		}
		return yield(ev)
	}
	return true
}

// emitGeminiFunctionCall emits one complete functionCall as EventToolStart +
// EventToolDelta(full args). Gemini-3 returns a unique id per call; when absent
// (older models) we fall back to a synthetic stream-local id so downstream
// pairing still works.
//
// emitGeminiFunctionCall 把一个完整 functionCall emit 为 EventToolStart +
// EventToolDelta（整段 args）。Gemini-3 每调用带唯一 id；缺失（旧模型）时
// 合成流内 id，保证下游配对可用。
func emitGeminiFunctionCall(fc geminiFunctionCall, toolIdx *int, yield func(StreamEvent) bool) bool {
	idx := *toolIdx
	*toolIdx++

	id := fc.ID
	if id == "" {
		id = fmt.Sprintf("gemini_call_%d", idx)
	}
	if !yield(StreamEvent{Type: EventToolStart, ToolIndex: idx, ToolID: id, ToolName: fc.Name}) {
		return false
	}
	args := fc.Args
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	return yield(StreamEvent{Type: EventToolDelta, ToolIndex: idx, ArgsDelta: string(args)})
}

// parseGeminiNonStreaming reads one non-streaming generateContent JSON response
// and synthesizes StreamEvents, mirroring the streaming part walk.
//
// parseGeminiNonStreaming 读单条非流式 generateContent JSON 响应并合成
// StreamEvent，与流式 part 遍历逻辑一致。
func parseGeminiNonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20))
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.google: read non-streaming body: %w", err)})
		return
	}
	var resp geminiResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.google: parse non-streaming response: %w", err)})
		return
	}
	toolIdx := 0
	emitGeminiChunk(resp, &toolIdx, yield)
}

// ── Request mapping ──────────────────────────────────────────────────────────

// toGeminiContents maps provider-neutral messages onto Gemini contents.
// Role map: user→"user", assistant→"model", tool→"user" with a functionResponse
// part. Consecutive tool messages merge into one user content (Gemini accepts
// multiple functionResponse parts in one turn), mirroring the Anthropic path.
//
// toGeminiContents 把中立消息映射为 Gemini contents。角色映射：user→"user"、
// assistant→"model"、tool→"user"（functionResponse part）。连续 tool 消息合并
// 为一条 user content（Gemini 允许一回合多 functionResponse），仿 Anthropic。
func toGeminiContents(msgs []LLMMessage) []geminiContent {
	// nameByCallID lets a tool turn recover the function NAME it is responding to;
	// Gemini keys functionResponse by name (+ id), but our tool message only
	// carries the call id, so we resolve the name from the preceding tool_call.
	// nameByCallID 让 tool 回合找回所应答的函数名；Gemini 的 functionResponse 按
	// name(+id) 配对，而我们的 tool 消息只带 call id，故从前序 tool_call 反查名字。
	nameByCallID := map[string]string{}
	for _, m := range msgs {
		if m.Role == RoleAssistant {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					nameByCallID[tc.ID] = tc.Name
				}
			}
		}
	}

	var out []geminiContent
	for i := 0; i < len(msgs); {
		m := msgs[i]
		if m.Role == RoleTool {
			var parts []geminiPart
			for i < len(msgs) && msgs[i].Role == RoleTool {
				parts = append(parts, geminiToolResponsePart(msgs[i], nameByCallID))
				i++
			}
			out = append(out, geminiContent{Role: "user", Parts: parts})
			continue
		}
		out = append(out, toGeminiContent(m))
		i++
	}
	return out
}

func toGeminiContent(m LLMMessage) geminiContent {
	switch m.Role {
	case RoleAssistant:
		return geminiContent{Role: "model", Parts: geminiAssistantParts(m)}
	default: // RoleUser
		return geminiContent{Role: "user", Parts: geminiUserParts(m)}
	}
}

func geminiUserParts(m LLMMessage) []geminiPart {
	if len(m.Parts) == 0 {
		return []geminiPart{{Text: m.Content}}
	}
	parts := make([]geminiPart, 0, len(m.Parts))
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			parts = append(parts, geminiPart{Text: p.Text})
		case "image_url":
			// data: URL → inlineData(base64); remote URL → fileData(uri). Reuses the
			// Anthropic data-URL helpers for the base64 split.
			// data: URL→inlineData(base64)；远程 URL→fileData(uri)。复用 Anthropic 的 data-URL 解析。
			if isDataURL(p.ImageURL) {
				parts = append(parts, geminiPart{InlineData: &geminiInlineData{
					MimeType: extractMediaType(p.ImageURL),
					Data:     extractBase64Data(p.ImageURL),
				}})
			} else {
				parts = append(parts, geminiPart{FileData: &geminiFileData{
					MimeType: "image/jpeg",
					FileURI:  p.ImageURL,
				}})
			}
		}
	}
	return parts
}

func isDataURL(s string) bool { return strings.HasPrefix(s, "data:") }

// geminiAssistantParts builds the "model" content: reasoning (with signature)
// first so the thoughtSignature round-trips, then tool calls as functionCall
// parts, then any plain text. Mirrors the Anthropic assistant ordering.
//
// geminiAssistantParts 构造 "model" content：先 reasoning（带签名）以回传
// thoughtSignature，再 tool 调用（functionCall part），最后纯文本。仿 Anthropic 顺序。
func geminiAssistantParts(m LLMMessage) []geminiPart {
	var parts []geminiPart
	if m.ReasoningContent != "" || m.ReasoningSignature != "" {
		parts = append(parts, geminiPart{
			Text:             m.ReasoningContent,
			Thought:          true,
			ThoughtSignature: m.ReasoningSignature,
		})
	}
	for _, tc := range m.ToolCalls {
		args := json.RawMessage("{}")
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				slog.Warn("llm.google: history tool-call arguments are malformed JSON, falling back to {}",
					"tool_call_id", tc.ID, "tool_name", tc.Name, "raw", tc.Arguments, "err", err)
				args = json.RawMessage("{}")
			}
		}
		parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: args,
		}})
	}
	if m.Content != "" {
		parts = append(parts, geminiPart{Text: m.Content})
	}
	return parts
}

// geminiToolResponsePart maps a tool message to a functionResponse part.
// Gemini keys the response by function NAME (+ id when present); we resolve the
// name from the matching tool_call via nameByCallID. The response must be a JSON
// object — raw string output is wrapped as {"result": <text>}.
//
// geminiToolResponsePart 把 tool 消息映射为 functionResponse part。Gemini 按
// 函数名(+id)配对，名字经 nameByCallID 从对应 tool_call 反查。response 必须是
// JSON object——裸字符串输出包装为 {"result": <text>}。
func geminiToolResponsePart(m LLMMessage, nameByCallID map[string]string) geminiPart {
	name := nameByCallID[m.ToolCallID]
	if name == "" {
		// Best-effort fallback: no matching tool_call name in history. Use the call
		// id as the name so the part is still well-formed; Gemini pairs on id too.
		// 兜底：历史中找不到对应 tool_call 名字，用 call id 充当 name 保证 part 合法；
		// Gemini 也按 id 配对。
		name = m.ToolCallID
	}
	return geminiPart{FunctionResponse: &geminiFunctionResponse{
		ID:       m.ToolCallID,
		Name:     name,
		Response: wrapGeminiToolResponse(m.Content),
	}}
}

// wrapGeminiToolResponse ensures the response is a JSON object. Tool output is
// usually a plain string; if it already parses as a JSON object pass it through,
// otherwise wrap it as {"result": <raw>}.
//
// wrapGeminiToolResponse 保证 response 为 JSON object。tool 输出通常是纯字符串；
// 若已是 JSON object 则透传，否则包装为 {"result": <raw>}。
func wrapGeminiToolResponse(content string) json.RawMessage {
	trimmed := bytes.TrimSpace([]byte(content))
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var probe map[string]json.RawMessage
		if json.Unmarshal(trimmed, &probe) == nil {
			return trimmed
		}
	}
	wrapped, _ := json.Marshal(map[string]string{"result": content})
	return wrapped
}

func toGeminiFunctionDeclarations(defs []ToolDef) []geminiFunctionDeclaration {
	out := make([]geminiFunctionDeclaration, len(defs))
	for i, d := range defs {
		// ToolDef and geminiFunctionDeclaration share name/description/parameters;
		// the conversion keeps the Gemini JSON tags. Add a field to either and this
		// breaks loudly — a good signal to revisit the mapping.
		// 两者共享 name/description/parameters，转换保留 Gemini 的 JSON tag；
		// 任一端加字段会在此处编译报错，正好提示重审映射。
		out[i] = geminiFunctionDeclaration(d)
	}
	return out
}

// encodeGeminiThinking maps the neutral ThinkingSpec to thinkingConfig (03 §5).
//   - on  → {thinkingBudget: Budget-or-default, includeThoughts:true}. Default
//     budget is the model's BudgetMax (clamped sane); thinkingBudget int form
//     works across 2.5 (and -1 dynamic / 0 off where allowed).
//   - off → {thinkingBudget:0}. 2.5-pro and 3.x can't fully disable; sending 0 is
//     still valid (the model floors it to its minimum), so we send what's valid.
//   - nil/auto → omit thinkingConfig entirely (lets the model self-pace).
//
// encodeGeminiThinking 把中立 ThinkingSpec 映射为 thinkingConfig（03 §5）：
// on→{thinkingBudget, includeThoughts:true}（默认取模型 BudgetMax，整数形跨 2.5
// 通用）；off→{thinkingBudget:0}（2.5-pro/3.x 不可全关，发 0 仍合法）；
// nil/auto→省略 thinkingConfig。
func encodeGeminiThinking(modelID string, spec *ThinkingSpec) *geminiThinkingConfig {
	if spec == nil || spec.Mode == "auto" {
		return nil
	}
	switch spec.Mode {
	case "on":
		budget := spec.Budget
		if budget == 0 {
			cap := modelcapspkg.Lookup("google", modelID)
			budget = cap.BudgetMax
			if budget == 0 {
				budget = 8192 // sane default when the model has no budget ceiling on record
			}
		}
		return &geminiThinkingConfig{ThinkingBudget: &budget, IncludeThoughts: true}
	case "off":
		zero := 0
		return &geminiThinkingConfig{ThinkingBudget: &zero}
	}
	return nil
}

// ── Native wire types ────────────────────────────────────────────────────────
//
// These are Gemini's own generateContent request/response shapes, typed here so
// gemini.go reads end-to-end as the complete Gemini story (no shared OpenAI
// structs). Field names follow the v1beta REST schema (camelCase JSON).
//
// 这些是 Gemini 原生 generateContent 请求/响应结构，本文件内定义以便端到端
// 自洽（不复用 OpenAI 结构）。字段名遵循 v1beta REST schema（camelCase）。

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart is one element of a content's parts array. A part carries exactly
// one of: text, inlineData, fileData, functionCall, functionResponse. Thought +
// ThoughtSignature accompany a reasoning text part (response side) and are
// echoed back on the assistant turn (request side).
//
// geminiPart 是 content.parts 的一个元素，互斥地携带 text / inlineData /
// fileData / functionCall / functionResponse 之一。Thought + ThoughtSignature
// 伴随推理文本 part（响应侧），并在 assistant 回合原样回传（请求侧）。
type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FileData         *geminiFileData         `json:"fileData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

// geminiFunctionCall is the model's tool invocation. Args is the parsed JSON
// object (not a string, unlike OpenAI's arguments). ID is unique per call on
// Gemini-3 and must be echoed in the matching functionResponse.
//
// geminiFunctionCall 是模型的工具调用。Args 是解析后的 JSON object（非字符串，
// 与 OpenAI 的 arguments 不同）。ID 在 Gemini-3 每调用唯一，须在对应
// functionResponse 中回传。
type geminiFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// geminiFunctionResponse carries a tool result back. Gemini pairs it to the call
// by Name (+ ID when present); Response must be a JSON object.
//
// geminiFunctionResponse 回传工具结果。Gemini 按 Name(+ID)配对；Response 必须
// 是 JSON object。
type geminiFunctionResponse struct {
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenerationConfig struct {
	// MaxOutputTokens is sent explicitly because Gemini's default (~8192, shared
	// with the thinking budget) silently truncates long output; pointer so 0 elides.
	//
	// MaxOutputTokens 显式发送：Gemini 默认 ~8192（且与 thinking 预算共享）会静默截断
	// 长输出；指针使 0 被 omitempty 省略。
	MaxOutputTokens *int                  `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

// geminiThinkingConfig is the native thinking knob. ThinkingBudget is a pointer
// so budget 0 (explicit off) serializes instead of being elided by omitempty.
//
// geminiThinkingConfig 是原生 thinking 旋钮。ThinkingBudget 用指针，使 budget 0
//（显式关闭）能被序列化而不被 omitempty 吞掉。
type geminiThinkingConfig struct {
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
}
