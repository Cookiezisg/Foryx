package crypto

import (
	"errors"
	"testing"
)

func TestMachineFingerprint_ReturnsNonEmpty(t *testing.T) {
	fp, err := MachineFingerprint()
	if errors.Is(err, ErrNoFingerprint) {
		t.Skip("sandbox denies machine-ID probes, skipping:", err)
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp == "" {
		t.Errorf("fingerprint is empty string")
	}
}

func TestMachineFingerprint_Deterministic(t *testing.T) {
	a, err := MachineFingerprint()
	if errors.Is(err, ErrNoFingerprint) {
		t.Skip("sandbox denies machine-ID probes, skipping")
	}
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	b, err := MachineFingerprint()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if a != b {
		t.Errorf("fingerprint is not deterministic: %q vs %q", a, b)
	}
}

func TestMachineFingerprint_NoHardcodedFallback(t *testing.T) {
	// Regression: ensure no hardcoded fallback string ever leaks back in.
	// 回归：确保任何硬编码 fallback 不再泄漏。
	fp, _ := MachineFingerprint()
	forbidden := []string{"forgify-fallback-key", "fallback", "default", "unknown"}
	for _, bad := range forbidden {
		if fp == bad {
			t.Errorf("fingerprint returned forbidden fallback %q", bad)
		}
	}
}
