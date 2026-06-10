package handler

import "testing"

// TestYieldBytes: a streaming method's yield renders as one terminal line — strings verbatim,
// anything else as compact JSON — for the entities run terminal.
//
// TestYieldBytes：流式 method 的 yield 渲成 entities run 终端的一行——string 原样、其余 compact JSON。
func TestYieldBytes(t *testing.T) {
	if got := string(yieldBytes("step 1")); got != "step 1\n" {
		t.Fatalf("string yield: got %q", got)
	}
	if got := string(yieldBytes(map[string]any{"pct": 50})); got != `{"pct":50}`+"\n" {
		t.Fatalf("object yield: got %q", got)
	}
}
