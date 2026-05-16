package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

func TestEmbed_MiseBinaryIsNonTrivial(t *testing.T) {
	if len(miseBinary) < 1<<20 {
		t.Fatalf("embed mise binary suspiciously small: %d bytes (want >1 MB). Run `make resources` to populate?",
			len(miseBinary))
	}
}

func TestExtractMiseBinary_HappyPath(t *testing.T) {
	sandboxRoot := t.TempDir()
	log := zap.NewNop()
	ctx := context.Background()

	first, err := ExtractMiseBinary(ctx, sandboxRoot, log)
	if err != nil {
		t.Fatalf("first ExtractMiseBinary: %v", err)
	}
	st1, err := os.Stat(first)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	const sizeTolerance = 10 << 20
	embedSize := int64(len(miseBinary))
	if diff := st1.Size() - embedSize; diff > sizeTolerance || diff < -sizeTolerance {
		t.Errorf("extracted binary size %d drifts >10MB from embed %d (diff %d)", st1.Size(), embedSize, diff)
	}
	if st1.Mode().Perm()&0o111 == 0 {
		t.Errorf("extracted binary not executable: mode %v", st1.Mode())
	}

	hashFile := filepath.Join(sandboxRoot, ".mise.hash")
	if _, err := os.Stat(hashFile); err != nil {
		t.Errorf("hash file not written: %v", err)
	}

	second, err := ExtractMiseBinary(ctx, sandboxRoot, log)
	if err != nil {
		t.Fatalf("second ExtractMiseBinary: %v", err)
	}
	if second != first {
		t.Errorf("path drift between calls: %q vs %q", first, second)
	}
	st2, err := os.Stat(first)
	if err != nil {
		t.Fatalf("re-stat: %v", err)
	}
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Errorf("idempotency broken: mtime changed (%v -> %v) — re-extract triggered when hash matched",
			st1.ModTime(), st2.ModTime())
	}
}

func TestExtractMiseBinary_RecoversAfterBinaryDeleted(t *testing.T) {
	sandboxRoot := t.TempDir()
	log := zap.NewNop()
	ctx := context.Background()

	first, err := ExtractMiseBinary(ctx, sandboxRoot, log)
	if err != nil {
		t.Fatalf("first ExtractMiseBinary: %v", err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatalf("remove binary mid-test: %v", err)
	}

	second, err := ExtractMiseBinary(ctx, sandboxRoot, log)
	if err != nil {
		t.Fatalf("re-extract after deletion: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Errorf("binary not re-extracted: %v", err)
	}
}

var _ sandboxdomain.RuntimeInstaller = (*MiseInstaller)(nil)

func TestMiseInstaller_Kind(t *testing.T) {
	cases := []string{"python", "node", "rust", "go", "java", "ruby", "php"}
	for _, kind := range cases {
		mi := NewMiseInstaller("/tmp/mise", kind, "1.0")
		if got := mi.Kind(); got != kind {
			t.Errorf("Kind() = %q, want %q", got, kind)
		}
	}
}

func TestMiseInstaller_ResolveDefault_ReturnsConstructionVersion(t *testing.T) {
	cases := map[string]string{
		"3.12":   "3.12",
		"22":     "22",
		"3.12.5": "3.12.5",
		"stable": "stable",
		"":       "",
	}
	for input, want := range cases {
		mi := NewMiseInstaller("/tmp/mise", "python", input)
		got, err := mi.ResolveDefault(context.Background())
		if err != nil {
			t.Errorf("ResolveDefault(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ResolveDefault(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMiseInstaller_NormalizeVersion_StripsRangePrefixes(t *testing.T) {
	mi := NewMiseInstaller("/tmp/mise", "python", "3.12")
	cases := map[string]string{
		"3.12":    "3.12",
		">=3.12":  "3.12",
		"<=3.12":  "3.12",
		"~=3.12":  "3.12",
		"==3.12":  "3.12",
		">3.12":   "3.12",
		"<3.12":   "3.12",
		"~3.12":   "3.12",
		"^3.12":   "3.12",
		">= 3.12": "3.12",
		"3.12.5":  "3.12.5",
		"":        "",
	}
	for input, want := range cases {
		if got := mi.NormalizeVersion(input); got != want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}
