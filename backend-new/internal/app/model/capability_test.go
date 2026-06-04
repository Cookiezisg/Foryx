package model

import (
	"context"
	"testing"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)

// fakeProbeReader feeds canned probe archives without an apikey store.
//
// fakeProbeReader 在无 apikey store 下喂预设探测档案。
type fakeProbeReader struct {
	keys []apikeydomain.ProbedKey
	err  error
}

func (f fakeProbeReader) ListProbed(_ context.Context) ([]apikeydomain.ProbedKey, error) {
	return f.keys, f.err
}

func TestCapabilityListAggregates(t *testing.T) {
	probed := []apikeydomain.ProbedKey{
		// live OpenAI key — both ids are in the static catalog → 2 views attributed to it.
		{ID: "aki_oa", DisplayName: "My OpenAI", Provider: "openai", TestStatus: apikeydomain.TestStatusOK,
			TestResponse: `{"object":"list","data":[{"id":"gpt-5.5"},{"id":"gpt-4o"}]}`},
		// non-OK key contributes nothing.
		{ID: "aki_dead", DisplayName: "Dead", Provider: "openai", TestStatus: apikeydomain.TestStatusError,
			TestResponse: `{"data":[{"id":"gpt-5.5"}]}`},
		// unparseable body contributes nothing but must not blank the whole catalog.
		{ID: "aki_bad", DisplayName: "Bad", Provider: "deepseek", TestStatus: apikeydomain.TestStatusOK,
			TestResponse: `not json`},
	}
	svc := NewCapabilityService(fakeProbeReader{keys: probed}, zap.NewNop())
	views, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("got %d views, want 2 (only the live OpenAI key's catalog models)", len(views))
	}
	byModel := map[string]CapabilityView{}
	for _, v := range views {
		if v.APIKeyID != "aki_oa" || v.KeyName != "My OpenAI" || v.Provider != "openai" {
			t.Errorf("view not attributed to the live key: %+v", v)
		}
		if v.ContextWindow == 0 {
			t.Errorf("model %q missing context window", v.ModelID)
		}
		byModel[v.ModelID] = v
	}
	// gpt-5.5 is a reasoning model → carries native knobs; gpt-4o is not → no knobs.
	// gpt-5.5 是推理模型→带原生旋钮；gpt-4o 非推理→无旋钮。
	if len(byModel["gpt-5.5"].Knobs) == 0 {
		t.Error("gpt-5.5 should expose native reasoning knobs")
	}
	if len(byModel["gpt-4o"].Knobs) != 0 {
		t.Error("gpt-4o (non-reasoning) should expose no knobs")
	}
}

func TestCapabilityListEmptyOnNoKeys(t *testing.T) {
	svc := NewCapabilityService(fakeProbeReader{}, zap.NewNop())
	views, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(views) != 0 {
		t.Errorf("got %d views, want 0", len(views))
	}
}
