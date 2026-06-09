package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strconv"
	"strings"
)

const geminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// geminiProvider speaks Google's native generateContent dialect, fully self-contained:
// contents/parts messages, x-goog-api-key auth, model-in-path streaming, thinkingConfig
// for reasoning. Native carries reasoning text back (thought:true parts) plus the
// thoughtSignature that Gemini-3 multi-turn tool loops require — that readback +
// round-trip is why this provider exists. Nothing here is shared with other providers'
// wire (only scanSSELines, which is the SSE line format itself).
//
// geminiProvider 完整自包含地讲 Google 原生 generateContent 方言：contents/parts 消息、
// x-goog-api-key 鉴权、model 在 URL 路径、thinkingConfig 控推理。原生面能读回推理文本
// （thought:true parts）+ Gemini-3 多轮工具循环必需的 thoughtSignature。与其他家 wire
// 不共享（仅用 scanSSELines，那是 SSE 行格式本身）。
type geminiProvider struct{}

func newGeminiProvider() *geminiProvider { return &geminiProvider{} }

func (p *geminiProvider) Name() string           { return "google" }
func (p *geminiProvider) DefaultBaseURL() string { return geminiDefaultBaseURL }

// BuildRequest encodes a Request into a native generateContent HTTP request. The model
// lives in the URL PATH (base + /models/{model}:streamGenerateContent?alt=sse), not the
// body — Gemini's per-method REST shape. Auth is x-goog-api-key.
//
// BuildRequest 把 Request 编码为原生 generateContent 请求。model 在 URL 路径
// （base + /models/{model}:streamGenerateContent?alt=sse）而非 body。鉴权用 x-goog-api-key。
func (p *geminiProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	// Sanitize tool_call ↔ tool_result pairing first; an orphan functionResponse 400s Gemini.
	// 先配对 sanitize；孤儿 functionResponse 会让 Gemini 400。
	req.Messages = SanitizeMessages(req.Messages)

	body := geminiRequest{Contents: toGeminiContents(req.Messages)}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	if len(req.Tools) > 0 {
		body.Tools = []geminiTool{{FunctionDeclarations: toGeminiFunctionDeclarations(req.Tools)}}
	}
	// maxOutputTokens from Request.MaxTokens when set; Gemini's default (~8192, shared with the
	// thinking budget) silently truncates long output, so a caller wanting long output sets it.
	//
	// maxOutputTokens 取 Request.MaxTokens（非零时）；Gemini 默认 ~8192（且与 thinking 预算共享）
	// 会静默截断长输出，要长输出的 caller 自行设定。
	gc := &geminiGenerationConfig{}
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		gc.MaxOutputTokens = &mt
	}
	gc.ThinkingConfig = encodeGeminiThinking(req.Options)
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

// ParseStream reads native generateContent SSE chunks and yields StreamEvents. Each data:
// line is a full GenerateContentResponse chunk: thought:true part → EventReasoning
// (with thoughtSignature on Signature); plain text → EventText; functionCall → EventToolStart
// + EventToolDelta (Gemini sends the COMPLETE call, so the whole args object is one delta);
// usageMetadata + finishReason → EventFinish.
//
// ParseStream 读原生 generateContent SSE chunk。每条 data: 是完整 GenerateContentResponse：
// thought:true part→EventReasoning（Signature 带 thoughtSignature）；text→EventText；
// functionCall→EventToolStart+EventToolDelta（Gemini 发完整调用，整段 args 一次）；
// usageMetadata+finishReason→EventFinish。
func (p *geminiProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseGeminiNonStreaming(resp.Body, yield)
			return
		}
		// Gemini does not number tool calls; count emission order for a stream-local index.
		// Gemini 不编号工具调用，按出现顺序计数作流内序号。
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
				// Reasoning part: text (if any) then signature (if any), so the consumer
				// stores both for verbatim round-trip. A signature-only thought still works.
				// 推理 part：先文本再签名，让消费者两者都存以原样回传；纯签名 part 也行。
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

	// Finish: finishReason on the candidate, usageMetadata at the top level; both can
	// arrive on the same final chunk.
	// finish：finishReason 在 candidate、usageMetadata 在顶层；常同在末尾 chunk。
	hasFinish := cand != nil && cand.FinishReason != ""
	if hasFinish || chunk.UsageMetadata != nil {
		ev := StreamEvent{Type: EventFinish}
		if cand != nil {
			ev.FinishReason = cand.FinishReason
		}
		if chunk.UsageMetadata != nil {
			ev.InputTokens = chunk.UsageMetadata.PromptTokenCount
			// Native splits visible-output vs thinking tokens; sum both so accounting
			// matches the other providers' OutputTokens totals.
			// 原生把可见输出与思考 token 分列；合计为 OutputTokens，与其他家口径一致。
			ev.OutputTokens = chunk.UsageMetadata.CandidatesTokenCount + chunk.UsageMetadata.ThoughtsTokenCount
		}
		return yield(ev)
	}
	return true
}

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

