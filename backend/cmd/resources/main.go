// Command resources downloads jdx/mise release binaries into the source
// tree at backend/internal/infra/sandbox/mise/<goos>-<goarch>/mise[.exe],
// where D2-2's per-platform go:embed directives pick them up at compile time.
//
// Default mode fetches just the current platform's binary (fast — what
// developers run locally). Pass --all-platforms to fetch all 5 supported
// platforms; this is what release builds invoke before cross-compiling
// per-platform binaries.
//
// Layout under the source tree (the per-platform sub-dirs match D2-2's
// embed pattern; .gitignore at mise/ keeps binaries out of git). Paths
// below are relative to the backend module root — the command must run
// from there (Makefile + devbox bootstrap both `cd backend` first):
//
//	internal/infra/sandbox/mise/.gitignore
//	internal/infra/sandbox/mise/darwin-arm64/mise
//	internal/infra/sandbox/mise/darwin-amd64/mise
//	internal/infra/sandbox/mise/linux-amd64/mise
//	internal/infra/sandbox/mise/linux-arm64/mise
//	internal/infra/sandbox/mise/windows-amd64/mise.exe
//
// Pin version via MISE_VERSION env (defaults to "latest", resolved through
// the GitHub releases API). The fetcher trusts mise's official SHA256SUMS
// asset and aborts on hash mismatch.
//
// Note: this command replaced the v1 uv + python-build-standalone fetcher
// (which targeted ~/.forgify-dev-resources). PluginSandbox v2 has mise
// install python + uv lazily on first use, so the v1 dev resources
// directory is no longer consumed. Forge sandbox v1 will return
// ErrSandboxUnavailable until D2-5 migrates forge to the v2 service —
// short-lived gap during the D2 sub-task chain.
//
// Command resources 把 jdx/mise release 二进制下到源码树
// backend/internal/infra/sandbox/mise/<goos>-<goarch>/mise[.exe]，
// 由 D2-2 的 per-platform go:embed 编译期取走。
//
// 默认拉当前平台（开发本地快），加 --all-platforms 拉全 5 平台
// （release pipeline 跨平台编译前用）。版本 pin via MISE_VERSION env，
// 默认 "latest" 走 GitHub releases API 解析。fetcher 校验 mise 官方
// SHA256SUMS asset，hash 不匹配立即 abort。
//
// 注：本命令替换了 v1 的 uv + python-build-standalone fetcher（原向
// ~/.forgify-dev-resources/）。PluginSandbox v2 改由 mise 在首次使用时
// lazy install python + uv，v1 dev resources 目录不再被消费。Forge sandbox v1
// 将返 ErrSandboxUnavailable 直到 D2-5 把 forge 迁到 v2 service——D2 子任务
// 链中的短暂过渡期。
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// platform encodes one supported (GOOS, GOARCH) tuple plus the upstream
// asset naming mise uses (macos≠darwin, x64≠amd64) and the archive
// extension (zip on windows, tar.gz elsewhere).
//
// platform 编一个支持的 (GOOS, GOARCH) tuple + mise 上游 asset 命名
// （macos≠darwin、x64≠amd64）+ 归档格式（windows .zip，其余 .tar.gz）。
type platform struct {
	goos    string // Go GOOS — used for output sub-dir naming
	goarch  string // Go GOARCH — used for output sub-dir naming
	miseOS  string // mise asset OS name: "linux" / "macos" / "windows"
	miseArc string // mise asset arch name: "x64" / "arm64"
	archExt string // archive format: ".tar.gz" or ".zip"
	binName string // binary name inside archive: "mise" or "mise.exe"
}

func (p platform) key() string { return p.goos + "-" + p.goarch }
func (p platform) outDir() string {
	// Path relative to backend module root (Makefile + devbox bootstrap
	// both `cd backend` before invoking us, so the cwd is backend/).
	// 路径相对 backend module 根（Makefile + devbox bootstrap 都先 cd backend）。
	return filepath.Join("internal", "infra", "sandbox", "mise", p.key())
}
func (p platform) outBin() string { return filepath.Join(p.outDir(), p.binName) }

var supported = []platform{
	{"darwin", "arm64", "macos", "arm64", ".tar.gz", "mise"},
	{"darwin", "amd64", "macos", "x64", ".tar.gz", "mise"},
	{"linux", "amd64", "linux", "x64", ".tar.gz", "mise"},
	{"linux", "arm64", "linux", "arm64", ".tar.gz", "mise"},
	{"windows", "amd64", "windows", "x64", ".zip", "mise.exe"},
}

