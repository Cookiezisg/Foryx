// preflight.go: Bootstrap orchestration. Extracts the bundled uv binary +
// python-build-standalone tarball from a resource directory into the data
// dir, runs platform-specific fixups (mac codesign), and verifies that both
// tools are runnable. After Bootstrap returns nil the sandbox is ready for
// Sync / Run.
//
// Resource directory layout (resourceDir is filled by cmd/server with
// $FORGIFY_DEV_RESOURCES in dev mode, or by cmd/desktop with a temp dir
// containing materials extracted from embed.FS in prod):
//
//	<resourceDir>/
//	├── uv-darwin-arm64        (or .exe on windows)
//	├── uv-darwin-amd64
//	├── uv-linux-amd64
//	├── uv-linux-arm64
//	├── uv-windows-amd64.exe
//	├── python-darwin-arm64.tar.gz
//	├── python-darwin-amd64.tar.gz
//	├── python-linux-amd64.tar.gz
//	├── python-linux-arm64.tar.gz
//	└── python-windows-amd64.tar.gz
//
// Bootstrap is idempotent: it hashes the source files and skips re-extraction
// when the existing data dir was set up from the same hashes. mac codesign /
// xattr fixups run on first install only — subsequent boots with same hash
// see them already applied.
//
// preflight.go：Bootstrap 编排。把捆绑 uv 二进制 + python-build-standalone
// tarball 从资源目录解压到数据目录，跑平台特定 fixup（mac codesign），
// 校验两个工具能跑。Bootstrap 返回 nil 后沙箱就绪，可调 Sync / Run。
//
// 资源目录布局见上方英文段。Bootstrap 幂等：按源文件 hash 跳过重解压；
// mac codesign / xattr fixup 仅首装时跑。

package sandbox

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"
)

// Bootstrap extracts uv + Python from resourceDir into the data dir, runs
// platform-specific fixups, and verifies both tools work. On success the
// Sandbox is marked ready and Sync / Run / Destroy / DestroyEnv become
// callable.
//
// Bootstrap is safe to call multiple times — when source hashes match the
// existing install it skips re-extraction. cmd/server should call it once at
// startup; subsequent calls (e.g. on resource refresh) cost only the
// validation runs.
//
// Bootstrap 把 uv + Python 从 resourceDir 解压到数据目录，跑平台 fixup，
// 校验两个工具。成功后 Sandbox 标记就绪，Sync / Run / Destroy / DestroyEnv
// 可调用。
//
// 多次调用安全——源 hash 不变跳过重解压。cmd/server 启动时调一次即可；
// 后续调用（如资源刷新）仅花校验时间。
func (s *Sandbox) Bootstrap(ctx context.Context, resourceDir string) error {
	if err := s.bootstrapInner(ctx, resourceDir); err != nil {
		s.bootstrapped = false
		return fmt.Errorf("sandbox.Bootstrap: %w", err)
	}
	s.bootstrapped = true
	s.log.Info("sandbox bootstrap complete",
		zap.String("data_dir", s.cfg.DataDir),
		zap.String("uv_path", s.UVPath()),
		zap.String("python_path", s.PythonPath()),
	)
	return nil
}