// ── request mapping ──────────────────────────────────────────────────────────

// toGeminiContents maps neutral messages onto Gemini contents. Roles: user→"user",
// assistant→"model", tool→"user" with a functionResponse part. Consecutive tool messages
// merge into one user content (Gemini accepts multiple functionResponse parts per turn).
//
// toGeminiContents 把中立消息映射为 Gemini contents。角色：user→"user"、assistant→"model"、
// tool→"user"（functionResponse part）。连续 tool 合并为一条 user content。
func toGeminiContents(msgs []LLMMessage) []geminiContent {
	// Gemini keys functionResponse by function NAME (+id); our tool message only carries
	// the call id, so recover the name from the preceding tool_call.
	// Gemini 的 functionResponse 按 name(+id) 配对，而 tool 消息只带 call id，从前序 tool_call 反查名字。
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
	if m.Role == RoleAssistant {
		return geminiContent{Role: "model", Parts: geminiAssistantParts(m)}
	}
	return geminiContent{Role: "user", Parts: geminiUserParts(m)}
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
			parts = append(parts, geminiImagePart(p.ImageURL))
		case "file":
			// PDF / document → inlineData (Gemini reads PDFs natively as inline base64).
			// PDF/文档 → inlineData（Gemini 原生以内联 base64 读 PDF）。
			parts = append(parts, geminiPart{InlineData: &geminiInlineData{MimeType: p.MediaType, Data: p.Data}})
		}
	}
	return parts
}

// geminiImagePart maps an image URL to an inlineData (base64 data: URL) or fileData
// (remote URL) part. Self-contained data-URL parsing — no dependency on other providers.
//
// geminiImagePart 把 image URL 映射为 inlineData（base64 data: URL）或 fileData（远程 URL）
// part。自包含的 data-URL 解析——不依赖其他 provider。
func geminiImagePart(url string) geminiPart {
	if strings.HasPrefix(url, "data:") {
		mime := "image/jpeg"
		rest := strings.TrimPrefix(url, "data:")
		if i := strings.Index(rest, ";"); i > 0 {
			mime = rest[:i]
		}
		data := url
		if _, d, ok := strings.Cut(url, ","); ok {
			data = d
		}
		return geminiPart{InlineData: &geminiInlineData{MimeType: mime, Data: data}}
	}
	return geminiPart{FileData: &geminiFileData{MimeType: "image/jpeg", FileURI: url}}
}

