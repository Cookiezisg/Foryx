package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newTestBash() *Bash {
	return &Bash{mgr: NewProcessManager()}
}

func ctxWithAgentState(t *testing.T) (context.Context, *agentstatepkg.AgentState) {
	t.Helper()
	state := &agentstatepkg.AgentState{}
	return reqctxpkg.WithAgentState(context.Background(), state), state
}


func TestBash_IdentityMethods(t *testing.T) {
	tool := newTestBash()
	if tool.Name() != "Bash" {
		t.Errorf("Name = %q, want Bash", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters should not be empty")
	}
}

func TestBash_StaticMetadata(t *testing.T) {
	tool := newTestBash()
	if tool.IsReadOnly() {
		t.Error("Bash should not be read-only")
	}
	if tool.NeedsReadFirst() {
		t.Error("Bash should not require Read first")
	}
	if tool.RequiresWorkspace() {
		t.Error("Bash should NOT require workspace (PathGuard intentionally not applied)")
	}
}

func TestBash_Schema_IsParsableObject(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(bashSchema, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	props := doc["properties"].(map[string]any)
	for _, want := range []string{"command", "run_in_background", "timeout"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing property %q", want)
		}
	}
	// `description` was removed (Phase C cleanup): the framework's
	// standard `summary` field already covers the per-call human note,
	// so an extra Bash-only `description` was confusing the LLM into
	// populating both.
	// `description` 已删（Phase C 清理）：框架标准 `summary` 字段已覆盖
	// per-call human note，多余的 Bash-only `description` 让 LLM 混淆
	// 两个字段都填。
	if _, ok := props["description"]; ok {
		t.Error("schema must NOT carry a separate `description` field; use the standard `summary` instead")
	}
}


func TestBash_ValidateInput_RequiresCommand(t *testing.T) {
	tool := newTestBash()
	if err := tool.ValidateInput(json.RawMessage(`{}`)); !errors.Is(err, ErrEmptyCommand) {
		t.Fatalf("want ErrEmptyCommand, got %v", err)
	}
	if err := tool.ValidateInput(json.RawMessage(`{"command":"   "}`)); !errors.Is(err, ErrEmptyCommand) {
		t.Fatalf("whitespace command should fail, got %v", err)
	}
}

func TestBash_ValidateInput_RejectsOutOfRangeTimeout(t *testing.T) {
	tool := newTestBash()
	if err := tool.ValidateInput(json.RawMessage(`{"command":"x","timeout":-1}`)); err == nil {
		t.Error("negative timeout should fail")
	}
	hugeTimeout := fmt.Sprintf(`{"command":"x","timeout":%d}`, maxTimeoutMS+1)
	if err := tool.ValidateInput(json.RawMessage(hugeTimeout)); err == nil {
		t.Error("over-cap timeout should fail")
	}
}

func TestBash_ValidateInput_AcceptsValidArgs(t *testing.T) {
	tool := newTestBash()
	if err := tool.ValidateInput(json.RawMessage(`{"command":"ls","timeout":5000}`)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}


func TestParseCDOnly_HandlesPlainCD(t *testing.T) {
	cases := []struct {
		in       string
		wantPath string
		wantOK   bool
	}{
		{"cd", "", true},
		{"  cd  ", "", true},
		{"cd /tmp", "/tmp", true},
		{"  cd   /tmp  ", "/tmp", true},
		{"cd ~/projects", "~/projects", true},
		{`cd "/tmp/with space"`, "/tmp/with space", true},
		{`cd '/tmp/single'`, "/tmp/single", true},
		// Chains and metachars must NOT be treated as cd-only.
		// 链式 / 元字符不该被当 cd-only。
		{"cd /tmp && ls", "", false},
		{"cd /tmp; ls", "", false},
		{"cd `pwd`", "", false},
		{"cd $HOME", "", false},
		{"cd /tmp | tee log", "", false},
		// Things that aren't cd at all.
		// 根本不是 cd 的。
		{"ls", "", false},
		{"cdr", "", false},
	}
	for _, c := range cases {
		gotPath, gotOK := parseCDOnly(c.in)
		if gotPath != c.wantPath || gotOK != c.wantOK {
			t.Errorf("parseCDOnly(%q) = (%q, %v), want (%q, %v)",
				c.in, gotPath, gotOK, c.wantPath, c.wantOK)
		}
	}
}


func TestBash_Execute_CD_UpdatesAgentState(t *testing.T) {
	tool := newTestBash()
	ctx, state := ctxWithAgentState(t)
	dir := t.TempDir()
	body := fmt.Sprintf(`{"command":"cd %s"}`, dir)
	out, err := tool.Execute(ctx, body)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Changed working directory") {
		t.Errorf("expected confirmation, got: %q", out)
	}
	if got := state.Cwd(); got != filepath.Clean(dir) {
		t.Errorf("Cwd = %q, want %q", got, filepath.Clean(dir))
	}
}

func TestBash_Execute_CD_RejectsNonexistentDir(t *testing.T) {
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	missing := filepath.Join(t.TempDir(), "nope")
	out, err := tool.Execute(ctx, fmt.Sprintf(`{"command":"cd %s"}`, missing))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "cd:") {
		t.Errorf("expected cd error, got: %q", out)
	}
}