func (s *Sandbox) bootstrapInner(ctx context.Context, resourceDir string) error {
	// Validate resource dir exists.
	// 校验资源目录存在。
	info, err := os.Stat(resourceDir)
	if err != nil {
		return fmt.Errorf("resource dir %q: %w", resourceDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("resource dir %q is not a directory", resourceDir)
	}

	// Create data dir subdirs (idempotent).
	// 建数据目录子目录（幂等）。
	for _, sub := range []string{"bin", "forges", "uv-cache"} {
		if err := os.MkdirAll(filepath.Join(s.cfg.DataDir, sub), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}

	// Resolve platform-specific source paths.
	// 决定平台特定源文件路径。
	plat, err := platformKey()
	if err != nil {
		return err
	}
	srcUV := filepath.Join(resourceDir, "uv-"+plat)
	if runtime.GOOS == "windows" {
		srcUV += ".exe"
	}
	srcPython := filepath.Join(resourceDir, "python-"+plat+".tar.gz")

	// Idempotency check: hash both source files; skip extract if matching
	// hash already on disk.
	// 幂等检查：hash 两个源文件；磁盘上已有匹配 hash 则跳过解压。
	hash, err := computeBootstrapHash(srcUV, srcPython)
	if err != nil {
		return fmt.Errorf("hash sources: %w", err)
	}
	hashFile := filepath.Join(s.cfg.DataDir, "bin", ".bootstrap-hash")
	if existing, readErr := os.ReadFile(hashFile); readErr == nil && string(existing) == hash {
		s.log.Debug("sandbox.Bootstrap: source hashes unchanged, skipping extract")
	} else {
		// Copy uv binary.
		// 拷 uv 二进制。
		if err := copyExecutable(srcUV, s.UVPath()); err != nil {
			return fmt.Errorf("copy uv: %w", err)
		}

		// Wipe + extract python tarball. Wipe first so a botched previous
		// extract can't leave stale files.
		// 先清再解 python tarball，避免上次失败留旧文件。
		pyDir := filepath.Join(s.cfg.DataDir, "bin", "python")
		if err := os.RemoveAll(pyDir); err != nil {
			return fmt.Errorf("clear python dir: %w", err)
		}
		if err := extractTarGz(srcPython, pyDir); err != nil {
			return fmt.Errorf("extract python: %w", err)
		}

		// mac fixup: strip com.apple.provenance + ad-hoc codesign every
		// executable. Without this the kernel SIGKILLs python with no log
		// (issue uv#16726). Required for v0.x; v1.0+ notarization covers
		// these binaries via the .app's signing chain (sandbox iter doc §4.3).
		//
		// mac 修复：剥 com.apple.provenance + ad-hoc codesign 每个可执行
		// 文件。不修则内核 SIGKILL Python 无日志（issue uv#16726）。
		// v0.x 必需；v1.0+ 公证后由 .app 签名链覆盖。
		if runtime.GOOS == "darwin" {
			if err := macCodesign(ctx, pyDir, s.log); err != nil {
				return fmt.Errorf("mac codesign: %w", err)
			}
		}

		// Persist hash so next Bootstrap call can skip if unchanged.
		// 写 hash 让下次 Bootstrap 可跳过。
		if err := os.WriteFile(hashFile, []byte(hash), 0o644); err != nil {
			return fmt.Errorf("write hash file: %w", err)
		}
	}

	// Verify uv + Python work. Always run — even when extract was skipped,
	// a corrupt prior install would surface here.
	// 校验 uv + Python 能跑。即使跳过解压也跑——之前损坏的安装在此暴露。
	if err := verifyTool(ctx, s.UVPath(), "--version"); err != nil {
		return fmt.Errorf("verify uv: %w", err)
	}
	if err := verifyTool(ctx, s.PythonPath(), "-c", "import sys"); err != nil {
		return fmt.Errorf("verify python: %w", err)
	}

	return nil
}

// platformKey returns the resource-file-name suffix matching the current
// runtime: e.g. "darwin-arm64", "linux-amd64", "windows-amd64". Returns an
// error for unsupported OS / arch combos so we fail loud at startup rather
// than later with an opaque "file not found".
//
// platformKey 返回匹配运行时的资源文件名后缀，如 "darwin-arm64" /
// "linux-amd64" / "windows-amd64"。不支持的组合返错——启动期 fail loud
// 比之后报模糊"文件找不到"好。
func platformKey() (string, error) {
	var goos, goarch string
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		goos = runtime.GOOS
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
		goarch = runtime.GOARCH
	default:
		return "", fmt.Errorf("unsupported arch: %s", runtime.GOARCH)
	}
	return goos + "-" + goarch, nil
}

// computeBootstrapHash returns a stable hex digest of (uvSrc, pySrc)
// concatenated. Used as the cache key in <dataDir>/bin/.bootstrap-hash to
// decide whether to re-extract. Salted with a version marker so future
// scheme changes can force re-extract via a code bump.
//
// computeBootstrapHash 返回 (uvSrc, pySrc) 串接后的稳定 hex 摘要。
// 用作 <dataDir>/bin/.bootstrap-hash 缓存键决定是否重解压。前置 version
// marker——schema 变化时改它强制重解压。
func computeBootstrapHash(uvSrc, pySrc string) (string, error) {
	const versionMarker = "sandbox-bootstrap-v1\n"
	h := sha256.New()
	h.Write([]byte(versionMarker))
	for _, p := range []string{uvSrc, pySrc} {
		f, err := os.Open(p)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", p, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Errorf("hash %s: %w", p, err)
		}
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyExecutable copies src to dest with mode 0o755 (executable). Replaces
// any existing dest atomically via tmp + rename.
//
// copyExecutable 把 src 拷到 dest，mode 0o755（可执行）。
// 通过 tmp + rename 原子替换已存在的 dest。
func copyExecutable(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// extractTarGz extracts a tar.gz archive to destDir. Strips a leading
// "python/" path component because python-build-standalone always wraps its
// contents under that directory and our destDir is already named "python".
//
// Handles regular files, dirs, symlinks, and hard links. Other entry types
// (devices, fifos) are skipped — Python distributions never include them.
//
// Defends against zip-slip: any entry resolving outside destDir is rejected.
//
// extractTarGz 把 tar.gz 解压到 destDir。剥前导 "python/" 路径分量——
// python-build-standalone 总把内容包在那个目录下，而我们的 destDir 已经
// 叫 "python"。
//
// 处理普通文件、目录、符号链接、硬链接。其他类型（设备、fifo）跳过——
// Python 发行包从不含这些。
//
// 防 zip-slip：任何解析到 destDir 外的条目被拒。
func extractTarGz(srcPath, destDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}

		const prefix = "python/"
		name := strings.TrimPrefix(hdr.Name, prefix)
		if name == "" {
			continue
		}

		target := filepath.Join(destDir, name)
		// Defend against zip-slip: ensure target stays inside destDir.
		// 防 zip-slip：确保 target 留在 destDir 内。
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(absTarget, absDest+string(filepath.Separator)) && absTarget != absDest {
			return fmt.Errorf("tar entry %q escapes dest dir", hdr.Name)
		}

		mode := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target) // remove existing if any
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			linkSrc := filepath.Join(destDir, strings.TrimPrefix(hdr.Linkname, prefix))
			_ = os.Remove(target)
			if err := os.Link(linkSrc, target); err != nil {
				return err
			}
		default:
			// Skip device files, fifos, char devices, etc.
			// 跳过设备文件、fifo、字符设备等。
		}
	}
}

