package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type webBridge struct{ events []streamdomain.Event }

func (b *webBridge) Publish(_ context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	b.events = append(b.events, e)
	return streamdomain.Envelope{}, nil
}
func (b *webBridge) Subscribe(_ context.Context, _ int64) (<-chan streamdomain.Envelope, func(), error) {
	return nil, func() {}, nil
}

// TestWebFetch_StreamsSummaryProgress: the inner summary LLM streams token by token into a
// `progress` block under the WebFetch tool_call, while the final tool_result is the full summary.
//
// TestWebFetch_StreamsSummaryProgress：内层摘要 LLM 逐 token 流进 WebFetch tool_call 下的 `progress`
// 块，而最终 tool_result 是完整摘要。
func TestWebFetch_StreamsSummaryProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("the page content"))
	}))
	defer srv.Close()
	old := jinaEndpoint
	jinaEndpoint = srv.URL + "/"
	defer func() { jinaEndpoint = old }()

	factory := llminfra.NewFactory()
	factory.Mock().PushScript(llminfra.MockScript{
		Events: []llminfra.StreamEvent{
			{Type: llminfra.EventText, Delta: "SUM"},
			{Type: llminfra.EventText, Delta: "MARY"},
		},
	})
	wf := &WebFetch{
		picker:  &fakePicker{ref: modeldomain.ModelRef{APIKeyID: "ak", ModelID: "m"}},
		keys:    &fakeKeys{creds: apikeydomain.Credentials{Provider: "mock"}},
		factory: factory,
	}

	bridge := &webBridge{}
	ctx := reqctxpkg.SetToolCallID(reqctxpkg.SetConversationID(loopapp.WithBridge(context.Background(), bridge), "c1"), "tc_fetch")
	out, err := wf.Execute(ctx, `{"url":"http://93.184.216.34/","prompt":"what"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "SUMMARY" {
		t.Fatalf("tool_result = %q, want SUMMARY", out)
	}

	open, ok := bridge.events[0].Frame.(streamdomain.Open)
	if !ok || open.ParentID != "tc_fetch" || open.Node.Type != "progress" {
		t.Fatalf("first frame not a progress Open under the tool_call: %+v", bridge.events[0])
	}
	var streamed strings.Builder
	for _, e := range bridge.events {
		if d, ok := e.Frame.(streamdomain.Delta); ok {
			streamed.WriteString(d.Chunk)
		}
	}
	if streamed.String() != "SUMMARY" {
		t.Fatalf("summary not streamed token-by-token: got %q", streamed.String())
	}
}
