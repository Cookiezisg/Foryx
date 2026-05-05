// installer_static.go — RuntimeInstaller for plugins shipped as a single
// pre-built static binary (e.g. GitHub MCP is a Go binary with no runtime
// deps; some MCP servers are Rust binaries; future Forgify-internal tools
// might ship the same way).
//
// Behaviour: HTTP GET the URL, write to disk under
// <sandboxRoot>/static-binaries/<kind>/<filename>, chmod 0755, run
// macCodesign on darwin to defang Gatekeeper. SHA256 verification is
// optional (passed via the version string in the form
// "sha256:<hex>@<url>"); if no checksum is supplied, install proceeds
// unverified (caller's choice).
//
// installer_static.go ——给以单个预构建静态二进制发布的 plugin 用
// （如 GitHub MCP 是 Go binary 无 runtime 依赖；某些 MCP server 是 Rust
// binary；未来 Forgify 内部 tool 也可能这样发）。
//
// 行为：HTTP GET URL，写到 <sandboxRoot>/static-binaries/<kind>/<filename>，
// chmod 0755，darwin 上跑 macCodesign 解 Gatekeeper。SHA256 校验可选
// （通过 version 字符串以 "sha256:<hex>@<url>" 形式传）；不带 checksum
// 则不校验进行 install（调用方选择）。

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// staticBinariesSubdir holds all StaticBinaryInstaller downloads; one
// sub-dir per kind so multiple static plugins can coexist.
//
// staticBinariesSubdir 收所有 StaticBinaryInstaller 下载；每 kind 一个
// 子目录让多个 static plugin 共存。
const staticBinariesSubdir = "static-binaries"

// StaticBinaryInstaller satisfies sandboxdomain.RuntimeInstaller for any
// pre-built binary downloadable over HTTP. The Kind() name maps 1:1 to a
// plugin family ("github-mcp", "filesystem-mcp", etc.); a single
// installer instance per family.
//
// StaticBinaryInstaller 满足 sandboxdomain.RuntimeInstaller 给任何 HTTP 可下
// 的预构建二进制。Kind() 名 1:1 对应 plugin family（"github-mcp" /
// "filesystem-mcp" 等）；每 family 一个 installer 实例。
type StaticBinaryInstaller struct {
	kind string
	log  *zap.Logger
}

// NewStaticBinaryInstaller constructs an installer for the given kind tag.
// Logger is required (panics on nil) — codesigning + download progress
// rely on it.
//
// NewStaticBinaryInstaller 构造给定 kind tag 的 installer。Logger 必填
// （nil panic）——codesign + 下载进度需要它。
func NewStaticBinaryInstaller(kind string, log *zap.Logger) *StaticBinaryInstaller {
	if log == nil {
		panic("sandbox.NewStaticBinaryInstaller: nil logger")
	}
	return &StaticBinaryInstaller{kind: kind, log: log}
}

// Kind reports the dispatch tag baked at construction.
//
// Kind 报告构造时固化的派发 tag。
func (s *StaticBinaryInstaller) Kind() string { return s.kind }

// Install downloads the binary specified by the version string. version
// formats accepted:
//
//	"https://example.com/path/to/binary"
//	"sha256:<64hex>@https://example.com/path/to/binary"
//
// The first form skips checksum verification; the second hashes the
// download and aborts on mismatch. Returns the binary's path relative to
// sandboxRoot.
//
// Install 下载 version 字符串指定的二进制。version 格式：
//
//	"https://example.com/path/to/binary"
//	"sha256:<64hex>@https://example.com/path/to/binary"
//
// 第一种跳过校验；第二种 hash 下载内容，不匹配则 abort。返二进制相对
// sandboxRoot 的路径。
func (s *StaticBinaryInstaller) Install(ctx context.Context, version, sandboxRoot string, stream sandboxdomain.ProgressFunc) (string, error) {
	url, wantHash, err := parseStaticVersion(version)
	if err != nil {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: %w", err)
	}

	binDir := filepath.Join(sandboxRoot, staticBinariesSubdir, s.kind)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: mkdir: %w", err)
	}

	// Derive filename from URL path. e.g. "/releases/v1/github-mcp" → "github-mcp".
	// 从 URL path 派生文件名。
	filename := path.Base(url)
	if filename == "" || filename == "." || filename == "/" {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: cannot derive filename from %q", url)
	}
	binPath := filepath.Join(binDir, filename)

	if stream != nil {
		stream("downloading", "GET "+url, -1)
	}

	body, err := httpGetBytesStatic(ctx, url)
	if err != nil {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: download: %w (runtime: %w)", err, sandboxdomain.ErrRuntimeInstallFailed)
	}

	if wantHash != "" {
		gotSum := sha256.Sum256(body)
		got := hex.EncodeToString(gotSum[:])
		if got != wantHash {
			return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: sha256 mismatch want %s got %s: %w",
				wantHash, got, sandboxdomain.ErrRuntimeInstallFailed)
		}
	}

	// Atomic write tmp+rename then chmod.
	// 原子写 tmp+rename + chmod。
	tmp := binPath + ".tmp"
	if err := os.WriteFile(tmp, body, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: write: %w", err)
	}
	if err := os.Rename(tmp, binPath); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: rename: %w", err)
	}

	// darwin: ad-hoc codesign so Gatekeeper doesn't SIGKILL on first exec.
	// Same approach as bootstrap_mise.go ExtractMiseBinary.
	//
	// darwin: ad-hoc codesign 让 Gatekeeper 首次 exec 不 SIGKILL。
	// 与 bootstrap_mise.go ExtractMiseBinary 同套路。
	if runtime.GOOS == "darwin" {
		if err := macCodesign(ctx, binPath, s.log); err != nil {
			return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: codesign: %w", err)
		}
	}

	rel, err := filepath.Rel(sandboxRoot, binPath)
	if err != nil {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Install: rel path: %w", err)
	}
	return rel, nil
}

