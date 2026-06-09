package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// reqBuilder is the BuildRequest half of every provider, enough to inspect the wire body.
//
// reqBuilder 是每家 provider 的 BuildRequest 半边，足以检视 wire body。
type reqBuilder interface {
	BuildRequest(context.Context, Request) (*http.Request, error)
}

// multimodalBody builds one user turn carrying text + an image (data-URL) + a PDF file part, runs
// it through a provider's BuildRequest, and returns the raw request body as a string. The same
// neutral ContentPart input lets every provider's renderer be asserted against its official wire.
//
// multimodalBody 构造一个带 text + 图（data-URL）+ PDF file part 的 user 回合，过 provider 的
// BuildRequest，返回原始请求体字符串。同一份中立 ContentPart 输入，让每家渲染器对照官方 wire 断言。
func multimodalBody(t *testing.T, p reqBuilder) string {
	t.Helper()
	req := Request{
		ModelID: "m",
		Key:     "k",
		BaseURL: "https://example.test",
		Messages: []LLMMessage{{
			Role: RoleUser,
			Parts: []ContentPart{
				{Type: PartText, Text: "describe these"},
				{Type: PartImageURL, ImageURL: "data:image/png;base64,IMG"},
				{Type: PartFile, MediaType: "application/pdf", Data: "PDF", Filename: "doc.pdf"},
			},
		}},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	raw, _ := io.ReadAll(httpReq.Body)
	return string(raw)
}

// TestMultimodalRendering verifies each provider renders the neutral multi-modal parts into its
// own official wire shape: vision-capable providers carry the image natively, the three with
// native document input (anthropic/openai/gemini) carry the PDF inline, and the rest degrade the
// PDF (skip the file part — sandbox text-extraction is R0053) while still carrying the image.
//
// TestMultimodalRendering 验证每家把中立多模态块渲成自家官方 wire：视觉模型原生承载图；三家原生
// 文档输入（anthropic/openai/gemini）内联 PDF；其余降级 PDF（跳过 file part——沙箱抽取是 R0053）
// 但仍承载图。
func TestMultimodalRendering(t *testing.T) {
	cases := []struct {
		name     string
		builder  reqBuilder
		contains []string
		absent   []string
	}{
		// Anthropic: image as a base64 source block, PDF as a document block. No "image_url".
		{"anthropic", newAnthropicProvider(),
			[]string{`"type":"document"`, `"type":"image"`, "application/pdf", "image/png", `"PDF"`}, []string{"image_url"}},
		// OpenAI: native file part + image_url part (it accepts inline PDF as a file).
		{"openai", newOpenAIProvider(),
			[]string{`"type":"file"`, `"type":"image_url"`, "doc.pdf"}, nil},
		// Gemini: inlineData for both image and PDF (mimeType carries the type). No "image_url".
		{"gemini", newGeminiProvider(),
			[]string{"application/pdf", "image/png"}, []string{"image_url"}},
		// OpenAI-compatible vision (image_url data-URL); PDF "file" part degraded (skipped).
		{"deepseek", newDeepSeekProvider(),
			[]string{`"image_url"`, "data:image/png;base64,IMG"}, []string{"doc.pdf"}},
		{"qwen", newQwenProvider(),
			[]string{`"image_url"`, "data:image/png"}, []string{"doc.pdf"}},
		{"zhipu", newZhipuProvider(),
			[]string{`"image_url"`, "data:image/png"}, []string{"doc.pdf"}},
		{"doubao", newDoubaoProvider(),
			[]string{`"image_url"`, "data:image/png"}, []string{"doc.pdf"}},
		{"moonshot", newMoonshotProvider(),
			[]string{`"image_url"`, "data:image/png"}, []string{"doc.pdf"}},
		{"openrouter", newOpenRouterProvider(),
			[]string{`"image_url"`, "data:image/png"}, []string{"doc.pdf"}},
		{"custom", newCustomProvider(),
			[]string{`"image_url"`, "data:image/png"}, []string{"doc.pdf"}},
		// Ollama: native `images` array of raw base64 (data-URL prefix stripped); no inline PDF.
		{"ollama", newOllamaProvider(),
			[]string{`"images"`, `"IMG"`}, []string{"doc.pdf", "data:image"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := multimodalBody(t, tc.builder)
			for _, want := range tc.contains {
				if !strings.Contains(body, want) {
					t.Errorf("%s wire missing %q\nbody: %s", tc.name, want, body)
				}
			}
			for _, no := range tc.absent {
				if strings.Contains(body, no) {
					t.Errorf("%s wire should not contain %q\nbody: %s", tc.name, no, body)
				}
			}
		})
	}
}

// TestMultimodalTextOnlyUnchanged guards the no-Parts path: a plain Content message must still
// render as before (the multimodal branch only triggers when Parts is non-empty).
//
// TestMultimodalTextOnlyUnchanged 守 no-Parts 路径：纯 Content 消息仍按原样渲染（多模态分支仅在
// Parts 非空时触发）。
func TestMultimodalTextOnlyUnchanged(t *testing.T) {
	builders := map[string]reqBuilder{
		"deepseek": newDeepSeekProvider(), "qwen": newQwenProvider(), "zhipu": newZhipuProvider(),
		"doubao": newDoubaoProvider(), "moonshot": newMoonshotProvider(), "openrouter": newOpenRouterProvider(),
		"custom": newCustomProvider(), "ollama": newOllamaProvider(), "anthropic": newAnthropicProvider(),
	}
	for name, b := range builders {
		t.Run(name, func(t *testing.T) {
			req := Request{ModelID: "m", Key: "k", BaseURL: "https://example.test",
				Messages: []LLMMessage{{Role: RoleUser, Content: "just text"}}}
			httpReq, err := b.BuildRequest(context.Background(), req)
			if err != nil {
				t.Fatalf("BuildRequest: %v", err)
			}
			raw, _ := io.ReadAll(httpReq.Body)
			if !strings.Contains(string(raw), "just text") {
				t.Errorf("%s text-only wire missing content\nbody: %s", name, raw)
			}
		})
	}
}
