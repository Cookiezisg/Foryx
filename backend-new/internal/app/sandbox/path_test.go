package sandbox

import (
	"os"
	"strings"
	"testing"
)

// TestPrependPATH: runtime bin dirs go to the FRONT of an existing PATH (so a runtime's own
// npx/dnx wins over any system copy), and a missing PATH is created.
//
// TestPrependPATH：runtime bin 目录放到现有 PATH 最前（使 runtime 自己的 npx/dnx 压过系统副本），
// 缺 PATH 则新建。
func TestPrependPATH(t *testing.T) {
	sep := string(os.PathListSeparator)

	out := prependPATH([]string{"FOO=1", "PATH=/usr/bin"}, "/rt/bin", "/rt")
	var got string
	for _, kv := range out {
		if strings.HasPrefix(kv, "PATH=") {
			got = kv
		}
	}
	if want := "PATH=/rt/bin" + sep + "/rt" + sep + "/usr/bin"; got != want {
		t.Fatalf("prepend onto existing PATH: want %q, got %q", want, got)
	}

	out2 := prependPATH([]string{"FOO=1"}, "/rt/bin")
	found := false
	for _, kv := range out2 {
		if kv == "PATH=/rt/bin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing PATH should be created, got %v", out2)
	}
}
