// Package harness boots the REAL backend binary and gives scenarios a typed black-box
// view of it. testend imports NOTHING from backend/ on purpose: it consumes pure
// HTTP/SSE exactly like the frontend will — every awkwardness a scenario hits here is a
// frontend-developer-experience finding, not a harness bug to paper over.
//
// Package harness 拉起**真实** backend 二进制，给场景一个带类型的黑盒视图。testend 刻意
// 不 import backend/ 任何代码：它像未来前端一样消费纯 HTTP/SSE——场景在这里碰到的每个
// 别扭都是前端开发者体验 finding，不是 harness 该兜的。
package harness

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// HeaderWorkspace is the workspace-identity header (wire fact from api.md, restated here
// because testend does not import backend).
//
// HeaderWorkspace 是 workspace 身份头（api.md 的线缆事实；testend 不 import backend 故复述）。
const HeaderWorkspace = "X-Forgify-Workspace-ID"

// Server is one running backend instance on a throwaway data dir.
//
// Server 是一个跑在一次性数据目录上的 backend 实例。
type Server struct {
	BaseURL string
	DataDir string
	cmd     *exec.Cmd
}

var (
	buildOnce sync.Once
	buildErr  error
	binPath   string
)

// binary builds cmd/server once per test run into a shared temp location.
//
// binary 每次测试运行只编译一次 cmd/server，落共享临时位置。
func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "testend-bin-*")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "forgify-server")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/server")
		cmd.Dir = backendDir()
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("build backend: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatalf("harness: %v", buildErr)
	}
	return binPath
}

// backendDir resolves ../backend relative to this package (testend sits beside backend).
//
// backendDir 解析本包旁的 ../backend（testend 与 backend 平级）。
func backendDir() string {
	wd, _ := os.Getwd()
	for d := wd; d != "/"; d = filepath.Dir(d) {
		cand := filepath.Join(d, "backend", "cmd", "server")
		if _, err := os.Stat(cand); err == nil {
			return filepath.Join(d, "backend")
		}
	}
	return ""
}

// runtimeCache returns the shared sandbox-runtime cache dir: real runs download python/
// node once, then every later Server boot pre-seeds from here instead of re-downloading.
//
// runtimeCache 返回共享 sandbox 运行时缓存目录：真跑首次下载 python/node 后，之后每次
// Server 启动从这里预置、不再重复下载。
func runtimeCache() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".forgify-testend-cache")
}

// Start boots a fresh backend on a free port + temp data dir, waits for health, and
// registers cleanup (kill + runtime cache save-back).
//
// Start 在空闲端口 + 临时数据目录上拉起全新 backend，等 health，注册清理（杀进程 +
// 运行时缓存回存）。
func Start(t *testing.T) *Server {
	t.Helper()
	bin := binary(t)
	dataDir := t.TempDir()

	// Pre-seed sandbox runtimes from the cache so scenarios that execute code don't
	// re-download per run. 从缓存预置运行时，免得执行类场景每次重新下载。
	if cache := runtimeCache(); cache != "" {
		if _, err := os.Stat(filepath.Join(cache, "sandbox")); err == nil {
			_ = exec.Command("cp", "-R", filepath.Join(cache, "sandbox"), filepath.Join(dataDir, "sandbox")).Run()
		}
	}

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"FORGIFY_DATA_DIR="+dataDir,
		"FORGIFY_ADDR="+addr,
	)
	cmd.Stdout = os.Stderr // backend logs interleave with test output for diagnosis. 后端日志混入测试输出便于诊断。
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("harness: start backend: %v", err)
	}
	s := &Server{BaseURL: "http://" + addr, DataDir: dataDir, cmd: cmd}
	t.Cleanup(func() {
		s.saveRuntimeCache()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	s.waitHealthy(t, 30*time.Second)
	return s
}

// saveRuntimeCache merges downloaded runtimes back into the shared cache per kind (first
// run pays, the rest ride). Per-kind, not all-or-nothing: python landing first must not
// block node/llamasrv/embedmodel downloaded by later waves from ever being cached.
//
// saveRuntimeCache 把已下载的运行时按 kind 合并回共享缓存（首跑买单、后跑搭车）。按 kind
// 而非 all-or-nothing：python 先落缓存不能挡住后续波次下的 node/llamasrv/embedmodel 入缓存。
func (s *Server) saveRuntimeCache() {
	cache := runtimeCache()
	if cache == "" {
		return
	}
	src := filepath.Join(s.DataDir, "sandbox", "runtimes")
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	dst := filepath.Join(cache, "sandbox", "runtimes")
	_ = os.MkdirAll(dst, 0o755)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dst, e.Name())); err == nil {
			continue // this kind already cached. 该 kind 已有缓存。
		}
		_ = exec.Command("cp", "-R", filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())).Run()
	}
}

func (s *Server) waitHealthy(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(s.BaseURL + "/api/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("harness: backend never became healthy at %s", s.BaseURL)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("harness: free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Kill9 hard-kills the backend (SIGKILL — the crash in "crash recovery"). The data dir
// survives; pair with Restart to assert durable recovery.
//
// Kill9 硬杀 backend（SIGKILL——「崩溃恢复」里的那个崩溃）。数据目录幸存；与 Restart 配对
// 断言持久化恢复。
func (s *Server) Kill9(t *testing.T) {
	t.Helper()
	if err := s.cmd.Process.Kill(); err != nil {
		t.Fatalf("harness: kill -9: %v", err)
	}
	_, _ = s.cmd.Process.Wait()
}

// Restart boots a fresh process on the SAME data dir (new port) and waits for health —
// the recovery half of a crash test. The caller must re-derive clients (BaseURL changed).
//
// Restart 在**同一**数据目录上拉起新进程（新端口）并等 health——崩溃测试的恢复半场。
// 调用方需重取客户端（BaseURL 已变）。
func (s *Server) Restart(t *testing.T) {
	t.Helper()
	bin := binary(t)
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"FORGIFY_DATA_DIR="+s.DataDir,
		"FORGIFY_ADDR="+addr,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("harness: restart backend: %v", err)
	}
	s.cmd = cmd
	s.BaseURL = "http://" + addr
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	s.waitHealthy(t, 30*time.Second)
}
