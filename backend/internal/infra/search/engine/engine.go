// Package engine hosts the two EmbeddingProvider adapters behind the search
// semantic layer (§domains/search.md):
//
//   - Builtin — the AnythingLLM-style zero-config default: directInstaller
//     fetches the pinned llama-server binary + EmbeddingGemma GGUF on first
//     demand (via the sandbox Ensure port), then a resident subprocess serves
//     OpenAI-compatible /v1/embeddings on 127.0.0.1. Same resident-process
//     family as handlers and MCP servers.
//   - Ollama — for users who already run Ollama and want to reuse its models.
//
// Both fail soft: any error degrades search to pure lexical, never breaks it.
//
// Package engine 承载搜索语义层的两个 EmbeddingProvider 适配器（§domains/search.md）：
// Builtin——AnythingLLM 式零配置默认：directInstaller 首用经 sandbox Ensure 端口拉
// 钉死的 llama-server 二进制 + EmbeddingGemma GGUF，常驻子进程在 127.0.0.1 上出
// OpenAI 兼容 /v1/embeddings，与 handler/MCP 同族常驻进程；Ollama——给已跑 Ollama
// 的用户复用其模型库。两者皆失败软化：任何错误把检索降为纯词法，绝不弄坏它。
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// Pinned identities — bumping = edit recipe + these two lines.
	// 钉死身份——升级 = 改 recipe + 这两行。
	llamasrvVersion    = "b9601"
	modelVersion       = "embeddinggemma-300m-qat-q8_0"
	spawnHealthTimeout = 90 * time.Second // model load on a slow disk takes a while. 慢盘上模型加载要点时间。
	embedTimeout       = 60 * time.Second
)

// Status values surfaced through GET /api/v1/search/settings.
//
// 经 GET /api/v1/search/settings 暴露的状态值。
const (
	StatusAbsent      = "absent"
	StatusDownloading = "downloading"
	StatusReady       = "ready"
	StatusError       = "error"
)

// Ensurer is the sandbox slice the builtin engine needs: resolve (kind,
// version) to an on-disk path, installing on first demand (directInstaller).
//
// Ensurer 是 builtin 引擎所需的 sandbox 切面：把 (kind, version) 解析为盘上路径，
// 首用即装（directInstaller）。
type Ensurer interface {
	EnsureTool(ctx context.Context, kind, version string) (string, error)
}

// Builtin is the default embedder: lazy install, lazy spawn, crash-respawn,
// graceful close.
//
// Builtin 是默认 embedder：惰性安装、惰性 spawn、crash 重拉、优雅关停。
type Builtin struct {
	ensure Ensurer
	log    *zap.Logger

	mu      sync.Mutex // serializes install+spawn — held across the whole first-demand download. 串行化安装+spawn——首用下载全程持有。
	cmd     *exec.Cmd
	baseURL string // set in tests to bypass install+spawn. 测试注入以绕过安装+spawn。

	// stmu is a leaf lock for the status snapshot: GET /search/settings polls Status()
	// while an install is in flight — it must report "downloading" instantly, never
	// queue behind mu for the duration of a model download.
	//
	// stmu 是状态快照的叶子锁：安装进行中 GET /search/settings 轮询 Status()——必须立刻
	// 报 "downloading"，绝不能在 mu 后面排队等完一次模型下载。
	stmu    sync.Mutex
	status  string
	lastErr string

	client *http.Client
}

// NewBuiltin constructs the builtin engine (no network, no process yet).
//
// NewBuiltin 构造 builtin 引擎（此刻无网络、无进程）。
func NewBuiltin(ensure Ensurer, log *zap.Logger) *Builtin {
	if log == nil {
		log = zap.NewNop()
	}
	return &Builtin{ensure: ensure, log: log.Named("search.engine"), status: StatusAbsent, client: &http.Client{Timeout: embedTimeout}}
}

// NewBuiltinForTest wires a fake /v1/embeddings server in place of the real
// install+spawn chain.
//
// NewBuiltinForTest 用假 /v1/embeddings server 替代真实安装+spawn 链。
func NewBuiltinForTest(baseURL string) *Builtin {
	return &Builtin{baseURL: baseURL, status: StatusReady, log: zap.NewNop(), client: &http.Client{Timeout: embedTimeout}}
}

// Model identifies stored vectors (switching embedders invalidates, never
// mixes).
//
// Model 标识存量向量（换 embedder 即失效，绝不混用）。
func (b *Builtin) Model() string { return modelVersion }

// Status reports the engine lifecycle for the settings surface. Reads the leaf lock only,
// so it stays instant while ensureRunning holds mu through a download.
//
// Status 报告引擎生命周期给 settings 面。只读叶子锁——ensureRunning 持 mu 下载期间它仍秒回。
func (b *Builtin) Status() (status, lastErr string) {
	b.stmu.Lock()
	defer b.stmu.Unlock()
	return b.status, b.lastErr
}

// setStatus updates the status snapshot under the leaf lock.
//
// setStatus 在叶子锁下更新状态快照。
func (b *Builtin) setStatus(status, lastErr string) {
	b.stmu.Lock()
	b.status = status
	b.lastErr = lastErr
	b.stmu.Unlock()
}