func TestBash_Execute_CD_RejectsFile(t *testing.T) {
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := tool.Execute(ctx, fmt.Sprintf(`{"command":"cd %s"}`, f))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not a directory") {
		t.Errorf("expected not-a-dir error, got: %q", out)
	}
}

func TestBash_Execute_CD_RelativeResolvesAgainstCwd(t *testing.T) {
	tool := newTestBash()
	ctx, state := ctxWithAgentState(t)
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	state.SetCwd(parent)
	out, err := tool.Execute(ctx, `{"command":"cd child"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Changed working directory") {
		t.Errorf("expected confirmation, got: %q", out)
	}
	if got := state.Cwd(); got != filepath.Clean(child) {
		t.Errorf("Cwd = %q, want %q", got, filepath.Clean(child))
	}
}


func TestBash_Execute_Foreground_EchoCapturesStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	out, err := tool.Execute(ctx, `{"command":"echo hello forgify"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello forgify") {
		t.Errorf("expected echoed text, got: %q", out)
	}
	if !strings.Contains(out, "[exit code: 0]") {
		t.Errorf("expected exit code footer, got: %q", out)
	}
}

func TestBash_Execute_Foreground_NonZeroExitReportedInFooter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	out, err := tool.Execute(ctx, `{"command":"exit 7"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "[exit code: 7]") {
		t.Errorf("expected non-zero exit code in footer, got: %q", out)
	}
}

func TestBash_Execute_Foreground_StderrAlsoCaptured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	out, err := tool.Execute(ctx, `{"command":"echo err 1>&2; echo ok"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "err") || !strings.Contains(out, "ok") {
		t.Errorf("expected both stderr and stdout in output, got: %q", out)
	}
}

func TestBash_Execute_Foreground_TimeoutFiresFootnote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	start := time.Now()
	out, err := tool.Execute(ctx, `{"command":"sleep 5","timeout":200}`)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "timed out") {
		t.Errorf("expected timeout note, got: %q", out)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Execute took %v; should be near 200ms timeout", elapsed)
	}
}

