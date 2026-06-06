package fspath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExpand_Empty(t *testing.T) {
	for _, in := range []string{"", "   ", "\t"} {
		if _, err := Expand(in); !errors.Is(err, ErrEmptyPath) {
			t.Fatalf("Expand(%q) err = %v, want ErrEmptyPath", in, err)
		}
	}
}

func TestExpand_Relative_Rejected(t *testing.T) {
	for _, in := range []string{"foo/bar", "./x", "../y", "rel.go"} {
		if _, err := Expand(in); !errors.Is(err, ErrNotAbsolute) {
			t.Fatalf("Expand(%q) err = %v, want ErrNotAbsolute", in, err)
		}
	}
}

func TestExpand_Absolute_Cleaned(t *testing.T) {
	cases := map[string]string{
		"/x/y":    "/x/y",
		"/x/../y": "/y",
		"/x/./y/": "/x/y",
		"/a//b":   "/a/b",
	}
	for in, want := range cases {
		got, err := Expand(in)
		if err != nil {
			t.Fatalf("Expand(%q) err = %v", in, err)
		}
		if got != want {
			t.Fatalf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpand_TildeRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home dir unknown in this environment")
	}
	got, err := Expand("~")
	if err != nil {
		t.Fatalf("Expand(~) err = %v", err)
	}
	if got != filepath.Clean(home) {
		t.Fatalf("Expand(~) = %q, want %q", got, home)
	}
}

func TestExpand_TildeSubpath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home dir unknown in this environment")
	}
	got, err := Expand("~/Downloads/x.pdf")
	if err != nil {
		t.Fatalf("Expand err = %v", err)
	}
	want := filepath.Join(home, "Downloads", "x.pdf")
	if got != want {
		t.Fatalf("Expand(~/Downloads/x.pdf) = %q, want %q", got, want)
	}
}

func TestExpand_TildeUser_NotSupported(t *testing.T) {
	// "~user" is not "~" nor "~/" → no expansion → not absolute → rejected.
	// "~user" 既非 "~" 也非 "~/" → 不展开 → 非绝对 → 拒绝。
	if _, err := Expand("~bob/x"); !errors.Is(err, ErrNotAbsolute) {
		t.Fatalf("Expand(~bob/x) err = %v, want ErrNotAbsolute", err)
	}
}
