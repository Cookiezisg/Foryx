package scheduler

import (
	"errors"
	"fmt"
	"testing"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
)

// TestNodeErrText — F104 (round-5 wfnodeval lane): a workflow node's durable Error column (read by
// get_flowrun / :triage) must carry the sentinel's CLEAN Message, not the wrapped Go call-path chain — a
// dangling fn ref leaked the literal "functionapp.RunFunction: function not found" (the flowrun-record
// sibling of F89's llmErrText). A raw error (a function's Python traceback) passes through unchanged.
func TestNodeErrText(t *testing.T) {
	sentinel := errorspkg.New(errorspkg.KindNotFound, "FUNCTION_NOT_FOUND", "function not found")
	if got := nodeErrText(fmt.Errorf("functionapp.RunFunction: %w", sentinel)); got != "function not found" {
		t.Errorf("a wrapped sentinel must surface its clean Message (no Go call-path), got: %q", got)
	}
	// Details (e.g. a control/approval CEL reason, F69) are appended so :triage still gets the actionable part.
	withDetails := errorspkg.New(errorspkg.KindInvalid, "CONTROL_INVALID_CEL", "control CEL is invalid").
		WithDetails(map[string]any{"reason": "undeclared reference to 'payload'"})
	if got := nodeErrText(withDetails); got != "control CEL is invalid (reason=undeclared reference to 'payload')" {
		t.Errorf("details should append to the message, got: %q", got)
	}
	// A raw non-sentinel error (a crashed function's Python traceback) passes through unchanged.
	if got := nodeErrText(errors.New("Traceback (most recent call last): ValueError: x")); got != "Traceback (most recent call last): ValueError: x" {
		t.Errorf("a raw error must pass through unchanged, got: %q", got)
	}
}
