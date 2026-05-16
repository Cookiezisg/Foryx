package installprogress

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type fakeEmitter struct {
	mu     sync.Mutex
	starts []startCall
	deltas []deltaCall
	stops  []stopCall
}
type startCall struct{ blockType string; attrs map[string]any }
type deltaCall struct{ blockID, delta string }
type stopCall struct{ blockID, status string; err error }

func (f *fakeEmitter) StartBlock(_ context.Context, blockType string, attrs map[string]any) string {
	f.mu.Lock(); defer f.mu.Unlock()
	id := "blk_fake_" + blockType
	f.starts = append(f.starts, startCall{blockType, attrs})
	return id
}
func (f *fakeEmitter) StopMessage(_ context.Context, _, _, _, _, _ string, _, _ int)         {}
func (f *fakeEmitter) EmitMessageStart(_ context.Context, _, _, _ string, _ map[string]any)  {}
func (f *fakeEmitter) EmitBlockStart(_ context.Context, _, _, _, _ string, _ map[string]any) {}
func (f *fakeEmitter) DeltaBlock(_ context.Context, blockID, delta string) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.deltas = append(f.deltas, deltaCall{blockID, delta})
}
func (f *fakeEmitter) StopBlock(_ context.Context, blockID, status string, err error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.stops = append(f.stops, stopCall{blockID, status, err})
}
var _ eventlogpkg.Emitter = (*fakeEmitter)(nil)

func TestRun_NoChatFlow_FnRunsButNoEmission(t *testing.T) {
	em := &fakeEmitter{}
	ctx := eventlogpkg.With(context.Background(), em)

	called := false
	out, err := Run(ctx, map[string]any{"runtime": "python"},
		func(progress sandboxdomain.ProgressFunc) (string, error) {
			called = true
			progress("download", "fetching", 50)
			return "ok", nil
		})

	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out != "ok" {
		t.Errorf("out = %q, want ok", out)
	}
	if !called {
		t.Error("fn not called")
	}
	if len(em.starts) > 0 || len(em.deltas) > 0 || len(em.stops) > 0 {
		t.Errorf("expected no emission outside chat flow, got starts=%d deltas=%d stops=%d",
			len(em.starts), len(em.deltas), len(em.stops))
	}
}

func TestRun_ChatFlow_EmitsProgressBlock(t *testing.T) {
	em := &fakeEmitter{}
	ctx := eventlogpkg.With(context.Background(), em)
	ctx = reqctxpkg.WithConversationID(ctx, "cv_test")
	ctx = reqctxpkg.WithParentBlockID(ctx, "tc_parent")

	_, err := Run(ctx, map[string]any{"runtime": "python", "stage": "preparing"},
		func(progress sandboxdomain.ProgressFunc) (string, error) {
			progress("download", "Fetching python@3.12", 25)
			progress("install", "Creating venv", 90)
			return "done", nil
		})

	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(em.starts) != 1 {
		t.Fatalf("expected 1 StartBlock, got %d", len(em.starts))
	}
	if em.starts[0].blockType != eventlogdomain.BlockTypeProgress {
		t.Errorf("blockType = %q, want progress", em.starts[0].blockType)
	}
	if len(em.deltas) != 4 {
		t.Errorf("expected 4 DeltaBlock calls (start + 2 fn + done), got %d", len(em.deltas))
	}
	if !strings.Contains(em.deltas[0].delta, "[starting]") || !strings.Contains(em.deltas[0].delta, "python") {
		t.Errorf("delta[0] expected synthetic [starting] python line, got %q", em.deltas[0].delta)
	}
	if !strings.Contains(em.deltas[1].delta, "[download]") || !strings.Contains(em.deltas[1].delta, "(25%)") {
		t.Errorf("delta[1] missing fn-supplied download line: %q", em.deltas[1].delta)
	}
	if !strings.Contains(em.deltas[len(em.deltas)-1].delta, "[done]") {
		t.Errorf("last delta expected synthetic [done] line, got %q", em.deltas[len(em.deltas)-1].delta)
	}
	if len(em.stops) != 1 || em.stops[0].status != eventlogdomain.StatusCompleted {
		t.Errorf("expected 1 StopBlock with status=completed, got %+v", em.stops)
	}
}

func TestRun_ChatFlow_PropagatesErrorAndStopsBlockAsError(t *testing.T) {
	em := &fakeEmitter{}
	ctx := eventlogpkg.With(context.Background(), em)
	ctx = reqctxpkg.WithConversationID(ctx, "cv_test")
	ctx = reqctxpkg.WithParentBlockID(ctx, "tc_parent")

	wantErr := errors.New("install borked")
	_, err := Run(ctx, nil,
		func(progress sandboxdomain.ProgressFunc) (string, error) {
			return "", wantErr
		})

	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr to propagate", err)
	}
	if len(em.stops) != 1 || em.stops[0].status != eventlogdomain.StatusError {
		t.Errorf("expected StopBlock with status=error, got %+v", em.stops)
	}
	if em.stops[0].err == nil {
		t.Error("expected StopBlock to receive non-nil err")
	}
}

func TestFormatProgressLine(t *testing.T) {
	cases := []struct {
		stage, msg string
		percent    int
		want       string
	}{
		{"download", "fetching", 50, "[download] fetching (50%)\n"},
		{"", "Generic message", -1, "Generic message\n"},
		{"build", "compiling", -1, "[build] compiling\n"},
		{"", "no stage", 0, "no stage (0%)\n"},
	}
	for _, c := range cases {
		got := formatProgressLine(c.stage, c.msg, c.percent)
		if got != c.want {
			t.Errorf("formatProgressLine(%q, %q, %d) = %q, want %q",
				c.stage, c.msg, c.percent, got, c.want)
		}
	}
}
