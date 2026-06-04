package model

import (
	"context"
	"errors"
	"testing"
)

// stubPicker is a fixed ModelPicker for exercising Resolve without a workspace store.
//
// stubPicker 是固定的 ModelPicker，用于在无 workspace store 下测 Resolve。
type stubPicker struct {
	ref ModelRef
	err error
}

func (s stubPicker) Pick(_ context.Context, _ string) (ModelRef, error) { return s.ref, s.err }

func TestResolveOverrideWins(t *testing.T) {
	override := &ModelRef{APIKeyID: "aki_x", ModelID: "gpt-5.5"}
	picker := stubPicker{ref: ModelRef{APIKeyID: "aki_default", ModelID: "default"}}
	got, err := Resolve(context.Background(), ScenarioDialogue, override, picker)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.APIKeyID != "aki_x" || got.ModelID != "gpt-5.5" {
		t.Errorf("got %+v, want the override", got)
	}
}

func TestResolveFallsBackToPicker(t *testing.T) {
	picker := stubPicker{ref: ModelRef{APIKeyID: "aki_default", ModelID: "default-model"}}
	got, err := Resolve(context.Background(), ScenarioAgent, nil, picker)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ModelID != "default-model" {
		t.Errorf("got %+v, want the picker default", got)
	}
}

func TestResolveZeroOverrideFallsBack(t *testing.T) {
	// An empty (zero) override means "unset" — it must fall through to the picker, not win.
	// 空（zero）override 表示"未设"——须 fall through 到 picker，不能胜出。
	picker := stubPicker{ref: ModelRef{APIKeyID: "aki_d", ModelID: "d"}}
	got, err := Resolve(context.Background(), ScenarioUtility, &ModelRef{}, picker)
	if err != nil || got.ModelID != "d" {
		t.Errorf("zero override should fall back; got %+v err %v", got, err)
	}
}

func TestResolveInvalidScenario(t *testing.T) {
	picker := stubPicker{ref: ModelRef{APIKeyID: "a", ModelID: "m"}}
	_, err := Resolve(context.Background(), "bogus", nil, picker)
	if !errors.Is(err, ErrScenarioInvalid) {
		t.Errorf("err = %v, want ErrScenarioInvalid", err)
	}
}

func TestResolvePropagatesNotConfigured(t *testing.T) {
	picker := stubPicker{err: ErrNotConfigured}
	_, err := Resolve(context.Background(), ScenarioDialogue, nil, picker)
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestModelRefValidate(t *testing.T) {
	if err := (ModelRef{APIKeyID: "a", ModelID: "m"}).Validate(); err != nil {
		t.Errorf("valid ref errored: %v", err)
	}
	if err := (ModelRef{APIKeyID: "a"}).Validate(); !errors.Is(err, ErrRefInvalid) {
		t.Errorf("missing modelId: err = %v, want ErrRefInvalid", err)
	}
	if err := (ModelRef{ModelID: "m"}).Validate(); !errors.Is(err, ErrRefInvalid) {
		t.Errorf("missing apiKeyId: err = %v, want ErrRefInvalid", err)
	}
}

func TestIsValidScenario(t *testing.T) {
	for _, s := range ListScenarios() {
		if !IsValidScenario(s) {
			t.Errorf("ListScenarios returned an invalid scenario %q", s)
		}
	}
	if IsValidScenario("bogus") {
		t.Error(`"bogus" should not be a valid scenario`)
	}
}
