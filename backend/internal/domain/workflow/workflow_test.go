package workflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestSentinels_Unique pins exported sentinels — every message starts with
// "workflow: " and messages are unique. Drift breaks errors.Is matching.
//
// TestSentinels_Unique 钉死导出 sentinel:全 workflow: 前缀 + 消息唯一。
func TestSentinels_Unique(t *testing.T) {
	all := []error{
		ErrNotFound, ErrDuplicateName, ErrVersionNotFound, ErrPendingNotFound,
		ErrNoActiveVersion, ErrDAGCycle, ErrInvalidReference, ErrNoTrigger,
		ErrOpInvalid, ErrCapabilityNotFound, ErrMCPServerNotInstalled,
	}
	if len(all) != 11 {
		t.Errorf("expected 11 sentinels per Plan 04 spec, got %d", len(all))
	}
	seen := map[string]bool{}
	for _, e := range all {
		msg := e.Error()
		if !strings.HasPrefix(msg, "workflow: ") {
			t.Errorf("sentinel %q must start with 'workflow: '", msg)
		}
		if seen[msg] {
			t.Errorf("duplicate sentinel message: %q", msg)
		}
		seen[msg] = true
	}
}

// TestSentinels_ErrorsIsCompatible verifies sentinels unwrap through
// fmt.Errorf("%w") chains (§S16).
func TestSentinels_ErrorsIsCompatible(t *testing.T) {
	wrapped := fmt.Errorf("workflowstore.GetWorkflow: %w",
		fmt.Errorf("workflowapp.Get: %w", ErrNotFound))
	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("errors.Is should unwrap to ErrNotFound through %%w chain")
	}
}

// TestNodeType_Whitelist pins the 13 V1 node types. Drift = protocol break.
func TestNodeType_Whitelist(t *testing.T) {
	valid := []string{
		NodeTypeTrigger, NodeTypeFunction, NodeTypeHandler, NodeTypeMCP,
		NodeTypeSkill, NodeTypeLLM, NodeTypeHTTP, NodeTypeCondition,
		NodeTypeLoop, NodeTypeParallel, NodeTypeApproval, NodeTypeWait,
		NodeTypeVariable,
	}
	if len(valid) != 13 {
		t.Errorf("expected 13 node types per Plan 04 spec, got %d", len(valid))
	}
	for _, nt := range valid {
		if !IsValidNodeType(nt) {
			t.Errorf("IsValidNodeType(%q) = false", nt)
		}
	}
	if IsValidNodeType("frobnicate") {
		t.Errorf("unknown type should be invalid")
	}
}

// TestCapabilityNode_Subset — exactly 6 capability nodes
// (function/handler/mcp/skill/llm/http) accept retry / onError / timeout.
func TestCapabilityNode_Subset(t *testing.T) {
	caps := []string{NodeTypeFunction, NodeTypeHandler, NodeTypeMCP, NodeTypeSkill, NodeTypeLLM, NodeTypeHTTP}
	for _, nt := range caps {
		if !IsCapabilityNode(nt) {
			t.Errorf("IsCapabilityNode(%q) = false, want true", nt)
		}
	}
	nonCaps := []string{NodeTypeTrigger, NodeTypeCondition, NodeTypeLoop, NodeTypeParallel, NodeTypeApproval, NodeTypeWait, NodeTypeVariable}
	for _, nt := range nonCaps {
		if IsCapabilityNode(nt) {
			t.Errorf("IsCapabilityNode(%q) = true, want false (non-capability)", nt)
		}
	}
}

// TestOnError_Whitelist — 3 values stop/continue/branch.
func TestOnError_Whitelist(t *testing.T) {
	for _, s := range []string{OnErrorStop, OnErrorContinue, OnErrorBranch} {
		if !IsValidOnError(s) {
			t.Errorf("IsValidOnError(%q) = false", s)
		}
	}
	if IsValidOnError("explode") {
		t.Errorf("unknown OnError should be invalid")
	}
}

// TestVariableType_Whitelist — 6 values matching function.ParameterSpec.
func TestVariableType_Whitelist(t *testing.T) {
	for _, v := range []string{VarTypeString, VarTypeNumber, VarTypeInteger, VarTypeBoolean, VarTypeObject, VarTypeArray} {
		if !IsValidVariableType(v) {
			t.Errorf("IsValidVariableType(%q) = false", v)
		}
	}
	if IsValidVariableType("date") {
		t.Errorf("unknown variable type should be invalid")
	}
}
