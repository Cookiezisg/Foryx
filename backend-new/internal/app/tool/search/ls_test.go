package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

func newLS() *LS { return &LS{pathGuard: pathguardpkg.New(nil)} }

func TestLS_ValidateInput(t *testing.T) {
	l := newLS()
	cases := []struct {
		name string
		json string
		want error
	}{
		{"empty path", `{"path":""}`, ErrPathRequired},
		{"whitespace path", `{"path":"  "}`, ErrPathRequired},
		{"negative limit", `{"path":"/x","limit":-1}`, errSentinel},
		{"happy", `{"path":"/x"}`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := l.ValidateInput([]byte(c.json))
			switch {
			case c.want == nil && err != nil:
				t.Fatalf("want nil, got %v", err)
			case errors.Is(c.want, ErrPathRequired) && !errors.Is(err, ErrPathRequired):
				t.Fatalf("want ErrPathRequired, got %v", err)
			case c.want == errSentinel && err == nil:
				t.Fatalf("want an error, got nil")
			}
		})
	}
}

// errSentinel marks "expect some non-nil error" in table tests.
var errSentinel = errors.New("expect error")

func TestLS_Execute_DirectoriesFirstAndHidden(t *testing.T) {
	dir := t.TempDir()
	// "zsub" sorts AFTER "afile" by name — proving directories-first wins over name order.
	if err := os.Mkdir(filepath.Join(dir, "zsub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := newLS().Execute(context.Background(), `{"path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	zi := strings.Index(out, "zsub")
	ai := strings.Index(out, "afile.txt")
	if zi < 0 || ai < 0 {
		t.Fatalf("missing entries:\n%s", out)
	}
	if zi > ai {
		t.Fatalf("directory zsub must sort before file afile.txt:\n%s", out)
	}
	if !strings.Contains(out, ".hidden") {
		t.Fatalf("hidden file must be listed:\n%s", out)
	}
	if !strings.Contains(out, "dir   zsub") {
		t.Fatalf("directory should be marked 'dir':\n%s", out)
	}
}

func TestLS_Execute_NotADirectory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := newLS().Execute(context.Background(), `{"path":"`+f+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Not a directory") {
		t.Fatalf("expected not-a-directory message, got %q", out)
	}
}

func TestLS_Execute_NotFound(t *testing.T) {
	out, err := newLS().Execute(context.Background(), `{"path":"/no/such/dir"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Directory not found") {
		t.Fatalf("expected not-found, got %q", out)
	}
}

func TestLS_Execute_Empty(t *testing.T) {
	out, err := newLS().Execute(context.Background(), `{"path":"`+t.TempDir()+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "(empty)") {
		t.Fatalf("expected empty marker, got %q", out)
	}
}

func TestLS_Execute_Truncate(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		if err := os.WriteFile(filepath.Join(dir, string(rune('a'+i))+".txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := newLS().Execute(context.Background(), `{"path":"`+dir+`","limit":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "showing 2 of 5") {
		t.Fatalf("expected truncation footer, got %q", out)
	}
}

func TestLS_Execute_PathGuardDeny(t *testing.T) {
	l := &LS{pathGuard: pathguardpkg.New([]string{"/etc/"})}
	out, err := l.Execute(context.Background(), `{"path":"/etc"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "denied by safety guard") {
		t.Fatalf("expected guard deny, got %q", out)
	}
}