// macCodesign strips com.apple.provenance recursively and ad-hoc codesigns
// every executable file under root. Required to bypass macOS Gatekeeper's
// kernel-level SIGKILL on uv-installed Python (issue uv#16726). Step is a
// no-op on non-darwin and should not be called there.
//
// macCodesign 递归剥 com.apple.provenance + ad-hoc codesign root 下所有
// 可执行文件。绕开 macOS Gatekeeper 内核层 SIGKILL（issue uv#16726）。
// 非 darwin 是 no-op，不该调用。
func macCodesign(ctx context.Context, root string, log *zap.Logger) error {
	// 1. Strip com.apple.provenance recursively. Failure here is fatal —
	// without it, codesign alone may not clear the Gatekeeper cache for
	// every nested .dylib / .so.
	//
	// 1. 递归剥 com.apple.provenance。失败致命——不剥则 codesign 单步可能
	// 清不掉每个嵌套 .dylib / .so 的 Gatekeeper 缓存。
	cmd := exec.CommandContext(ctx, "xattr", "-dr", "com.apple.provenance", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xattr -dr: %w (output: %s)", err, out)
	}

	// 2. Walk all regular executable files and ad-hoc sign each. We must
	// sign every Mach-O loaded by the interpreter (libpython.dylib, lots of
	// stdlib .so) — relying on a single sign of python3 is not enough
	// because dlopen rechecks each library.
	//
	// 2. 遍历所有正则可执行文件 ad-hoc 签。每个解释器加载的 Mach-O 都要签
	// （libpython.dylib + 一堆 stdlib .so）——只签 python3 不够，dlopen
	// 重新校验每个库。
	signed := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Only regular files with at least one execute bit.
		// 仅普通文件且至少一个执行位。
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return nil
		}
		signCmd := exec.CommandContext(ctx, "codesign", "--force", "--sign", "-", path)
		if out, signErr := signCmd.CombinedOutput(); signErr != nil {
			return fmt.Errorf("codesign %s: %w (output: %s)", path, signErr, out)
		}
		signed++
		return nil
	})
	if err != nil {
		return err
	}
	log.Info("mac codesign complete", zap.String("root", root), zap.Int("signed_files", signed))
	return nil
}

// verifyTool runs the tool with given args and returns nil on exit code 0.
// Any non-zero exit or exec error wraps the combined output for diagnostics.
//
// verifyTool 用给定参数跑工具，exit 0 返 nil。非零 exit 或 exec 错误时把
// 合并输出包进 error 供诊断。
func verifyTool(ctx context.Context, toolPath string, args ...string) error {
	cmd := exec.CommandContext(ctx, toolPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w (output: %s)", toolPath, args, err, out)
	}
	return nil
}