// Locate reads the (kind, version) install dir from sandboxRoot and
// returns the absolute path to the binary inside. Caller is expected to
// have called Install first; if the binary is missing we still return
// the predicted path (caller's stat call surfaces the absence).
//
// Locate 从 sandboxRoot 读 (kind, version) install 目录返内部 binary 绝对
// 路径。调用方应先调 Install；binary 缺失时仍返预测路径（调用方 stat
// 暴露缺失）。
func (s *StaticBinaryInstaller) Locate(version, sandboxRoot string) (string, error) {
	url, _, err := parseStaticVersion(version)
	if err != nil {
		return "", fmt.Errorf("sandbox.StaticBinaryInstaller.Locate: %w", err)
	}
	filename := path.Base(url)
	return filepath.Join(sandboxRoot, staticBinariesSubdir, s.kind, filename), nil
}

// ListAvailable returns nil — static binary installers don't enumerate
// versions (each binary is a one-off URL).
//
// ListAvailable 返 nil——static binary installer 不枚举版本（每 binary 一个
// 一次性 URL）。
func (s *StaticBinaryInstaller) ListAvailable(ctx context.Context) ([]string, error) {
	return nil, nil
}

// ResolveDefault returns "" — no default version concept for static
// binaries; caller must always pass an explicit URL via RuntimeSpec.Version.
//
// ResolveDefault 返 ""——static binary 无默认版本概念；调用方必须经
// RuntimeSpec.Version 显式传 URL。
func (s *StaticBinaryInstaller) ResolveDefault(ctx context.Context) (string, error) {
	return "", nil
}

// parseStaticVersion accepts either "<url>" or "sha256:<64hex>@<url>" and
// splits into (url, wantHash). Empty wantHash means "no verification".
//
// parseStaticVersion 接 "<url>" 或 "sha256:<64hex>@<url>" 拆成 (url, wantHash)。
// wantHash 为空表示"不校验"。
func parseStaticVersion(version string) (url, wantHash string, err error) {
	if version == "" {
		return "", "", fmt.Errorf("empty version (URL or sha256:<hex>@<URL> required)")
	}
	if !strings.HasPrefix(version, "sha256:") {
		return version, "", nil
	}
	rest := strings.TrimPrefix(version, "sha256:")
	at := strings.Index(rest, "@")
	if at < 0 {
		return "", "", fmt.Errorf("malformed sha256 version: missing '@' separator")
	}
	hash := rest[:at]
	url = rest[at+1:]
	if len(hash) != 64 {
		return "", "", fmt.Errorf("malformed sha256 hash: want 64 hex chars, got %d", len(hash))
	}
	if url == "" {
		return "", "", fmt.Errorf("malformed sha256 version: empty URL")
	}
	return url, hash, nil
}

// httpGetBytesStatic GETs url with the given context. Body capped at
// 200 MB for sanity (static plugin binaries are typically <50 MB).
//
// httpGetBytesStatic 用给定 ctx 拉 url。body 上限 200 MB
// （static plugin 二进制通常 <50 MB）。
func httpGetBytesStatic(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("get %s: status %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}
