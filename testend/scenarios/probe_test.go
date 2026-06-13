package scenarios

import (
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// TestFunction_CreateEnvVisibility pins the product promise behind the SYNCHRONOUS
// create: while the HTTP call blocks on env materialization, the notifications stream
// must already be telling the user something is happening — function.created lands
// immediately, env_status_changed marks the build start and its terminal state. The
// user is never staring at a silent spinner (AC-PD-1 closed by design on this evidence).
//
// TestFunction_CreateEnvVisibility 钉死同步 create 背后的产品承诺：HTTP 阻塞在 env 物化
// 期间，notifications 流必须已经在告诉用户「在搞了」——function.created 立即落、
// env_status_changed 标记构建开始与终态。用户绝不对着无声 spinner（AC-PD-1 凭此证据
// by-design 关闭）。
func TestFunction_CreateEnvVisibility(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "env-vis"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	ns := wc.Subscribe(t, "notifications")
	// The POST runs in a goroutine; its status comes back over the channel (Fatalf is
	// illegal off the test goroutine).
	// POST 在 goroutine 里跑；状态经 channel 带回（测试 goroutine 之外不可 Fatalf）。
	done := make(chan int, 1)
	go func() {
		r := wc.POST("/api/v1/functions", map[string]any{
			"name": "dep_probe", "code": "import requests\ndef f() -> dict:\n    return {}\n",
			"dependencies": []string{"requests"},
		})
		done <- r.Status
	}()

	// Both signals must arrive WHILE create may still be blocking — the wait runs
	// concurrently with the POST, not after it.
	// 两个信号必须在 create 可能仍阻塞期间到达——等待与 POST 并发，不在其后。
	ns.WaitFor(t, 20000, "created lands before env build finishes", "function.created")
	ns.WaitFor(t, 30000, "env build start/terminal is signalled", "sandbox.env_status_changed")
	if st := <-done; st != 201 {
		t.Fatalf("create failed: %d", st)
	}
	ns.WaitFor(t, 10000, "env reaches a terminal state", "sandbox.env_status_changed", "ready")
}
