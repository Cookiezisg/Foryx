package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_Dev(t *testing.T) {
	log, err := New(true, "")
	if err != nil {
		t.Fatalf("New(true): %v", err)
	}
	if log == nil {
		t.Fatal("nil logger")
	}
	log.Info("dev logger smoke") // must not panic
	_ = log.Sync()
}

func TestNew_Prod(t *testing.T) {
	log, err := New(false, "")
	if err != nil {
		t.Fatalf("New(false): %v", err)
	}
	if log == nil {
		t.Fatal("nil logger")
	}
	log.Info("prod logger smoke")
	_ = log.Sync()
}

// TestNew_FileSink: with a logDir the logger tees into <dir>/forgify.log — the desktop
// support story ("send me the log file").
//
// TestNew_FileSink：给 logDir 时 logger tee 进 <dir>/forgify.log——桌面报障故事（「把日志文件发我」）。
func TestNew_FileSink(t *testing.T) {
	dir := t.TempDir()
	log, err := New(false, dir)
	if err != nil {
		t.Fatalf("New with logDir: %v", err)
	}
	log.Info("file sink smoke")
	_ = log.Sync()
	b, err := os.ReadFile(filepath.Join(dir, "forgify.log"))
	if err != nil {
		t.Fatalf("log file missing: %v", err)
	}
	if !strings.Contains(string(b), "file sink smoke") {
		t.Fatalf("log line not written: %q", b)
	}
}
