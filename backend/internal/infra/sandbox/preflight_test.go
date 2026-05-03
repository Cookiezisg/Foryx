package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// ── platformKey ───────────────────────────────────────────────────────────────

func TestPlatformKey(t *testing.T) {
	got, err := platformKey()
	if err != nil {
		// Only fails on unsupported OS/arch — tests should always run on a
		// supported combo.
		t.Fatalf("platformKey() error: %v", err)
	}
	want := runtime.GOOS + "-" + runtime.GOARCH
	if got != want {
		t.Errorf("platformKey() = %q, want %q", got, want)
	}
}

// ── computeBootstrapHash ──────────────────────────────────────────────────────

func TestComputeBootstrapHash_StableAndContentSensitive(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	h1, err := computeBootstrapHash(a, b)
	if err != nil {
		t.Fatalf("hash err: %v", err)
	}
	h2, err := computeBootstrapHash(a, b)
	if err != nil {
		t.Fatalf("hash err: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hash should be stable: %q vs %q", h1, h2)
	}

	// Change content: hash must differ.
	// 改内容：hash 必须变。
	if err := os.WriteFile(a, []byte("hello!"), 0o644); err != nil {
		t.Fatal(err)
	}
	h3, err := computeBootstrapHash(a, b)
	if err != nil {
		t.Fatalf("hash err: %v", err)
	}
	if h3 == h1 {
		t.Errorf("hash should differ after content change")
	}
}

func TestComputeBootstrapHash_OrderSensitive(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	os.WriteFile(a, []byte("alpha"), 0o644)
	os.WriteFile(b, []byte("beta"), 0o644)

	hAB, _ := computeBootstrapHash(a, b)
	hBA, _ := computeBootstrapHash(b, a)
	if hAB == hBA {
		t.Errorf("hash should be order-sensitive (uv first, python second)")
	}
}

func TestComputeBootstrapHash_MissingFile(t *testing.T) {
	_, err := computeBootstrapHash("/nonexistent/uv", "/nonexistent/py.tar.gz")
	if err == nil {
		t.Errorf("expected error for missing source files")
	}
}

// ── copyExecutable ────────────────────────────────────────────────────────────

func TestCopyExecutable(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dest := filepath.Join(t.TempDir(), "nested", "dest")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyExecutable(src, dest); err != nil {
		t.Fatalf("copyExecutable err: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Errorf("dest content = %q, want %q", got, "payload")
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		// On windows mode bits are mostly cosmetic; only check on unix.
		// windows 上 mode 位主要是装饰；只在 unix 上检查。
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("dest should be executable, got mode %v", info.Mode())
		}
	}
}

func TestCopyExecutable_Overwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "dest")
	os.WriteFile(src, []byte("new"), 0o644)
	os.WriteFile(dest, []byte("old"), 0o644)

	if err := copyExecutable(src, dest); err != nil {
		t.Fatalf("copyExecutable err: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "new" {
		t.Errorf("dest content = %q, want overwrite to %q", got, "new")
	}
}

// ── extractTarGz ──────────────────────────────────────────────────────────────

// buildTarGz creates an in-memory tar.gz with the given entries.
//
// buildTarGz 在内存中构造给定条目的 tar.gz。
type tarEntry struct {
	name     string
	body     string
	mode     int64
	typeFlag byte
	linkname string
}

func buildTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Typeflag: e.typeFlag,
			Linkname: e.linkname,
			Size:     int64(len(e.body)),
		}
		if hdr.Mode == 0 {
			hdr.Mode = 0o644
		}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if hdr.Typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg && len(e.body) > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write body: %v", err)
			}
		}
	}
	tw.Close()
	gz.Close()

	path := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}
	return path
}

