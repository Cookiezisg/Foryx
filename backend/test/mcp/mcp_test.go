//go:build pipeline

// mcp_test.go — pipeline coverage for the MCP integration. Per
// mcp.md §13 the suite covers five scenarios driving the real
// stdio Client + Service:
//
//  1. Tools_ListSearchCall_Closed_Loop — fakeserver → AddServer →
//     ListTools cached → CallTool echo → result text matches input.
//  2. BadCommand_MarksFailed — non-existent command at AddServer →
//     ServerStatus.Status=failed + LastError populated; row preserved
//     so the user can edit + reconnect.
//  3. Consecutive_Failures_Trigger_Degraded — fakeserver `fail` tool
//     called ≥ degradedThreshold (3) times in a row → ServerStatus
//     transitions ready → degraded; ConsecutiveFailures ≥ 3.
//  4. Auto_Heal_Back_To_Ready — after #3, one `echo` success flips
//     status back to ready (per §5.6 self-heal).
//  5. Live_RegistryInstallEverything — gated install of the
//     reference @modelcontextprotocol/server-everything via the
//     marketplace install flow. Requires sandbox.IsReady()
//     (mise-backed node + npm) AND env FORGIFY_LIVE_MCP_INSTALL=1
//     so the suite stays offline-friendly by default.
//
// The fake stdio MCP server is built once in TestMain (no go-run
// per scenario; binary launches are cheap once cached).
//
// mcp_test.go ——MCP 集成的 pipeline 覆盖。按 mcp.md §13 的 5 场景驱动真
// stdio Client + Service：(1) 列/搜/调闭环；(2) 坏命令 → failed；(3) 连
// 续失败 → degraded；(4) 成功后自愈回 ready；(5) Live_ 门控装
// `everything` server。Fake server 在 TestMain 一次性 build。
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

// fakeServerBin is the absolute path of the built fakeserver binary,
// populated by TestMain and read by every scenario.
//
// fakeServerBin 是 TestMain 编出的 fakeserver 绝对路径，每个场景读取。
var fakeServerBin string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "forgify-fake-mcp-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: mkdtemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	bin := filepath.Join(tmpDir, "fakeserver")
	// Build by module-qualified path so this works regardless of where
	// `go test` is invoked from. The fakeserver has no build tags so the
	// same compile works under -tags=pipeline.
	// 用 module 完整路径 build——`go test` 从哪跑都行。fakeserver 无 build
	// tag，-tags=pipeline 编同样命中。
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

// ── 1. happy path: list + search + call ──────────────────────────────

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

	// tools/list cached on connect; expect echo + fail + crash.
	// connect 时缓存的 tools/list；预期 echo + fail + crash。
	gotNames := map[string]bool{}
	for _, td := range st.Tools {
		gotNames[td.Name] = true
	}
	for _, want := range []string{"echo", "fail", "crash"} {
		if !gotNames[want] {
			t.Errorf("tools/list missing %q (got %v)", want, st.Tools)
		}
	}

	// CallTool echo with text payload.
	// CallTool 调 echo 带 text 参数。
	args, _ := json.Marshal(map[string]string{"text": "hello mcp"})
	out, err := h.MCP.CallTool(ctx, "fake", "echo", args)
	if err != nil {
		t.Fatalf("CallTool echo: %v", err)
	}
	if !strings.Contains(out, "hello mcp") {
		t.Errorf("echo result = %q, want contains %q", out, "hello mcp")
	}

	// recordCallResult on success bumps TotalCalls + sets LastSuccessAt.
	// recordCallResult 成功时增 TotalCalls + 设 LastSuccessAt。
	st2, _ := h.MCP.GetServer(ctx, "fake")
	if st2.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", st2.TotalCalls)
	}
	if st2.LastSuccessAt == nil {
		t.Error("LastSuccessAt = nil, want set after success")
	}
}

// ── 2. bad command → status=failed ───────────────────────────────────

func TestMCP_BadCommand_MarksFailed(t *testing.T) {
	h := th.New(t)
	ctx := context.Background()

	// AddServer's Connect step fails synchronously when the binary path
	// doesn't exist; we expect err != nil but the state row preserved
	// (per AddServer godoc: "Connect failure stays so user can edit").
	// AddServer 的 Connect 子进程起不来时同步 err；我们期望 err 非 nil 但
	// state 行保留（让用户编辑 + Reconnect）。
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

// ── 3 + 4. consecutive failures → degraded → auto-heal ──────────────

// Combined into one test because #4 explicitly depends on #3's
// degraded state being established first; running them as separate
// tests would either duplicate the setup or require ordering.
//
// 合并 #3 + #4：#4 显式依赖 #3 进入 degraded 后再走自愈；分开会重复
// setup 或要求顺序。
func TestMCP_Degraded_Then_AutoHeal(t *testing.T) {
	h := th.New(t)
	ctx := context.Background()

	if err := h.MCP.AddServer(ctx, mcpdomain.ServerConfig{
		Name:    "fake",
		Command: fakeServerBin,
	}); err != nil {
		t.Fatalf("AddServer fake: %v", err)
	}

	// Three back-to-back fail-tool calls. Each returns isError:true →
	// our stdio Client wraps as ErrToolCallFailed → recordCallResult
	// increments ConsecutiveFailures; on the 3rd it crosses
	// degradedThreshold and flips ready → degraded.
	// 连 3 次调 fail tool。每次返 isError:true → stdio Client 包成
	// ErrToolCallFailed → recordCallResult 增 ConsecutiveFailures；第 3
	// 次跨 degradedThreshold 翻 ready → degraded。
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

	// One success → recordCallResult sees wasDegraded → flips back to
	// ready, clears ConsecutiveFailures, sets LastSuccessAt.
	// 一次成功 → recordCallResult 见 wasDegraded → 翻回 ready，清零
	// ConsecutiveFailures，设 LastSuccessAt。
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

// ── 5. Live install via Registry ─────────────────────────────────────

// Gated double: needs sandbox v2 fully bootstrapped (mise embedded for
// host platform) AND env FORGIFY_LIVE_MCP_INSTALL=1 to opt in (npx
// fetches the package from npm; we don't want that on every CI run).
//
// 门控双条件：sandbox v2 已 bootstrap（host 平台 mise embed 在）且 env
// FORGIFY_LIVE_MCP_INSTALL=1（npx 拉 npm 包不该每次 CI 跑）。
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
