// Command resources fetches jdx/mise binaries into backend/internal/infra/sandbox/mise/<goos>-<goarch>/ for go:embed.
//
// Command resources 下载 jdx/mise 二进制到 sandbox/mise 目录供 go:embed 取走。
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

// platform pairs a Go GOOS/GOARCH with mise's upstream asset naming and archive format.
//
// platform 把 Go GOOS/GOARCH 配上 mise 上游 asset 命名与归档格式。
type platform struct {
	goos    string
	goarch  string
	miseOS  string
	miseArc string
	archExt string
	binName string
}

func (p platform) key() string { return p.goos + "-" + p.goarch }
func (p platform) outDir() string {
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

// currentPlatform returns the supported entry matching runtime.GOOS/GOARCH or fatals.
//
// currentPlatform 返回匹配 runtime.GOOS/GOARCH 的项，无匹配则 fatal。
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

// fetchOne downloads, verifies SHA256, and extracts the mise binary for one platform.
//
// fetchOne 下载 + SHA256 校验 + 解压一份 mise 二进制。
func fetchOne(version string, p platform) error {
	assetName := fmt.Sprintf("mise-%s-%s-%s%s", version, p.miseOS, p.miseArc, p.archExt)
	url := fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/%s", version, assetName)

	fmt.Printf("→ download %s\n", url)
	body, err := httpGetBytes(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

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

// extractTarGz writes binName from a tar.gz blob to dst with 0755; matches by Base name.
//
// extractTarGz 从 tar.gz 按 Base 名抽 binName 写到 dst（0755）。
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

// extractZip writes binName from a zip blob to dst; used for windows-amd64.
//
// extractZip 从 zip blob 抽 binName 写到 dst（windows-amd64 专用）。
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

// writeBinary streams r into dst (0755) via tmp+rename for atomicity.
//
// writeBinary 用 tmp+rename 原子写 r 到 dst（0755）。
func writeBinary(r io.Reader, dst string) error {
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		// §S3 例外: best-effort cleanup of half-written tmp.
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	return os.Rename(tmp, dst)
}

// lookupSum returns the hex digest for assetName from a SHASUMS256.txt blob.
//
// lookupSum 从 SHASUMS256.txt 找 assetName 对应的 hex digest。
func lookupSum(sums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "./") == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no entry for %s in SHASUMS256.txt", assetName)
}

// httpGetBytes GETs url and returns the body (capped at 100 MB).
//
// httpGetBytes 拉 url 返 body，上限 100 MB。
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

// mustLatestTag returns the latest tag for owner/repo via GitHub API; fatals on failure.
//
// mustLatestTag 通过 GitHub API 拿 owner/repo 最新 tag；失败 fatal。
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
