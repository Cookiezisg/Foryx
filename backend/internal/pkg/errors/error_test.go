package errors

import (
	stderrors "errors"
	"fmt"
	"testing"
)

func TestError_FieldsAndMessage(t *testing.T) {
	e := New(KindNotFound, "X_NOT_FOUND", "x not found")
	if e.Kind != KindNotFound || e.Code != "X_NOT_FOUND" || e.Message != "x not found" {
		t.Fatalf("unexpected fields: %+v", e)
	}
	if e.Error() != "x not found" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestError_WithCause_UnwrapAndMessage(t *testing.T) {
	base := New(KindUnprocessable, "RUN_FAILED", "run failed")
	cause := stderrors.New("boom")
	wrapped := base.WithCause(cause)

	if wrapped.Error() != "run failed: boom" {
		t.Errorf("Error() = %q, want \"run failed: boom\"", wrapped.Error())
	}
	if !stderrors.Is(wrapped, cause) {
		t.Error("Unwrap should expose the cause")
	}
	if base.cause != nil {
		t.Error("WithCause mutated the original sentinel")
	}
}

func TestError_Is_ByCode(t *testing.T) {
	sentinel := New(KindNotFound, "API_KEY_NOT_FOUND", "api key not found")

	if !stderrors.Is(sentinel, sentinel) {
		t.Error("sentinel should match itself")
	}
	if !stderrors.Is(sentinel.WithCause(stderrors.New("x")), sentinel) {
		t.Error("WithCause clone should match the sentinel by code")
	}
	if !stderrors.Is(fmt.Errorf("ctx: %w", sentinel), sentinel) {
		t.Error("fmt-wrapped error should match the sentinel")
	}
}

func TestError_Is_DistinctCodesDoNotMatch(t *testing.T) {
	a := New(KindNotFound, "A_NOT_FOUND", "a")
	b := New(KindNotFound, "B_NOT_FOUND", "b")
	if stderrors.Is(a, b) {
		t.Error("different codes must not match even under the same kind")
	}
}

func TestError_WithDetails(t *testing.T) {
	base := New(KindInvalid, "BAD", "bad")
	withD := base.WithDetails(map[string]any{"field": "name"})
	if withD.Details["field"] != "name" {
		t.Errorf("details not carried: %+v", withD.Details)
	}
	if base.Details != nil {
		t.Error("WithDetails mutated the original sentinel")
	}
}

func TestSentinels(t *testing.T) {
	if ErrInvalidRequest.Kind != KindInvalid || ErrInvalidRequest.Code != "INVALID_REQUEST" {
		t.Errorf("ErrInvalidRequest: %+v", ErrInvalidRequest)
	}
	if ErrUnauthorizedNoWorkspace.Kind != KindUnauthorized || ErrUnauthorizedNoWorkspace.Code != "UNAUTH_NO_WORKSPACE" {
		t.Errorf("ErrUnauthorizedNoWorkspace: %+v", ErrUnauthorizedNoWorkspace)
	}
}

// TestSurface — the shared LLM/flowrun/agent error surface (F89/F104 extracted): a wrapped sentinel
// yields its clean Message (+ Details), NOT the fmt.Errorf("pkg.Method: %w") chain that leaks Go
// package paths; a raw error passes through; nil → "".
func TestSurface(t *testing.T) {
	if got := Surface(nil); got != "" {
		t.Errorf("Surface(nil) = %q, want empty", got)
	}

	// A wrapped sentinel: the Go call-path breadcrumb must be stripped, only the clean Message kept.
	sentinel := New(KindNotFound, "FUNCTION_NOT_FOUND", "function not found")
	wrapped := fmt.Errorf("functionapp.Get: %w", sentinel)
	if got := Surface(wrapped); got != "function not found" {
		t.Errorf("Surface(wrapped sentinel) = %q, want clean message without the Go path", got)
	}

	// Details ride along, sorted.
	withDetails := New(KindUnprocessable, "X_BAD", "bad thing").WithDetails(map[string]any{"reason": "too big", "field": "name"})
	got := Surface(fmt.Errorf("pkg.M: %w", withDetails))
	if got != "bad thing (field=name; reason=too big)" {
		t.Errorf("Surface with details = %q", got)
	}

	// A raw (non-structured) error passes through unchanged.
	if got := Surface(stderrors.New("raw python traceback")); got != "raw python traceback" {
		t.Errorf("Surface(raw) = %q, want passthrough", got)
	}
}
