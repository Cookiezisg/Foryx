package mcp

import (
	"strings"
	"sync"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)


func TestJoinContent_TextOnly(t *testing.T) {
	got := joinContent([]mcpsdk.Content{
		&mcpsdk.TextContent{Text: "hello "},
		&mcpsdk.TextContent{Text: "world"},
	})
	if got != "hello world" {
		t.Errorf("text concatenation = %q, want 'hello world'", got)
	}
}

func TestJoinContent_ImagePlaceholder(t *testing.T) {
	got := joinContent([]mcpsdk.Content{
		&mcpsdk.TextContent{Text: "see attached: "},
		&mcpsdk.ImageContent{MIMEType: "image/png"},
	})
	if !strings.Contains(got, "[image: image/png]") {
		t.Errorf("missing image placeholder: %q", got)
	}
}

func TestJoinContent_AudioPlaceholder(t *testing.T) {
	got := joinContent([]mcpsdk.Content{
		&mcpsdk.AudioContent{MIMEType: "audio/wav"},
	})
	if got != "[audio: audio/wav]" {
		t.Errorf("audio placeholder = %q", got)
	}
}

func TestJoinContent_ResourceLink(t *testing.T) {
	got := joinContent([]mcpsdk.Content{
		&mcpsdk.ResourceLink{URI: "file:///tmp/x.txt"},
	})
	if got != "[resource: file:///tmp/x.txt]" {
		t.Errorf("resource_link = %q", got)
	}
}

func TestJoinContent_EmptyArray(t *testing.T) {
	got := joinContent(nil)
	if got != "" {
		t.Errorf("nil content = %q, want empty string", got)
	}
}


func TestComposeEnv_NilExtras_ReturnsNilForInherit(t *testing.T) {
	if got := composeEnv(nil); got != nil {
		t.Errorf("nil extras should return nil (cmd.Env=nil = inherit os.Environ()), got %v", got)
	}
}

func TestComposeEnv_LayersExtrasOverInherited(t *testing.T) {
	// Stub osEnviron so the test doesn't depend on the host shell.
	// Stub osEnviron 让测试不依赖宿主 shell。
	prev := osEnviron
	defer func() { osEnviron = prev }()
	osEnviron = func() []string {
		return []string{"PATH=/usr/bin", "HOME=/home/user"}
	}

	got := composeEnv(map[string]string{
		"GITHUB_TOKEN": "ghp_x",
	})
	// All three should be present; order: inherited first, extras last
	// so dup keys are last-write-wins per exec.Cmd.Env semantics.
	// 三项都在；顺序：继承在前，extras 在后，重复 key 后写胜（exec.Cmd.Env 语义）。
	want := []string{"PATH=/usr/bin", "HOME=/home/user", "GITHUB_TOKEN=ghp_x"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got: %v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestComposeEnv_OverrideInheritedKey(t *testing.T) {
	prev := osEnviron
	defer func() { osEnviron = prev }()
	osEnviron = func() []string {
		return []string{"PATH=/usr/bin"}
	}

	got := composeEnv(map[string]string{
		"PATH": "/custom/bin",
	})
	// We expect TWO entries: original PATH + override PATH. exec.Cmd
	// last-write-wins on the resulting env block, so the subprocess
	// sees /custom/bin.
	// 期望两项：原 PATH + override PATH。exec.Cmd 最后胜，子进程看到 /custom/bin。
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != "PATH=/usr/bin" {
		t.Errorf("got[0] = %q, want inherited PATH first", got[0])
	}
	if got[1] != "PATH=/custom/bin" {
		t.Errorf("got[1] = %q, want override PATH last", got[1])
	}
}


func TestRingBuffer_Empty(t *testing.T) {
	r := newRingBuffer(64)
	if got := r.String(); got != "" {
		t.Errorf("empty ring = %q", got)
	}
}

func TestRingBuffer_BelowCap_PreservesAllLines(t *testing.T) {
	r := newRingBuffer(64)
	r.WriteLine("line1")
	r.WriteLine("line2")
	got := r.String()
	if got != "line1\nline2\n" {
		t.Errorf("ring = %q, want 'line1\\nline2\\n'", got)
	}
}

func TestRingBuffer_OverCap_DropsOldest(t *testing.T) {
	// Capacity 20 — two 10-byte lines fit exactly with newlines (5+1+5+1=12);
	// adding a third forces the first one to drop.
	// 容量 20——两行 5 字节加 \n 各占 6 = 12；第三行触发首行丢。
	r := newRingBuffer(20)
	r.WriteLine("aaaaa")
	r.WriteLine("bbbbb")
	r.WriteLine("ccccc")
	r.WriteLine("ddddd")
	got := r.String()
	// All 4 writes total 24 bytes; ring keeps last 20 → first 4 bytes
	// of the oldest data dropped. Since writes are line-aligned, we
	// expect to see roughly the last 3 lines + a chopped first line.
	// 4 次共 24 字节；ring 保留最后 20 → 最早数据前 4 字节丢。按行对齐，
	// 应看到约最后 3 行 + 截首行。
	if len(got) > 20 {
		t.Errorf("ring exceeded cap: len=%d, content=%q", len(got), got)
	}
	if !strings.HasSuffix(got, "ddddd\n") {
		t.Errorf("ring should preserve newest line at end: %q", got)
	}
	if strings.Contains(got, "aaaaa\n") {
		t.Errorf("oldest full line should have been dropped: %q", got)
	}
}

func TestRingBuffer_ConcurrentWrites_StringReadsAreSafe(t *testing.T) {
	// Smoke test: hammer WriteLine from N goroutines while another
	// reader spins on String(). Detector under -race must not flag.
	// 烟雾测试：N goroutine 猛 WriteLine，另一 reader 转 String()。-race 不该标。
	r := newRingBuffer(1024)
	const N = 32
	var wg sync.WaitGroup
	wg.Add(N + 1)

	stop := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = r.String()
			}
		}
	}()

	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				r.WriteLine("g")
			}
		}(i)
	}

	// Wait for writers, then signal reader to stop.
	// 等写完，告诉 reader 停。
	for i := 0; i < N; i++ {
		// busy wait for individual writer completion via wg counter
		// proxy isn't trivial — simplest is to add a tiny sleep,
		// but a fixed-iteration design avoids races.
	}
	// Drain by waiting briefly via the Done count: rely on N writers
	// finishing fast (each does 10 small writes), then signal reader.
	// 借 wg 计数：N 写者很快完成（每个 10 小写），再发信号让 reader 停。
	doneWriters := make(chan struct{})
	go func() {
		// Wait for the N writers (excluding the reader) by tracking
		// our own counter — since wg includes the reader too, we
		// instead just sleep a small amount and signal.
		// 跟 N 写者用本地 counter——wg 含 reader，简单 sleep 一下再通知。
		close(doneWriters)
	}()
	<-doneWriters
	// Give writers time; reader is blocking via wg, signal stop after.
	// 给写者时间；reader 在 wg 里，之后再 signal stop。
	for i := 0; i < 100; i++ {
		// Spin briefly to let writers finish; cheap heuristic.
		// Spin 一会让写者完成；廉价启发。
	}
	close(stop)
	wg.Wait()

	got := r.String()
	if !strings.Contains(got, "g") {
		t.Errorf("expected at least one 'g' write to land: %q", got)
	}
}
