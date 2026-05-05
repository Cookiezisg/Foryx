// installer_static_test.go — pure-function unit tests for
// StaticBinaryInstaller. Real HTTP downloads belong in the D9 pipeline
// suite; here we cover parseStaticVersion + path derivation in Locate.
//
// installer_static_test.go ——StaticBinaryInstaller pure-function 单测。
// 真 HTTP 下载归 D9 pipeline 套；这里覆盖 parseStaticVersion + Locate 路径
// 推导。

package sandbox

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.RuntimeInstaller = (*StaticBinaryInstaller)(nil)

func TestStaticBinaryInstaller_Kind(t *testing.T) {
	si := NewStaticBinaryInstaller("github-mcp", zap.NewNop())
	if got := si.Kind(); got != "github-mcp" {
		t.Errorf("Kind() = %q, want github-mcp", got)
	}
}

func TestStaticBinaryInstaller_ListAvailable_Nil(t *testing.T) {
	si := NewStaticBinaryInstaller("github-mcp", zap.NewNop())
	got, err := si.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if got != nil {
		t.Errorf("ListAvailable: want nil (no enumeration), got %v", got)
	}
}

func TestStaticBinaryInstaller_ResolveDefault_Empty(t *testing.T) {
	si := NewStaticBinaryInstaller("github-mcp", zap.NewNop())
	got, err := si.ResolveDefault(context.Background())
	if err != nil {
		t.Fatalf("ResolveDefault: %v", err)
	}
	if got != "" {
		t.Errorf("ResolveDefault: want empty (no default URL), got %q", got)
	}
}

func TestParseStaticVersion(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantURL  string
		wantHash string
		wantErr  bool
	}{
		{
			name:    "plain URL",
			input:   "https://example.com/binaries/github-mcp",
			wantURL: "https://example.com/binaries/github-mcp",
		},
		{
			name:     "sha256 + URL",
			input:    "sha256:" + strings.Repeat("a", 64) + "@https://example.com/binaries/github-mcp",
			wantURL:  "https://example.com/binaries/github-mcp",
			wantHash: strings.Repeat("a", 64),
		},
		{
			name:    "empty version",
			input:   "",
			wantErr: true,
		},
		{
			name:    "sha256 missing @",
			input:   "sha256:" + strings.Repeat("a", 64),
			wantErr: true,
		},
		{
			name:    "sha256 wrong length",
			input:   "sha256:abc@https://example.com/x",
			wantErr: true,
		},
		{
			name:    "sha256 empty URL",
			input:   "sha256:" + strings.Repeat("a", 64) + "@",
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotHash, err := parseStaticVersion(c.input)
			if c.wantErr {
				if err == nil {
					t.Errorf("want error, got url=%q hash=%q", gotURL, gotHash)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if gotURL != c.wantURL {
				t.Errorf("URL = %q, want %q", gotURL, c.wantURL)
			}
			if gotHash != c.wantHash {
				t.Errorf("hash = %q, want %q", gotHash, c.wantHash)
			}
		})
	}
}

func TestStaticBinaryInstaller_Locate(t *testing.T) {
	si := NewStaticBinaryInstaller("github-mcp", zap.NewNop())
	got, err := si.Locate("https://example.com/releases/v1/github-mcp", "/data/sandbox")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	want := filepath.Join("/data/sandbox", staticBinariesSubdir, "github-mcp", "github-mcp")
	if got != want {
		t.Errorf("Locate = %q, want %q", got, want)
	}
}

func TestStaticBinaryInstaller_Locate_AcceptsShaPrefix(t *testing.T) {
	si := NewStaticBinaryInstaller("github-mcp", zap.NewNop())
	version := "sha256:" + strings.Repeat("a", 64) + "@https://example.com/v1/binary"
	got, err := si.Locate(version, "/data/sandbox")
	if err != nil {
		t.Fatalf("Locate sha256 form: %v", err)
	}
	want := filepath.Join("/data/sandbox", staticBinariesSubdir, "github-mcp", "binary")
	if got != want {
		t.Errorf("Locate sha256 form = %q, want %q", got, want)
	}
}
