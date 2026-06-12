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

// saveRuntimeCache copies downloaded runtimes back into the shared cache (first run pays,
// the rest ride).
//
// saveRuntimeCache 把已下载的运行时拷回共享缓存（首跑买单、后跑搭车）。
func (s *Server) saveRuntimeCache() {
	cache := runtimeCache()
	if cache == "" {
		return
	}
	src := filepath.Join(s.DataDir, "sandbox", "runtimes")
	if _, err := os.Stat(src); err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(cache, "sandbox", "runtimes")); err == nil {
		return // cache already populated. 缓存已就绪。
	}
	_ = os.MkdirAll(filepath.Join(cache, "sandbox"), 0o755)
	_ = exec.Command("cp", "-R", src, filepath.Join(cache, "sandbox", "runtimes")).Run()
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