func TestExtractTarGz_StripsPythonPrefix(t *testing.T) {
	src := buildTarGz(t, []tarEntry{
		{name: "python/", typeFlag: tar.TypeDir, mode: 0o755},
		{name: "python/bin/", typeFlag: tar.TypeDir, mode: 0o755},
		{name: "python/bin/python3", body: "fakepy", mode: 0o755},
		{name: "python/lib/python3.12/os.py", body: "import sys", mode: 0o644},
	})

	dest := t.TempDir()
	if err := extractTarGz(src, dest); err != nil {
		t.Fatalf("extract err: %v", err)
	}

	// "python/" prefix stripped — files land directly under dest.
	// "python/" 前缀剥掉——文件直接在 dest 下。
	if _, err := os.Stat(filepath.Join(dest, "bin", "python3")); err != nil {
		t.Errorf("expected bin/python3 under dest, got: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "bin", "python3"))
	if string(got) != "fakepy" {
		t.Errorf("file content = %q, want %q", got, "fakepy")
	}
	if _, err := os.Stat(filepath.Join(dest, "lib", "python3.12", "os.py")); err != nil {
		t.Errorf("expected lib/python3.12/os.py: %v", err)
	}
}

func TestExtractTarGz_PreservesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits not meaningful on windows")
	}

	src := buildTarGz(t, []tarEntry{
		{name: "python/bin/python3", body: "fakepy", mode: 0o755},
	})
	dest := t.TempDir()
	if err := extractTarGz(src, dest); err != nil {
		t.Fatalf("extract err: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "bin", "python3"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("executable mode lost, got %v", info.Mode().Perm())
	}
}

func TestExtractTarGz_HandlesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}

	src := buildTarGz(t, []tarEntry{
		{name: "python/bin/python3.13", body: "fakepy", mode: 0o755},
		{name: "python/bin/python3", typeFlag: tar.TypeSymlink, linkname: "python3.13"},
	})
	dest := t.TempDir()
	if err := extractTarGz(src, dest); err != nil {
		t.Fatalf("extract err: %v", err)
	}

	link := filepath.Join(dest, "bin", "python3")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "python3.13" {
		t.Errorf("symlink target = %q, want %q", target, "python3.13")
	}
}

func TestExtractTarGz_RejectsZipSlip(t *testing.T) {
	src := buildTarGz(t, []tarEntry{
		{name: "python/../../etc/passwd", body: "evil"},
	})
	dest := t.TempDir()
	err := extractTarGz(src, dest)
	if err == nil {
		t.Fatal("expected zip-slip rejection")
	}
	if !strings.Contains(err.Error(), "escapes dest") {
		t.Errorf("error should mention escape, got: %v", err)
	}
}

func TestExtractTarGz_NoPythonPrefix(t *testing.T) {
	// Tarball without leading "python/" prefix should still extract fine.
	// 没有前导 "python/" 前缀的 tarball 也该正常解压。
	src := buildTarGz(t, []tarEntry{
		{name: "loose-file", body: "hi"},
	})
	dest := t.TempDir()
	if err := extractTarGz(src, dest); err != nil {
		t.Fatalf("extract err: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "loose-file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi" {
		t.Errorf("content = %q, want %q", got, "hi")
	}
}

// ── withUVEnv ─────────────────────────────────────────────────────────────────

func TestWithUVEnv_SetsExpectedVars(t *testing.T) {
	s := New(Config{
		DataDir:       "/data",
		DefaultPython: ">=3.12",
		Logger:        zap.NewNop(),
	})

	env := s.withUVEnv()

	must := []string{
		"UV_CACHE_DIR=" + filepath.Join("/data", "uv-cache"),
		"UV_NO_CONFIG=1",
		"UV_NO_PROGRESS=1",
		"UV_PYTHON=" + s.PythonPath(),
		"PYTHONDONTWRITEBYTECODE=1",
	}
	for _, m := range must {
		found := false
		for _, e := range env {
			if e == m {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("withUVEnv missing %q", m)
		}
	}

	// os.Environ() should also be merged in (PATH or HOME usually present).
	// os.Environ() 也该合进来（PATH 或 HOME 通常都有）。
	hasBaseEnv := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") || strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "USER=") {
			hasBaseEnv = true
			break
		}
	}
	if !hasBaseEnv {
		t.Errorf("withUVEnv should include base os.Environ(); got %d entries without recognizable base var", len(env))
	}
}

// ── ensureReady ───────────────────────────────────────────────────────────────

func TestEnsureReady_BeforeBootstrap(t *testing.T) {
	s := New(Config{
		DataDir: "/data",
		Logger:  zap.NewNop(),
	})
	if err := s.ensureReady(); err == nil {
		t.Error("ensureReady should fail before Bootstrap")
	}
}

func TestEnsureReady_AfterBootstrap(t *testing.T) {
	s := New(Config{
		DataDir: "/data",
		Logger:  zap.NewNop(),
	})
	s.bootstrapped = true // simulate successful bootstrap
	if err := s.ensureReady(); err != nil {
		t.Errorf("ensureReady should pass after bootstrap, got %v", err)
	}
}