func main() {
	var (
		allPlatforms = flag.Bool("all-platforms", false, "fetch binaries for all 5 supported platforms (release pipeline)")
		force        = flag.Bool("force", false, "redownload even when output file already exists")
	)
	flag.Parse()

	version := os.Getenv("MISE_VERSION")
	if version == "" {
		fmt.Println("→ resolving latest mise release ...")
		version = mustLatestTag("jdx/mise")
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	targets := []platform{currentPlatform()}
	if *allPlatforms {
		targets = supported
	}

	for _, p := range targets {
		fmt.Printf("\n=== %s (mise %s/%s, %s) ===\n", p.key(), p.miseOS, p.miseArc, p.archExt)
		out := p.outBin()
		if !*force && fileExists(out) {
			fmt.Printf("✓ already present: %s\n", out)
			continue
		}
		if err := os.MkdirAll(p.outDir(), 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", p.outDir(), err)
		}
		if err := fetchOne(version, p); err != nil {
			log.Fatalf("%s: %v", p.key(), err)
		}
		fmt.Printf("✓ wrote %s\n", out)
	}

	fmt.Printf("\n✓ done. Embed layout ready under backend/internal/infra/sandbox/mise/\n")
	if !*allPlatforms {
		fmt.Printf("  (current platform only; pass --all-platforms for the full release set)\n")
	}
}

// currentPlatform returns the supported entry matching runtime.GOOS/GOARCH or
// dies if the host platform isn't in our v1 matrix.
//
// currentPlatform 返回匹配 runtime.GOOS/GOARCH 的支持项；不在 v1 矩阵则 fatal。
func currentPlatform() platform {
	for _, p := range supported {
		if p.goos == runtime.GOOS && p.goarch == runtime.GOARCH {
			return p
		}
	}
	log.Fatalf("unsupported host platform %s/%s; mise embed only ships %d targets",
		runtime.GOOS, runtime.GOARCH, len(supported))
	return platform{}
}

// fetchOne downloads + verifies + extracts the mise binary for one platform.
//
// fetchOne 下载 + 校验 + 解压一份 mise 二进制（单平台）。
func fetchOne(version string, p platform) error {
	assetName := fmt.Sprintf("mise-%s-%s-%s%s", version, p.miseOS, p.miseArc, p.archExt)
	url := fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/%s", version, assetName)

	fmt.Printf("→ download %s\n", url)
	body, err := httpGetBytes(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// SHA256 verification — mise publishes SHASUMS256.txt next to assets.
	// Match the line `<hex>  <assetName>` then compare to local hash.
	//
	// SHA256 校验——mise 在 release 旁边发布 SHASUMS256.txt。匹配
	// `<hex>  <assetName>` 行后比本地 hash。
	sumsURL := fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/SHASUMS256.txt", version)
	sums, err := httpGetBytes(sumsURL)
	if err != nil {
		return fmt.Errorf("download SHASUMS256.txt: %w", err)
	}
	want, err := lookupSum(sums, assetName)
	if err != nil {
		return fmt.Errorf("checksum lookup: %w", err)
	}
	gotSum := sha256.Sum256(body)
	got := hex.EncodeToString(gotSum[:])
	if got != want {
		return fmt.Errorf("sha256 mismatch: want %s got %s", want, got)
	}
	fmt.Printf("✓ sha256 ok\n")

	if p.archExt == ".tar.gz" {
		return extractTarGz(body, p.binName, p.outBin())
	}
	return extractZip(body, p.binName, p.outBin())
}

// extractTarGz finds binName inside a tar.gz blob and writes it to dst with
// 0755. mise's tarball layout puts the binary at "mise/bin/mise" — we
// match by Base name to stay resilient to layout tweaks.
//
// extractTarGz 从 tar.gz 找到 binName 写到 dst，权限 0755。mise tarball
// 把二进制放在 "mise/bin/mise"——按 Base 名匹配以抗布局微调。
func extractTarGz(blob []byte, binName, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s not found in tarball", binName)
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName {
			continue
		}
		return writeBinary(tr, dst)
	}
}

// extractZip finds binName inside a zip blob and writes it to dst. Used for
// the windows-amd64 asset.
//
// extractZip 从 zip blob 找 binName 写到 dst。windows-amd64 asset 用。
func extractZip(blob []byte, binName, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != binName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}
		err = writeBinary(rc, dst)
		rc.Close()
		return err
	}
	return fmt.Errorf("%s not found in zip", binName)
}

// writeBinary streams r into dst with 0755 permission. Uses tmp+rename for
// atomicity so partial downloads never leave a half-written binary.
//
// writeBinary 把 r 流到 dst，权限 0755。tmp+rename 原子写避免半成品。
func writeBinary(r io.Reader, dst string) error {
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		// Best-effort cleanup of half-written tmp; copy already failed
		// so caller will surface that. A Remove failure here (e.g. file
		// got renamed by concurrent run, permission flipped) is not
		// actionable for the build script. §S3 例外。
		//
		// 半成 tmp 尽力清理；copy 已失败上抛。Remove 失败（被并发运行
		// 改名 / 权限翻转）无可执行动作。§S3 例外。
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	return os.Rename(tmp, dst)
}

// lookupSum scans a SHASUMS256.txt blob for the line whose 2nd column matches
// assetName and returns the hex digest from the 1st column. mise's format
// is `<hex>  ./<name>` — names are prefixed with `./`, so we strip it
// before comparing.
//
// lookupSum 扫 SHASUMS256.txt blob 找第 2 列匹配 assetName 的行，返第 1 列的
// hex digest。mise 格式 `<hex>  ./<name>`——文件名带 `./` 前缀，比较前剥掉。
func lookupSum(sums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "./") == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no entry for %s in SHASUMS256.txt", assetName)
}

// httpGetBytes GETs url and returns the full body, or an error on non-2xx /
// network failure. Body capped at 100 MB defensive against accidental
// content-type surprises (mise binary is ~25 MB).
//
// httpGetBytes 拉 url 返完整 body，非 2xx / 网络失败返错。100 MB 上限防意外
// content-type 巨型响应（mise 二进制 ~25 MB）。
func httpGetBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("get %s: status %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20))
}

// mustLatestTag returns the latest tag for owner/repo via GitHub API; fatal
// on failure.
//
// mustLatestTag 通过 GitHub API 返 owner/repo 最新 tag；失败 fatal。
func mustLatestTag(repo string) string {
	body, err := httpGetBytes("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		log.Fatalf("latest tag for %s: %v", repo, err)
	}
	var v struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		log.Fatalf("decode latest tag for %s: %v", repo, err)
	}
	if v.TagName == "" {
		log.Fatalf("empty tag_name from %s latest release", repo)
	}
	return v.TagName
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
