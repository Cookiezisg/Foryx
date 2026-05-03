package sandbox

import (
	"bytes"
	"strings"
	"testing"
)

// ── parseUVLine ───────────────────────────────────────────────────────────────

func TestParseUVLine_KnownStages(t *testing.T) {
	cases := []struct {
		line, wantStage string
	}{
		{"Resolved 12 packages in 1.5s", "resolving"},
		{"Resolved 1 package in 70ms", "resolving"},
		{"Prepared 12 packages in 800ms", "preparing"},
		{"Prepared 1 package in 200ms", "preparing"},
		{"Installed 12 packages in 200ms", "installing"},
		{"Installed 1 package in 1ms", "installing"},
	}
	for _, c := range cases {
		got := parseUVLine(c.line)
		if got == nil {
			t.Errorf("parseUVLine(%q) = nil, want stage %q", c.line, c.wantStage)
			continue
		}
		if got.Stage != c.wantStage {
			t.Errorf("parseUVLine(%q).Stage = %q, want %q", c.line, got.Stage, c.wantStage)
		}
		if got.Detail != c.line {
			t.Errorf("parseUVLine(%q).Detail = %q, want %q", c.line, got.Detail, c.line)
		}
	}
}

func TestParseUVLine_LeadingWhitespaceTrimmed(t *testing.T) {
	got := parseUVLine("   Resolved 12 packages in 1.5s")
	if got == nil || got.Stage != "resolving" {
		t.Errorf("parseUVLine should strip leading whitespace before matching; got %+v", got)
	}
}

func TestParseUVLine_UnknownReturnsNil(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"warning: something is up",
		"+ pandas==2.5.0",
		"Downloading numpy (15 MB)", // sub-progress, not a summary line
		"× No solution found when resolving dependencies",
		"some random output",
		"resolved: lowercase doesn't match",
	}
	for _, line := range cases {
		if got := parseUVLine(line); got != nil {
			t.Errorf("parseUVLine(%q) = %+v, want nil", line, got)
		}
	}
}

// ── scanProgress ──────────────────────────────────────────────────────────────

func TestScanProgress_DispatchesProgressLines(t *testing.T) {
	input := `Resolved 12 packages in 1.5s
Prepared 12 packages in 800ms
Installed 12 packages in 200ms
`
	var stages []string
	var details []string
	var errBuf bytes.Buffer

	scanProgress(strings.NewReader(input), func(stage, detail string) {
		stages = append(stages, stage)
		details = append(details, detail)
	}, &errBuf)

	wantStages := []string{"resolving", "preparing", "installing"}
	if len(stages) != 3 {
		t.Fatalf("expected 3 progress callbacks, got %d (%v)", len(stages), stages)
	}
	for i, s := range wantStages {
		if stages[i] != s {
			t.Errorf("stage[%d] = %q, want %q", i, stages[i], s)
		}
	}
	if errBuf.Len() != 0 {
		t.Errorf("errBuf should be empty when only progress lines present, got %q", errBuf.String())
	}
}

func TestScanProgress_BuffersUnknownLines(t *testing.T) {
	input := `× No solution found when resolving dependencies:
  ╰─▶ Because numpy>=2.0 requires Python>=3.12 and you require...
`
	var calls int
	var errBuf bytes.Buffer

	scanProgress(strings.NewReader(input), func(string, string) { calls++ }, &errBuf)

	if calls != 0 {
		t.Errorf("no progress lines, callback should not fire, got %d calls", calls)
	}
	if !strings.Contains(errBuf.String(), "No solution found") {
		t.Errorf("errBuf should contain unrecognized lines, got: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "numpy>=2.0 requires") {
		t.Errorf("errBuf should contain all unknown lines, got: %q", errBuf.String())
	}
}

func TestScanProgress_MixedLines(t *testing.T) {
	input := `Resolved 12 packages in 1.5s
warning: something flagged
Prepared 12 packages in 800ms
Downloading numpy (15 MB)
Installed 12 packages in 200ms
`
	var stages []string
	var errBuf bytes.Buffer

	scanProgress(strings.NewReader(input), func(stage, _ string) {
		stages = append(stages, stage)
	}, &errBuf)

	if len(stages) != 3 {
		t.Errorf("expected 3 progress callbacks, got %d", len(stages))
	}
	// warning + Downloading sub-progress should be buffered
	// warning + Downloading sub-progress 都被缓存
	if !strings.Contains(errBuf.String(), "warning") || !strings.Contains(errBuf.String(), "Downloading numpy") {
		t.Errorf("errBuf should contain non-stage lines, got: %q", errBuf.String())
	}
}

func TestScanProgress_NilCallback(t *testing.T) {
	input := `Resolved 12 packages in 1.5s
Prepared 12 packages in 800ms
`
	var errBuf bytes.Buffer

	// Must not panic on nil callback.
	// nil callback 不能 panic。
	scanProgress(strings.NewReader(input), nil, &errBuf)

	if errBuf.Len() != 0 {
		t.Errorf("errBuf should be empty when only progress lines present (even with nil cb), got %q", errBuf.String())
	}
}

func TestScanProgress_EmptyInput(t *testing.T) {
	var calls int
	var errBuf bytes.Buffer

	scanProgress(strings.NewReader(""), func(string, string) { calls++ }, &errBuf)

	if calls != 0 || errBuf.Len() != 0 {
		t.Errorf("empty input: calls=%d errBuf=%q", calls, errBuf.String())
	}
}
