//go:build e2e

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
)

// TestE2E_Context7ViaNpx is a REAL-MACHINE end-to-end of the whole mcp stdio path: extract the
// embedded mise → install node → create an env → ResolveExec turns `npx` into <runtime>/bin/npx
// → SpawnLongLived (with the runtime's bin on PATH so npx's `#!/usr/bin/env node` shebang
// resolves) → go-sdk Initialize + ListTools against the LIVE context7 MCP server (node, no API
// key needed). It proves the sandbox runtime-tool rework actually launches a real server.
//
// Run: go test -tags e2e -run TestE2E_Context7ViaNpx -v -timeout 300s ./internal/infra/mcp/
//
// 真机端到端，跑通整条 mcp stdio 路径：抽 embed mise → 装 node → 建 env → ResolveExec 把 `npx`
// 解析为 <runtime>/bin/npx → SpawnLongLived（runtime bin 在 PATH，使 npx 的 `env node` shebang
// 解析）→ go-sdk 对 live context7 server（node，无需 key）Initialize + ListTools。证明 sandbox
// runtime-tool 回改真能拉起真 server。
func TestE2E_Context7ViaNpx(t *testing.T) {
	root := t.TempDir()
	log, _ := zap.NewDevelopment()
	ctx := context.Background()

	mise, err := sandboxinfra.ExtractMiseBinary(ctx, root, log)
	if err != nil {
		t.Fatalf("extract mise: %v", err)
	}
	rel, err := sandboxinfra.NewMiseInstaller(mise, "node", "22").Install(ctx, "22", root, nil)
	if err != nil {
		t.Fatalf("install node: %v", err)
	}
	nodeAbs := filepath.Join(root, rel)
	t.Logf("node runtime @ %s", nodeAbs)

	mgr := sandboxinfra.NewNodeEnvManager()
	envPath := filepath.Join(root, "env")
	if err := mgr.CreateEnv(ctx, nodeAbs, envPath); err != nil {
		t.Fatalf("create env: %v", err)
	}

	cmd, args, cwd := mgr.ResolveExec(nodeAbs, envPath, sandboxdomain.SpawnOpts{
		Cmd:  "npx",
		Args: []string{"-y", "@upstash/context7-mcp"},
	})
	t.Logf("resolved spawn: %s %v", cmd, args)
	if want := filepath.Join(nodeAbs, "bin", "npx"); cmd != want {
		t.Fatalf("npx should resolve to %q, got %q", want, cmd)
	}

	// PATH must carry the runtime's bin (npx → `#!/usr/bin/env node`); mirrors prepareSpawn.
	binDir := filepath.Join(nodeAbs, "bin")
	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	handle, err := sandboxinfra.SpawnLongLived(ctx, sandboxinfra.SpawnOptions{Cmd: cmd, Args: args, Cwd: cwd, Env: env})
	if err != nil {
		t.Fatalf("spawn npx: %v", err)
	}
	defer func() { _ = handle.Kill() }()
	t.Logf("spawned pid=%d", handle.PID())

	cl := NewClient(ClientSpec{
		Name:   "context7",
		Stdin:  handle.Stdin(),
		Stdout: handle.Stdout(),
		Stderr: handle.Stderr(),
	}, log)
	ictx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()

	if err := cl.Initialize(ictx); err != nil {
		t.Fatalf("initialize (stderr tail: %s): %v", cl.StderrTail(), err)
	}
	tools, err := cl.ListTools(ictx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatalf("expected ≥1 tool from context7 (stderr: %s)", cl.StderrTail())
	}
	for _, tl := range tools {
		t.Logf("✓ context7 tool: %s", tl.Name)
	}
	_ = cl.Close()
}
