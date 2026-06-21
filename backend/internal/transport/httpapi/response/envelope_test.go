package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSuccessEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	Success(w, 200, map[string]string{"x": "y"})
	var env map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if _, ok := env["data"]; !ok {
		t.Errorf("success must wrap in {data}: %s", w.Body.String())
	}
	if _, ok := env["error"]; ok {
		t.Error("success must not carry error")
	}
}

// TestSuccessEnvelope_NilSliceIsArray — F170: a NON-paged list endpoint (documents/skills/memories) that
// returns a nil slice when empty must still serialize as {"data": []}, never {"data": null} — otherwise
// the same endpoint flips between [] (populated) and null (empty) and breaks a client's `for (x of data)`.
// A single-object body still passes through (not coerced to a slice).
func TestSuccessEnvelope_NilSliceIsArray(t *testing.T) {
	w := httptest.NewRecorder()
	Success(w, 200, []int(nil))
	if got := w.Body.String(); got != "{\"data\":[]}\n" && got != "{\"data\":[]}" {
		t.Fatalf("empty list must be [] not null, got %q", got)
	}
	// A single object must NOT be coerced.
	w2 := httptest.NewRecorder()
	Success(w2, 200, map[string]string{"x": "y"})
	var env struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &env); err != nil || env.Data["x"] != "y" {
		t.Fatalf("single-object body must pass through untouched, got %q err=%v", w2.Body.String(), err)
	}
}

func TestPagedEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	Paged(w, []int{1, 2}, "cur1", true)
	var env struct {
		Data       []int   `json:"data"`
		NextCursor *string `json:"nextCursor"`
		HasMore    *bool   `json:"hasMore"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data) != 2 || env.NextCursor == nil || *env.NextCursor != "cur1" || env.HasMore == nil || !*env.HasMore {
		t.Errorf("paged envelope = %s", w.Body.String())
	}
}

// TestPagedEnvelope_EmptyIsArray — F-empty-list-null (round-9 entitydelete): an empty page must
// serialize as {"data": []}, never null or an absent key, so a client iterating data does not NPE (N4).
// Covers both a nil typed slice (the common store-returns-nil case) and an explicit empty slice.
func TestPagedEnvelope_EmptyIsArray(t *testing.T) {
	for _, items := range []any{[]int(nil), []int{}, []string(nil)} {
		w := httptest.NewRecorder()
		Paged(w, items, "", false)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("unmarshal %s: %v", w.Body.String(), err)
		}
		data, ok := raw["data"]
		if !ok {
			t.Fatalf("empty page must include a data key, got %s", w.Body.String())
		}
		if string(data) != "[]" {
			t.Fatalf("empty page data must be [], got %s (full: %s)", data, w.Body.String())
		}
	}
}

func TestErrorEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, 400, "BAD", "bad thing", map[string]any{"field": "name"})
	if w.Code != 400 {
		t.Errorf("status = %d", w.Code)
	}
	code, msg := decodeErr(t, w.Body.Bytes())
	if code != "BAD" || msg != "bad thing" {
		t.Errorf("error envelope = %q / %q", code, msg)
	}
}