// geminiAssistantParts builds "model" content: reasoning (with signature) first so the
// thoughtSignature round-trips, then tool calls, then plain text.
//
// geminiAssistantParts 构造 "model" content：先 reasoning（带签名）以回传 thoughtSignature，
// 再 tool 调用，最后纯文本。
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
		// Malformed persisted args → fall back to "{}" silently (history corruption must
		// not 400 the live turn).
		// 历史里 args JSON 烂了 → 静默回退 "{}"（历史损坏不该让当前回合 400）。
		args := json.RawMessage("{}")
		if tc.Arguments != "" && json.Valid([]byte(tc.Arguments)) {
			args = json.RawMessage(tc.Arguments)
		}
		parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{ID: tc.ID, Name: tc.Name, Args: args}})
	}
	if m.Content != "" {
		parts = append(parts, geminiPart{Text: m.Content})
	}
	return parts
}

// geminiToolResponsePart maps a tool message to a functionResponse part; the name is
// resolved from the matching tool_call (Gemini pairs by name, falling back to id).
//
// geminiToolResponsePart 把 tool 消息映射为 functionResponse part；name 从对应 tool_call
// 反查（Gemini 按 name 配对，回退 id）。
func geminiToolResponsePart(m LLMMessage, nameByCallID map[string]string) geminiPart {
	name := nameByCallID[m.ToolCallID]
	if name == "" {
		name = m.ToolCallID // best-effort: Gemini also pairs on id
	}
	return geminiPart{FunctionResponse: &geminiFunctionResponse{
		ID:       m.ToolCallID,
		Name:     name,
		Response: wrapGeminiToolResponse(m.Content),
	}}
}

// wrapGeminiToolResponse ensures the response is a JSON object: pass a JSON object
// through, otherwise wrap a plain string as {"result": <raw>}.
//
// wrapGeminiToolResponse 保证 response 为 JSON object：JSON object 透传，否则把纯字符串
// 包装为 {"result": <raw>}。
func wrapGeminiToolResponse(content string) json.RawMessage {
	trimmed := bytes.TrimSpace([]byte(content))
	if len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed) {
		return trimmed
	}
	wrapped, _ := json.Marshal(map[string]string{"result": content})
	return wrapped
}

func toGeminiFunctionDeclarations(defs []ToolDef) []geminiFunctionDeclaration {
	out := make([]geminiFunctionDeclaration, len(defs))
	for i, d := range defs {
		out[i] = geminiFunctionDeclaration{Name: d.Name, Description: d.Description, Parameters: d.Parameters}
	}
	return out
}

// encodeGeminiThinking reads native thinking knobs from Options: thinkingLevel (Gemini-3 enum:
// minimal/low/medium/high) OR thinkingBudget (Gemini-2.5 int: -1 dynamic / 0 off / model range).
// They are mutually exclusive on the wire (sending both 400s); a model's Knobs only ever offers
// one form, so reading whichever is present is safe. Nothing set → omit (provider default).
//
// encodeGeminiThinking 从 Options 读原生 thinking 旋钮：thinkingLevel（Gemini-3 枚举）或
// thinkingBudget（Gemini-2.5 整数：-1 动态 / 0 关 / 模型范围）。二者 wire 上互斥（同发 400）；
// 模型 Knobs 只给其一，故读到哪个用哪个安全。都没设 → 省略（取 provider 默认）。
func encodeGeminiThinking(options map[string]string) *geminiThinkingConfig {
	if v := options["thinkingLevel"]; v != "" {
		return &geminiThinkingConfig{ThinkingLevel: v, IncludeThoughts: true}
	}
	if v := options["thinkingBudget"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil
		}
		tc := &geminiThinkingConfig{ThinkingBudget: &n}
		if n != 0 {
			tc.IncludeThoughts = true // surface thought summaries unless thinking is off
		}
		return tc
	}
	return nil
}

// ── native wire types ──────────────────────────────────────────────────────────

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

// geminiPart carries exactly one of: text, inlineData, fileData, functionCall,
// functionResponse. Thought + ThoughtSignature accompany a reasoning text part.
//
// geminiPart 互斥地携带 text / inlineData / fileData / functionCall / functionResponse 之一。
// Thought + ThoughtSignature 伴随推理文本 part。
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

