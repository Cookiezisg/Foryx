package handler

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestSentinels_Unique pins exported sentinels: every message starts with
// "handler:" and messages are unique. Drift breaks errors.Is matching after
// future renames.
//
// TestSentinels_Unique 钉死导出 sentinel:全 handler: 前缀 + 消息唯一。
// 改名漂移会破 errors.Is 匹配。
func TestSentinels_Unique(t *testing.T) {
	all := []error{
		ErrNotFound, ErrDuplicateName, ErrMethodNotFound, ErrVersionNotFound,
		ErrPendingNotFound, ErrInstanceSpawnFailed,
		ErrInstanceCrashed, ErrInstanceRPCTimeout, ErrInstanceNotFound,
		ErrNoActiveVersion, ErrEnvNotReady, ErrEnvFailed, ErrSandboxUnavailable,
		ErrOpInvalid, ErrASTParseError, ErrConfigIncomplete, ErrConfigInvalid,
		ErrConfigDecryptFailed,
	}
	if len(all) != 18 {
		t.Errorf("expected 18 sentinels, got %d", len(all))
	}
	seen := map[string]bool{}
	for _, e := range all {
		msg := e.Error()
		if !strings.HasPrefix(msg, "handler: ") {
			t.Errorf("sentinel %q must start with 'handler: '", msg)
		}
		if seen[msg] {
			t.Errorf("duplicate sentinel message: %q", msg)
		}
		seen[msg] = true
	}
}

// TestSentinels_ErrorsIsCompatible verifies sentinels work through %w wrap
// chains (§S16 wrap discipline).
//
// TestSentinels_ErrorsIsCompatible 验证 sentinel 经 %w wrap 链 errors.Is 通。
func TestSentinels_ErrorsIsCompatible(t *testing.T) {
	wrapped := fmt.Errorf("handlerstore.Get: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("errors.Is should unwrap %%w: got %v", wrapped)
	}
	double := fmt.Errorf("outer: %w", wrapped)
	if !errors.Is(double, ErrNotFound) {
		t.Errorf("errors.Is should unwrap two-level wrap: got %v", double)
	}
}

// TestStatusConstants_Stable pins status enum strings (DB CHECK literal).
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

// TestEnvStatusConstants_Stable pins env-status enum strings.
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

// TestConfigStateConstants_Stable pins computed ConfigState enum.
func TestConfigStateConstants_Stable(t *testing.T) {
	cases := map[string]string{
		"ConfigStateUnconfigured":        ConfigStateUnconfigured,
		"ConfigStatePartiallyConfigured": ConfigStatePartiallyConfigured,
		"ConfigStateReady":               ConfigStateReady,
	}
	expect := map[string]string{
		"ConfigStateUnconfigured":        "unconfigured",
		"ConfigStatePartiallyConfigured": "partially_configured",
		"ConfigStateReady":               "ready",
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

// TestAcceptedVersionCap pins the per-handler accepted-version cap.
func TestAcceptedVersionCap(t *testing.T) {
	if AcceptedVersionCap != 50 {
		t.Errorf("AcceptedVersionCap = %d, want 50", AcceptedVersionCap)
	}
}

// TestTableNames pins GORM table names.
func TestTableNames(t *testing.T) {
	if (Handler{}).TableName() != "handlers" {
		t.Errorf("Handler.TableName() = %q, want 'handlers'", (Handler{}).TableName())
	}
	if (Version{}).TableName() != "handler_versions" {
		t.Errorf("Version.TableName() = %q, want 'handler_versions'", (Version{}).TableName())
	}
}
