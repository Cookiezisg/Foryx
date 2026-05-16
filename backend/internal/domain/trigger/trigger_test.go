package trigger

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestKind_FourValues(t *testing.T) {
	got := []string{KindCron, KindFsnotify, KindWebhook, KindManual}
	want := []string{"cron", "fsnotify", "webhook", "manual"}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("Kind[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestState_ThreeValues(t *testing.T) {
	got := []string{StateActive, StateIdle, StateError}
	want := []string{"active", "idle", "error"}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("State[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestSentinels_DistinctMessages(t *testing.T) {
	sentinels := []error{
		ErrPathNotExist, ErrPathConflict,
		ErrWebhookSecretMismatch, ErrInvalidCronExpression,
	}
	seen := make(map[string]bool, len(sentinels))
	for _, s := range sentinels {
		if seen[s.Error()] {
			t.Errorf("duplicate sentinel message %q", s.Error())
		}
		seen[s.Error()] = true
	}
}

func TestSentinels_ErrorsIs(t *testing.T) {
	wrapped := errors.Join(ErrInvalidCronExpression, errors.New("expr: bogus"))
	if !errors.Is(wrapped, ErrInvalidCronExpression) {
		t.Errorf("errors.Is fails through errors.Join")
	}
}

func TestSpec_JSONRoundTrip(t *testing.T) {
	s := Spec{
		WorkflowID: "wf_abc",
		NodeID:     "trig_cron",
		Kind:       KindCron,
		Config:     map[string]any{"expression": "0 */1 * * *"},
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Spec
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.WorkflowID != s.WorkflowID || back.Kind != s.Kind {
		t.Errorf("round trip lost data: %+v", back)
	}
	if back.Config["expression"] != "0 */1 * * *" {
		t.Errorf("config lost: %v", back.Config)
	}
}

func TestState_OmitsEmptyOptionals(t *testing.T) {
	// LastFiredAt / NextFireAt / LastError empty → should not appear in JSON.
	s := State{
		WorkflowID: "wf_abc",
		NodeID:     "trig_manual",
		Kind:       KindManual,
		Status:     StateIdle,
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(raw)
	for _, k := range []string{"lastFiredAt", "nextFireAt", "lastError"} {
		if contains(str, k) {
			t.Errorf("expected %q to be omitted, got %s", k, str)
		}
	}
}

func TestState_IncludesLastFiredWhenSet(t *testing.T) {
	when := time.Date(2026, 5, 12, 18, 30, 0, 0, time.UTC)
	s := State{
		WorkflowID:  "wf_abc",
		NodeID:      "trig_cron",
		Kind:        KindCron,
		Status:      StateActive,
		LastFiredAt: &when,
	}
	raw, _ := json.Marshal(s)
	if !contains(string(raw), "lastFiredAt") {
		t.Errorf("expected lastFiredAt in JSON: %s", raw)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