// geminiFunctionCall is the model's tool invocation. Args is the parsed JSON object
// (not a string, unlike OpenAI). ID is unique per call on Gemini-3.
//
// geminiFunctionCall 是模型的工具调用。Args 是解析后的 JSON object（非字符串）。
// ID 在 Gemini-3 每调用唯一。
type geminiFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

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
	// MaxOutputTokens is a pointer so 0 elides; Gemini's default truncates long output.
	// MaxOutputTokens 用指针使 0 被省略；Gemini 默认会截断长输出。
	MaxOutputTokens *int                  `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

// geminiThinkingConfig is the native thinking knob. ThinkingBudget is a pointer so an
// explicit 0 (off) serializes instead of being elided.
//
// geminiThinkingConfig 是原生 thinking 旋钮。ThinkingBudget 用指针，使显式 0（关闭）能序列化。
type geminiThinkingConfig struct {
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
	IncludeThoughts bool   `json:"includeThoughts,omitempty"`
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

// ── model catalog (Gemini ListModels is rich — it carries inputTokenLimit/outputTokenLimit per
// model — but the thinking knob shape (2.5 budget int vs 3.x level enum) is NOT in the payload, so
// windows come from /models and knobs are filled statically by generation) ─────────────────────

// geminiKnobsFor returns the native thinking knob by model generation: Gemini-3 → thinkingLevel
// enum (cannot be disabled); Gemini-2.5 → thinkingBudget int (-1 dynamic / 0 off / model range).
//
// geminiKnobsFor 按模型代际返回原生 thinking 旋钮：Gemini-3 → thinkingLevel 枚举（不可关）；
// Gemini-2.5 → thinkingBudget 整数（-1 动态 / 0 关 / 模型范围）。
func geminiKnobsFor(modelID string) []Knob {
	id := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(id, "gemini-3"):
		return []Knob{enumKnob("thinkingLevel", "Thinking level", []string{"minimal", "low", "medium", "high"}, "high")}
	case strings.HasPrefix(id, "gemini-2.5"):
		return []Knob{intKnob("thinkingBudget", "Thinking budget", "-1")}
	default:
		return nil
	}
}

// DescribeModels parses Gemini's ListModels body ({"models":[{"name","baseModelId",
// "inputTokenLimit","outputTokenLimit"}]}). Windows come straight from the payload (rich); the
// thinking knob is filled per generation since the payload omits the level enum / budget range.
//
// DescribeModels 解析 Gemini ListModels 返回。窗口直接取自载荷（富）；thinking 旋钮按代际补
// （载荷不含 level 枚举 / budget 范围）。
func (p *geminiProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	var resp struct {
		Models []struct {
			Name             string `json:"name"`
			BaseModelID      string `json:"baseModelId"`
			InputTokenLimit  int    `json:"inputTokenLimit"`
			OutputTokenLimit int    `json:"outputTokenLimit"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, nil
	}
	out := make([]ModelInfo, 0, len(resp.Models))
	for _, m := range resp.Models {
		id := m.BaseModelID
		if id == "" {
			id = strings.TrimPrefix(m.Name, "models/")
		}
		if id == "" {
			continue
		}
		// Every gemini-* generative model is natively multimodal (image + inline PDF); embedding /
		// aqa models are not (and are never picked for chat anyway).
		//
		// 每个 gemini-* 生成模型都原生多模态（图 + 内联 PDF）；embedding / aqa 模型不是（也从不被选作对话）。
		multimodal := strings.HasPrefix(id, "gemini")
		out = append(out, ModelInfo{
			ID:            id,
			DisplayName:   id,
			ContextWindow: m.InputTokenLimit,
			MaxOutput:     m.OutputTokenLimit,
			Vision:        multimodal,
			NativeDocs:    multimodal,
			Knobs:         geminiKnobsFor(id),
		})
	}
	return out, nil
}