// Embed ensures the engine is installed + running, then embeds texts (batch
// cap applied by the caller). Every failure path records status and returns an
// error — the caller degrades, the next call retries.
//
// Embed 确保引擎已装+在跑，然后嵌入 texts（批上限由调用方控制）。每条失败路径记录
// status 并返错——调用方降级，下次调用重试。
func (b *Builtin) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	base, err := b.ensureRunning(ctx)
	if err != nil {
		return nil, err
	}
	return embedHTTP(ctx, b.client, base+"/v1/embeddings", "", modelVersion, texts)
}

// ensureRunning installs (download on first demand) and spawns the resident
// llama-server, serializing concurrent callers.
//
// ensureRunning 安装（首用下载）并 spawn 常驻 llama-server，串行化并发调用方。
func (b *Builtin) ensureRunning(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.baseURL != "" && b.processAlive() {
		return b.baseURL, nil
	}
	if b.baseURL != "" && b.cmd == nil {
		return b.baseURL, nil // test mode: injected URL, no process. 测试态：注入 URL、无进程。
	}

	b.setStatus(StatusDownloading, "")
	bin, err := b.ensure.EnsureTool(ctx, "llamasrv", llamasrvVersion)
	if err != nil {
		b.fail("install llama-server: %v", err)
		return "", fmt.Errorf("search engine: install llama-server: %w", err)
	}
	model, err := b.ensure.EnsureTool(ctx, "embedmodel", modelVersion)
	if err != nil {
		b.fail("download embedding model: %v", err)
		return "", fmt.Errorf("search engine: download model: %w", err)
	}

	port, err := freePort()
	if err != nil {
		b.fail("no free port: %v", err)
		return "", fmt.Errorf("search engine: free port: %w", err)
	}
	// Detached from the request ctx: the resident process outlives any caller.
	// 脱离请求 ctx：常驻进程活得比任何调用方长。
	cmd := exec.Command(bin,
		"--embeddings", "-m", model,
		"--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"--ctx-size", "2048", "--threads", "4", "--log-disable")
	if err := cmd.Start(); err != nil {
		b.fail("spawn llama-server: %v", err)
		return "", fmt.Errorf("search engine: spawn: %w", err)
	}
	go func() { _ = cmd.Wait() }() // reap; liveness checked via ProcessState. 收尸；存活经 ProcessState 判断。

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	if err := waitHealthy(ctx, b.client, base, spawnHealthTimeout); err != nil {
		_ = cmd.Process.Kill()
		b.fail("llama-server never became healthy: %v", err)
		return "", fmt.Errorf("search engine: health: %w", err)
	}
	b.cmd = cmd
	b.baseURL = base
	b.setStatus(StatusReady, "")
	b.log.Info("search engine ready", zap.String("addr", base), zap.String("model", modelVersion))
	return base, nil
}

func (b *Builtin) processAlive() bool {
	return b.cmd != nil && b.cmd.ProcessState == nil
}

func (b *Builtin) fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	b.setStatus(StatusError, msg)
	b.log.Warn("search engine: " + msg)
}

// Close stops the resident process (app shutdown). Off ≠ uninstall: files stay,
// re-enabling is instant.
//
// Close 停常驻进程（app 关停）。off ≠ 卸载：文件保留，再启秒回。
func (b *Builtin) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil && b.cmd.ProcessState == nil {
		_ = b.cmd.Process.Kill()
	}
	b.cmd = nil
	b.stmu.Lock()
	if b.status == StatusReady {
		b.status = StatusAbsent
	}
	b.stmu.Unlock()
}

// Ollama adapts a local Ollama daemon (/api/embed) for users reusing its
// model library.
//
// Ollama 适配本机 Ollama 守护（/api/embed），复用其模型库。
type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllama constructs the adapter; empty args take the conventional defaults.
//
// NewOllama 构造适配器；空参取惯例默认。
func NewOllama(baseURL, model string) *Ollama {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	if model == "" {
		model = "embeddinggemma"
	}
	return &Ollama{baseURL: baseURL, model: model, client: &http.Client{Timeout: embedTimeout}}
}

func (o *Ollama) Model() string { return "ollama:" + o.model }

func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": o.model, "input": texts})
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama embed: %s: %s", resp.Status, msg)
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama embed: decode: %w", err)
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d texts", len(out.Embeddings), len(texts))
	}
	return out.Embeddings, nil
}

// embedHTTP calls an OpenAI-compatible /v1/embeddings endpoint.
//
// embedHTTP 调 OpenAI 兼容 /v1/embeddings 端点。
func embedHTTP(ctx context.Context, client *http.Client, url, bearer, model string, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": model, "input": texts})
	if err != nil {
		return nil, fmt.Errorf("embed: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embed: %s: %s", resp.Status, msg)
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embed: decode: %w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embed: got %d vectors for %d texts", len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

func waitHealthy(ctx context.Context, client *http.Client, base string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return fmt.Errorf("health timeout after %s", timeout)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
