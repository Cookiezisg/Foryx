package function

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestSentinels_Unique ensures all exported sentinels have distinct
// human-readable messages and start with the "function:" prefix (§S16).
//
// TestSentinels_Unique 检查所有导出 sentinel 消息唯一 + "function:" 前缀。
func TestSentinels_Unique(t *testing.T) {
	all := []error{
		ErrNotFound, ErrDuplicateName, ErrVersionNotFound, ErrPendingNotFound,
		ErrRunFailed, ErrASTParseError, ErrNoActiveVersion,
		ErrEnvNotReady, ErrEnvFailed, ErrDependencyResolution, ErrSandboxUnavailable,
		ErrOpInvalid,
	}
	if len(all) != 12 {
		t.Errorf("expected 12 sentinels per spec, got %d", len(all))
	}
	seen := map[string]bool{}
	for _, e := range all {
		msg := e.Error()
		if !strings.HasPrefix(msg, "function: ") {
			t.Errorf("sentinel %q must start with 'function: '", msg)
		}
		if seen[msg] {
			t.Errorf("duplicate sentinel message: %q", msg)
		}
		seen[msg] = true
	}
}

// TestSentinels_ErrorsIsCompatible verifies sentinels work with errors.Is
// through fmt.Errorf("%w") wrap chains (§S16 wrap discipline).
//
// TestSentinels_ErrorsIsCompatible 验证 sentinel 经过 fmt.Errorf "%w"
// 包装链后 errors.Is 仍能 unwrap 到。
func TestSentinels_ErrorsIsCompatible(t *testing.T) {
	wrapped := fmt.Errorf("functionstore.Get: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("errors.Is should unwrap fmt.Errorf %%w: got %v", wrapped)
	}

	double := fmt.Errorf("outer: %w", wrapped)
	if !errors.Is(double, ErrNotFound) {
		t.Errorf("errors.Is should unwrap two-level wrap: got %v", double)
	}
}

// TestStatusConstants_Stable pins the status enum string values — DB CHECK
// constraint uses these literals;changing them breaks migrations.
//
// TestStatusConstants_Stable 钉死 status 枚举字符串(DB CHECK 用,改了破坏迁移)。
func TestStatusConstants_Stable(t *testing.T) {
	cases := map[string]string{
		"StatusPending":  StatusPending,
		"StatusAccepted": StatusAccepted,
		"StatusRejected": StatusRejected,
	}
	expect := map[string]string{
		"StatusPending":  "pending",
		"StatusAccepted": "accepted",
		"StatusRejected": "rejected",
	}
	for k, v := range cases {
		if v != expect[k] {
			t.Errorf("%s = %q, want %q", k, v, expect[k])
		}
	}
}

// TestEnvStatusConstants_Stable pins env-status enum.
//
// TestEnvStatusConstants_Stable 钉死 env-status 枚举。
func TestEnvStatusConstants_Stable(t *testing.T) {
	cases := map[string]string{
		"EnvStatusPending": EnvStatusPending,
		"EnvStatusSyncing": EnvStatusSyncing,
		"EnvStatusReady":   EnvStatusReady,
		"EnvStatusFailed":  EnvStatusFailed,
		"EnvStatusEvicted": EnvStatusEvicted,
	}
	expect := map[string]string{
		"EnvStatusPending": "pending",
		"EnvStatusSyncing": "syncing",
		"EnvStatusReady":   "ready",
		"EnvStatusFailed":  "failed",
		"EnvStatusEvicted": "evicted",
	}
	for k, v := range cases {
		if v != expect[k] {
			t.Errorf("%s = %q, want %q", k, v, expect[k])
		}
	}
}

// TestDefaultPythonVersion checks the default PEP 440 spec.
func TestDefaultPythonVersion(t *testing.T) {
	if DefaultPythonVersion != ">=3.12" {
		t.Errorf("DefaultPythonVersion = %q, want '>=3.12'", DefaultPythonVersion)
	}
}

// TestAcceptedVersionCap pins the per-function accepted-version cap.
func TestAcceptedVersionCap(t *testing.T) {
	if AcceptedVersionCap != 50 {
		t.Errorf("AcceptedVersionCap = %d, want 50", AcceptedVersionCap)
	}
}

// TestFunctionTableName ensures GORM uses 'functions' (not 'function').
func TestFunctionTableName(t *testing.T) {
	if (Function{}).TableName() != "functions" {
		t.Errorf("Function.TableName() = %q, want 'functions'", (Function{}).TableName())
	}
	if (Version{}).TableName() != "function_versions" {
		t.Errorf("Version.TableName() = %q, want 'function_versions'", (Version{}).TableName())
	}
}