func TestBash_Execute_Foreground_RespectsCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, state := ctxWithAgentState(t)
	dir := t.TempDir()
	state.SetCwd(dir)
	out, err := tool.Execute(ctx, `{"command":"pwd"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// macOS resolves /var → /private/var, so EvalSymlinks the expected
	// path before comparing.
	// macOS 把 /var → /private/var；比对前对预期路径 EvalSymlinks。
	expected, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(out, expected) && !strings.Contains(out, dir) {
		t.Errorf("pwd should reflect AgentState cwd; got: %q (expected to contain %q)", out, expected)
	}
}

// Regression: when the parent ctx is cancelled (user hits Cancel on the
// conversation), the killed command must surface as "[cancelled]" — not the
// confusing "[exec failed: signal: killed]" the LLM would otherwise see.
//
// 回归：父 ctx 取消（用户在对话里点 Cancel）时，被杀命令必须报 "[cancelled]"，
// 不能再报 "[exec failed: signal: killed]" 让 LLM 误以为命令自己崩了。
func TestBash_Execute_Foreground_ParentCtxCancelled_ReportsCancelled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	state := &agentstatepkg.AgentState{}
	parentCtx, parentCancel := context.WithCancel(reqctxpkg.WithAgentState(context.Background(), state))
	go func() {
		time.Sleep(100 * time.Millisecond)
		parentCancel()
	}()
	out, err := tool.Execute(parentCtx, `{"command":"sleep 5"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "cancelled") {
		t.Errorf("expected '[cancelled]' footer, got: %q", out)
	}
	if strings.Contains(out, "exec failed") {
		t.Errorf("regression: 'exec failed' should not appear on cancellation, got: %q", out)
	}
}


func TestBash_Execute_Background_ReturnsBashID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	out, err := tool.Execute(ctx, `{"command":"sleep 5","run_in_background":true}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "bash_id=bsh_") {
		t.Errorf("expected bash_id in result, got: %q", out)
	}
	// Verify the process actually got registered.
	// 验证进程确实进了注册表。
	if len(tool.mgr.procs) != 1 {
		t.Errorf("expected 1 registered process, got %d", len(tool.mgr.procs))
	}
	// Cleanup: kill the sleeping bg child.
	// 清理：杀掉睡眠的后台子进程。
	for _, p := range tool.mgr.procs {
		if p.Cmd != nil && p.Cmd.Process != nil {
			_ = p.Cmd.Process.Kill()
		}
	}
}

func TestBash_Execute_Background_OutputCapturedForPolling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell semantics; Windows behavior pending real Windows test environment (D10)")
	}
	tool := newTestBash()
	ctx, _ := ctxWithAgentState(t)
	out, err := tool.Execute(ctx, `{"command":"echo bg-output-marker; sleep 0.05","run_in_background":true}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Find the spawned process and wait briefly for output capture.
	// 找到 spawn 的进程，稍等输出捕获。
	if !strings.Contains(out, "bash_id=bsh_") {
		t.Fatalf("expected bash_id in result")
	}
	var procID string
	for id := range tool.mgr.procs {
		procID = id
		break
	}
	deadline := time.Now().Add(2 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		proc, err := tool.mgr.Get(procID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		newBytes, _, _, _ := proc.drainNew()
		got += string(newBytes)
		if strings.Contains(got, "bg-output-marker") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("never saw bg-output-marker; got: %q", got)
}


func TestResolveCwd_FallsBackToProcessCwd_WhenNoAgentState(t *testing.T) {
	got := resolveCwd(context.Background())
	if got == "" || got == "/" {
		t.Errorf("expected real process cwd, got %q", got)
	}
}

func TestResolveCwd_PrefersAgentStateCwd(t *testing.T) {
	state := &agentstatepkg.AgentState{}
	state.SetCwd("/tmp")
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	if got := resolveCwd(ctx); got != "/tmp" {
		t.Errorf("got %q, want /tmp", got)
	}
}


func TestCapOutput_BelowLimit_Pass(t *testing.T) {
	got := capOutput([]byte("short"))
	if got != "short" {
		t.Errorf("got %q", got)
	}
}

func TestCapOutput_OverLimit_TruncatesFromHead(t *testing.T) {
	huge := make([]byte, outputCapBytes+50)
	for i := range huge {
		huge[i] = 'A'
	}
	huge[len(huge)-1] = 'Z' // marker at the tail to verify it survived
	got := capOutput(huge)
	if !strings.HasPrefix(got, "...[truncated") {
		t.Errorf("expected truncation marker prefix, got: %q", got[:60])
	}
	if !strings.HasSuffix(got, "Z") {
		t.Errorf("tail marker missing: %q", got[len(got)-10:])
	}
}
