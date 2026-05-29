package model

import (
	"encoding/json"
	"testing"
)

func TestModelRef_ThinkingJSON_Nil(t *testing.T) {
	ref := ModelRef{APIKeyID: "aki_x", ModelID: "gpt-4o"}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ModelRef
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Thinking != nil {
		t.Errorf("Thinking = %+v, want nil", got.Thinking)
	}
	// nil Thinking must produce absent field, not explicit null.
	if string(b) != `{"apiKeyId":"aki_x","modelId":"gpt-4o"}` {
		t.Errorf("unexpected JSON: %s", b)
	}
}

func TestModelRef_ThinkingJSON_OnWithEffort(t *testing.T) {
	ref := ModelRef{
		APIKeyID: "aki_x",
		ModelID:  "gpt-4o",
		Thinking: &ThinkingSpec{Mode: "on", Effort: "high"},
	}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ModelRef
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Thinking == nil {
		t.Fatal("Thinking is nil after round-trip")
	}
	if got.Thinking.Mode != "on" || got.Thinking.Effort != "high" || got.Thinking.Budget != 0 {
		t.Errorf("Thinking = %+v, want {on high 0}", got.Thinking)
	}
}

func TestModelRef_ThinkingJSON_OnWithBudget(t *testing.T) {
	ref := ModelRef{
		APIKeyID: "aki_y",
		ModelID:  "claude-3-7-sonnet-latest",
		Thinking: &ThinkingSpec{Mode: "on", Budget: 5000},
	}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ModelRef
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Thinking == nil {
		t.Fatal("Thinking is nil after round-trip")
	}
	if got.Thinking.Mode != "on" || got.Thinking.Budget != 5000 || got.Thinking.Effort != "" {
		t.Errorf("Thinking = %+v, want {on  5000}", got.Thinking)
	}
}

func TestModelRef_ThinkingJSON_Off(t *testing.T) {
	ref := ModelRef{
		APIKeyID: "aki_z",
		ModelID:  "deepseek-v4",
		Thinking: &ThinkingSpec{Mode: "off"},
	}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ModelRef
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Thinking == nil {
		t.Fatal("Thinking is nil after round-trip")
	}
	if got.Thinking.Mode != "off" {
		t.Errorf("Mode = %q, want \"off\"", got.Thinking.Mode)
	}
}
