//go:build pipeline

// Package mcp runs pipeline coverage for the MCP integration.
//
// Package mcp 跑 MCP 集成 pipeline 覆盖。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// fakeServerBin is the absolute path of the built fakeserver, populated by TestMain.
//
// fakeServerBin 是 TestMain 编出的 fakeserver 绝对路径。
var fakeServerBin string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "forgify-fake-mcp-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: mkdtemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	bin := filepath.Join(tmpDir, "fakeserver")
	cmd := exec.Command("go", "build", "-o", bin,
		"github.com/sunweilin/forgify/backend/test/mcp/fakeserver")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: build fakeserver: %v\n", err)
		os.Exit(1)
	}
	fakeServerBin = bin

	os.Exit(m.Run())
}

func TestMCP_Tools_ListSearchCall_Closed_Loop(t *testing.T) {
	h := th.New(t)
	ctx := context.Background()

	if err := h.MCP.AddServer(ctx, mcpdomain.ServerConfig{
		Name:    "fake",
		Command: fakeServerBin,
	}); err != nil {
		t.Fatalf("AddServer fake: %v", err)
	}

	st, err := h.MCP.GetServer(ctx, "fake")
	if err != nil {
		t.Fatalf("GetServer fake: %v", err)
	}
	if st.Status != mcpdomain.StatusReady {
		t.Fatalf("status = %q, want ready (lastErr=%q)", st.Status, st.LastError)
	}

	gotNames := map[string]bool{}
	for _, td := range st.Tools {
		gotNames[td.Name] = true
	}
	for _, want := range []string{"echo", "fail", "crash"} {
		if !gotNames[want] {
			t.Errorf("tools/list missing %q (got %v)", want, st.Tools)
		}
	}

	args, _ := json.Marshal(map[string]string{"text": "hello mcp"})
	out, err := h.MCP.CallTool(ctx, "fake", "echo", args)
	if err != nil {
		t.Fatalf("CallTool echo: %v", err)
	}
	if !strings.Contains(out, "hello mcp") {
		t.Errorf("echo result = %q, want contains %q", out, "hello mcp")
	}

	st2, _ := h.MCP.GetServer(ctx, "fake")
	if st2.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", st2.TotalCalls)
	}
	if st2.LastSuccessAt == nil {
		t.Error("LastSuccessAt = nil, want set after success")
	}
}

func TestMCP_BadCommand_MarksFailed(t *testing.T) {
	h := th.New(t)
	ctx := context.Background()

	err := h.MCP.AddServer(ctx, mcpdomain.ServerConfig{
		Name:    "bogus",
		Command: filepath.Join(t.TempDir(), "definitely-not-here"),
	})
	if err == nil {
		t.Fatal("AddServer returned nil for missing binary; expected connect error")
	}

	st, gerr := h.MCP.GetServer(ctx, "bogus")
	if gerr != nil {
		t.Fatalf("GetServer bogus after failed connect: %v", gerr)
	}
	if st.Status != mcpdomain.StatusFailed {
		t.Errorf("status = %q, want failed", st.Status)
	}
	if st.LastError == "" {
		t.Error("LastError empty; expected populated after connect failure")
	}
	if st.LastErrorAt == nil {
		t.Error("LastErrorAt nil; expected set on connect failure")
	}
}

func TestMCP_Degraded_Then_AutoHeal(t *testing.T) {
	h := th.New(t)
	ctx := context.Background()

	if err := h.MCP.AddServer(ctx, mcpdomain.ServerConfig{
		Name:    "fake",
		Command: fakeServerBin,
	}); err != nil {
		t.Fatalf("AddServer fake: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := h.MCP.CallTool(ctx, "fake", "fail", nil); err == nil {
			t.Fatalf("CallTool fail #%d returned nil err; expected isError pass-through", i+1)
		}
	}
	st, _ := h.MCP.GetServer(ctx, "fake")
	if st.Status != mcpdomain.StatusDegraded {
		t.Fatalf("status = %q, want degraded after 3 failures", st.Status)
	}
	if st.ConsecutiveFailures < 3 {
		t.Errorf("ConsecutiveFailures = %d, want ≥ 3", st.ConsecutiveFailures)
	}

	args, _ := json.Marshal(map[string]string{"text": "back from the dead"})
	if _, err := h.MCP.CallTool(ctx, "fake", "echo", args); err != nil {
		t.Fatalf("CallTool echo (heal): %v", err)
	}
	st2, _ := h.MCP.GetServer(ctx, "fake")
	if st2.Status != mcpdomain.StatusReady {
		t.Errorf("status = %q after success, want ready (auto-heal)", st2.Status)
	}
	if st2.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d after success, want 0", st2.ConsecutiveFailures)
	}
	if st2.LastSuccessAt == nil {
		t.Error("LastSuccessAt nil after heal, want set")
	}
}

func TestMCP_Live_RegistryInstallEverything(t *testing.T) {
	if os.Getenv("FORGIFY_LIVE_MCP_INSTALL") != "1" {
		t.Skip("set FORGIFY_LIVE_MCP_INSTALL=1 to opt in (downloads npm package)")
	}
	h := th.New(t)
	if !h.Sandbox.IsReady() {
		t.Skip("sandbox not ready (run `make resources` to embed mise)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	st, err := h.MCP.InstallFromRegistry(ctx, "everything", nil, nil)
	if err != nil {
		t.Fatalf("InstallFromRegistry everything: %v", err)
	}
	if st.Status != mcpdomain.StatusReady {
		t.Errorf("status = %q after install, want ready (lastErr=%q)", st.Status, st.LastError)
	}
	if len(st.Tools) == 0 {
		t.Error("tools/list empty after install; everything server should expose multiple tools")
	}
}
