package sandbox

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// ── ComputeEnvID ──────────────────────────────────────────────────────────────

func TestComputeEnvID_Stable(t *testing.T) {
	a := ComputeEnvID([]string{"pandas>=2.0"}, ">=3.12")
	b := ComputeEnvID([]string{"pandas>=2.0"}, ">=3.12")
	if a != b {
		t.Fatalf("EnvID not stable across calls: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "env_") {
		t.Errorf("EnvID must have env_ prefix, got %q", a)
	}
	if len(a) != len("env_")+12 {
		t.Errorf("EnvID hex portion must be 12 chars (6 bytes), got %q (len=%d)", a, len(a))
	}
}

func TestComputeEnvID_OrderIndependent(t *testing.T) {
	a := ComputeEnvID([]string{"pandas>=2.0", "numpy"}, ">=3.12")
	b := ComputeEnvID([]string{"numpy", "pandas>=2.0"}, ">=3.12")
	if a != b {
		t.Errorf("EnvID should be order-independent: %q vs %q", a, b)
	}
}

func TestComputeEnvID_CaseInsensitivePackageName(t *testing.T) {
	a := ComputeEnvID([]string{"Pandas>=2.0"}, ">=3.12")
	b := ComputeEnvID([]string{"pandas>=2.0"}, ">=3.12")
	c := ComputeEnvID([]string{"PANDAS>=2.0"}, ">=3.12")
	if a != b || b != c {
		t.Errorf("EnvID should be case-insensitive on package name: %q / %q / %q", a, b, c)
	}
}

func TestComputeEnvID_WhitespaceTrimmed(t *testing.T) {
	a := ComputeEnvID([]string{"  pandas>=2.0 "}, "  >=3.12  ")
	b := ComputeEnvID([]string{"pandas>=2.0"}, ">=3.12")
	if a != b {
		t.Errorf("EnvID should ignore leading/trailing whitespace: %q vs %q", a, b)
	}
}

func TestComputeEnvID_BlankSpecifierFiltered(t *testing.T) {
	a := ComputeEnvID([]string{"pandas", "", "  "}, ">=3.12")
	b := ComputeEnvID([]string{"pandas"}, ">=3.12")
	if a != b {
		t.Errorf("blank specifiers should be filtered: %q vs %q", a, b)
	}
}

func TestComputeEnvID_DifferentSpecifiersDiffer(t *testing.T) {
	cases := []struct{ a, b string }{
		{"pandas", "pandas>=2.0"},        // unconstrained vs constrained
		{"pandas>=2.0", "pandas==2.0"},   // range vs pinned
		{"pandas>=2.0", "pandas>=2.0.0"}, // string-different even if PEP 440 equivalent
		{"pandas>=2.0", "pandas>2.0"},    // semantically different
		{"pandas", "numpy"},              // different package
	}
	for _, c := range cases {
		a := ComputeEnvID([]string{c.a}, ">=3.12")
		b := ComputeEnvID([]string{c.b}, ">=3.12")
		if a == b {
			t.Errorf("EnvID should differ for %q vs %q (got both = %q)", c.a, c.b, a)
		}
	}
}

func TestComputeEnvID_DifferentPythonVersionDiffers(t *testing.T) {
	a := ComputeEnvID([]string{"pandas"}, ">=3.12")
	b := ComputeEnvID([]string{"pandas"}, ">=3.13")
	if a == b {
		t.Errorf("EnvID should differ for different python versions: both = %q", a)
	}
}

func TestComputeEnvID_EmptyDeps(t *testing.T) {
	a := ComputeEnvID(nil, ">=3.12")
	b := ComputeEnvID([]string{}, ">=3.12")
	if a != b {
		t.Errorf("nil and empty deps should be equivalent: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "env_") {
		t.Errorf("empty deps still produce a valid EnvID, got %q", a)
	}
}

// ── normalizeSpecifier ────────────────────────────────────────────────────────

func TestNormalizeSpecifier(t *testing.T) {
	cases := []struct{ in, want string }{
		{"pandas", "pandas"},
		{"Pandas", "pandas"},
		{" PANDAS  ", "pandas"},
		{"pandas>=2.0", "pandas>=2.0"},
		{"Pandas>=2.0", "pandas>=2.0"},
		{"PANDAS-DataReader>=0.10", "pandas-datareader>=0.10"},
		{"my_package==1.0", "my_package==1.0"},
		{" requests ", "requests"},
		{"pandas[excel]>=2.0", "pandas[excel]>=2.0"}, // extras kept verbatim
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		got := normalizeSpecifier(c.in)
		if got != c.want {
			t.Errorf("normalizeSpecifier(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── Path helpers ──────────────────────────────────────────────────────────────

func TestBundledPythonPath(t *testing.T) {
	got := bundledPythonPath("/data")
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(got, "python.exe") {
			t.Errorf("windows python path should end with python.exe, got %q", got)
		}
	} else {
		if !strings.HasSuffix(got, "bin/python3") {
			t.Errorf("unix python path should end with bin/python3, got %q", got)
		}
	}
}

func TestBundledUVPath(t *testing.T) {
	got := bundledUVPath("/data")
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(got, "uv.exe") {
			t.Errorf("windows uv path should end with uv.exe, got %q", got)
		}
	} else {
		if !strings.HasSuffix(got, "/uv") {
			t.Errorf("unix uv path should end with /uv, got %q", got)
		}
	}
}

func TestEnvAndVersionAndForgeDir(t *testing.T) {
	if got := envDir("/data", "f_x", "env_y"); !strings.Contains(got, "f_x") || !strings.Contains(got, "env_y") || !strings.Contains(got, "envs") {
		t.Errorf("envDir wrong: %q", got)
	}
	if got := versionDir("/data", "f_x", "fv_y"); !strings.Contains(got, "f_x") || !strings.Contains(got, "fv_y") || !strings.Contains(got, "versions") {
		t.Errorf("versionDir wrong: %q", got)
	}
	if got := forgeDir("/data", "f_x"); !strings.HasSuffix(got, "f_x") {
		t.Errorf("forgeDir wrong: %q", got)
	}
	if got := uvCacheDir("/data"); !strings.HasSuffix(got, "uv-cache") {
		t.Errorf("uvCacheDir wrong: %q", got)
	}
}

// ── forgeMutexMap ─────────────────────────────────────────────────────────────

func TestForgeMutexMap_DifferentForgesParallel(t *testing.T) {
	m := newForgeMutexMap()

	// Hold forge_a's lock; locking forge_b must not block.
	unlockA := m.Lock("forge_a")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB := m.Lock("forge_b")
		close(done)
		unlockB()
	}()

	select {
	case <-done:
		// expected: different forges don't block each other
	case <-time.After(100 * time.Millisecond):
		t.Error("Lock on different forge should not block")
	}
}

func TestForgeMutexMap_SameForgeSerial(t *testing.T) {
	m := newForgeMutexMap()

	unlock1 := m.Lock("forge_a")

	// Second Lock on same forge must block until first is released.
	locked := make(chan struct{})
	go func() {
		unlock2 := m.Lock("forge_a")
		close(locked)
		unlock2()
	}()

	select {
	case <-locked:
		t.Error("second Lock on same forge should block while first holds")
	case <-time.After(50 * time.Millisecond):
		// expected: blocked
	}

	unlock1()

	select {
	case <-locked:
		// expected: now released and acquired
	case <-time.After(100 * time.Millisecond):
		t.Error("second Lock should acquire after first unlock")
	}
}

func TestForgeMutexMap_LockSameForgeAgainAfterUnlock(t *testing.T) {
	m := newForgeMutexMap()

	for i := 0; i < 3; i++ {
		unlock := m.Lock("forge_a")
		unlock()
	}
	// Should reach here without hanging — repeated lock+unlock on same forge
	// works without leaking.
}
