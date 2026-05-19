// App holds the Wails app state and the embedded backend subprocess.
//
// App 持有 Wails app 状态以及后端子进程。
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// App is bound to Wails. Methods on App become callable from the frontend
// via window.go.main.App.MethodName(). Keep the surface minimal — only
// GetBackendPort is needed; everything else goes through HTTP.
//
// App 绑定到 Wails。其方法对前端经 window.go.main.App.X() 可调用。
// 暴露面保持最小——只有 GetBackendPort，其他一律走 HTTP。
type App struct {
	mu       sync.RWMutex
	port     int
	backend  *exec.Cmd
	ctx      context.Context
	readyCh  chan int // closed once port is known
}

func NewApp() *App {
	return &App{readyCh: make(chan int, 1)}
}

// startup runs on Wails app launch. It spawns the backend, blocks until
// the BACKEND_PORT= line is read, then continues. If the backend dies the
// channel is closed with port=0 and GetBackendPort returns an error.
//
// startup 在 Wails 启动时调用。拉起后端子进程，阻塞到读到
// BACKEND_PORT= 行；后端死掉则 channel 关闭返 port=0。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	bin, err := locateBackendBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "desktop: backend binary not found: %v\n", err)
		return
	}

	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".forgify")

	cmd := exec.Command(bin, "-port=0", "-data-dir="+dataDir)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "desktop: stdout pipe: %v\n", err)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "desktop: backend start: %v\n", err)
		return
	}

	a.backend = cmd

	go func() {
		scanner := bufio.NewScanner(stdout)
		seen := false
		for scanner.Scan() {
			line := scanner.Text()
			// Mirror backend stdout so we don't lose logs.
			fmt.Fprintln(os.Stdout, line)
			if !seen && strings.HasPrefix(line, "BACKEND_PORT=") {
				p, err := strconv.Atoi(strings.TrimPrefix(line, "BACKEND_PORT="))
				if err != nil {
					continue
				}
				a.mu.Lock()
				a.port = p
				a.mu.Unlock()
				a.readyCh <- p
				close(a.readyCh)
				seen = true
			}
		}
		if !seen {
			// Pipe closed before we ever saw the port: signal failure.
			close(a.readyCh)
		}
	}()

	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "desktop: backend exited: %v\n", err)
		}
	}()
}

// shutdown kills the backend cleanly on app exit.
func (a *App) shutdown(_ context.Context) {
	a.mu.RLock()
	cmd := a.backend
	a.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
	}
}

// GetBackendPort returns the HTTP port the backend is listening on.
// Blocks up to 10s while the backend boots; surfaces an error if the
// subprocess died without printing BACKEND_PORT.
//
// GetBackendPort 返回后端监听端口；最长阻塞 10s 等后端启动；
// 子进程未打印端口就死则返错误。
func (a *App) GetBackendPort() (int, error) {
	a.mu.RLock()
	if a.port != 0 {
		p := a.port
		a.mu.RUnlock()
		return p, nil
	}
	a.mu.RUnlock()

	select {
	case p, ok := <-a.readyCh:
		if !ok || p == 0 {
			return 0, errors.New("backend failed to start (no BACKEND_PORT line)")
		}
		return p, nil
	case <-time.After(10 * time.Second):
		return 0, errors.New("backend boot timed out after 10s")
	}
}

// locateBackendBinary resolves the cmd/server binary. In dev we look in
// $PATH-style locations adjacent to the desktop binary; in packaged apps
// the binary is bundled in Contents/MacOS/.
//
// locateBackendBinary 找到 cmd/server 二进制；dev 时在与桌面 binary 同目录
// 找，打包后在 Contents/MacOS/ 下。
func locateBackendBinary() (string, error) {
	if v := os.Getenv("FORGIFY_BACKEND_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v, nil
		}
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, name := range []string{"forgify-server", "server"} {
			cand := filepath.Join(dir, name)
			if _, err := os.Stat(cand); err == nil {
				return cand, nil
			}
		}
	}
	// Last resort: rely on PATH lookup.
	return exec.LookPath("forgify-server")
}
