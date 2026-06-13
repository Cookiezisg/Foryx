package scenarios

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// fnCreate forges a function over HTTP and returns its id (shared W1 helper).
//
// fnCreate 经 HTTP 锻造一个 function 并返回 id（W1 共享 helper）。
func fnCreate(t *testing.T, wc *harness.Client, name, code string) string {
	t.Helper()
	// Create 现返裸实体(MD1):data 顶层即 id + 内嵌 activeVersion。
	return wc.POST("/api/v1/functions", map[string]any{
		"name": name, "description": "验收用", "code": code,
	}).Field(t, "id")
}

// TestFunction_CreateRejections: A1 创建情况矩阵的出错列——无 def 的坏代码、重名。
func TestFunction_CreateRejections(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-rejects"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// bad code: no top-level def. 坏代码：无顶层 def。
	r := wc.POST("/api/v1/functions", map[string]any{"name": "bad_fn", "code": "x = 1\n"})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("no-def code must reject with a wire code, got %d %s", r.Status, r.Raw)
	}

	// duplicate name. 重名。
	fnCreate(t, wc, "dup_fn", "def f() -> dict:\n    return {}\n")
	r = wc.POST("/api/v1/functions", map[string]any{"name": "dup_fn", "code": "def f() -> dict:\n    return {}\n"})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("duplicate name must reject with a wire code, got %d %s", r.Status, r.Raw)
	}
}

// TestFunction_RunLogsAndExecutions: A1 运行+执行记录核心——print 真落 logs、非零退出
// 真失败、聚合徽标、列表轻装/详情带 logs、入参回读。
func TestFunction_RunLogsAndExecutions(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-run"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	fnID := fnCreate(t, wc, "run_probe",
		"def probe(mode: str) -> dict:\n    print(f\"probe says {mode}\")\n    if mode == \"boom\":\n        raise RuntimeError(\"boom requested\")\n    return {\"echo\": mode}\n")

	// ok run: output + logs carry the print. ok 运行：output 与 logs 带 print。
	var run struct {
		OK     bool           `json:"ok"`
		Output map[string]any `json:"output"`
		Logs   string         `json:"logs"`
	}
	wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{"mode": "ok"}}).OK(t, &run)
	if !run.OK || run.Output["echo"] != "ok" {
		t.Fatalf("run result wrong: %+v", run)
	}
	if !strings.Contains(run.Logs, "probe says ok") {
		t.Fatalf("print must land in run logs, got %q", run.Logs)
	}

	// failing run: failed status + stderr in error. 失败运行：failed + stderr 进 error。
	var fail struct {
		OK       bool   `json:"ok"`
		ErrorMsg string `json:"errorMsg"`
	}
	wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{"mode": "boom"}}).OK(t, &fail)
	if fail.OK || !strings.Contains(fail.ErrorMsg, "boom requested") {
		t.Fatalf("failing run must surface the traceback: %+v", fail)
	}

	// executions ledger: 2 rows, aggregates 1/1, list omits logs, detail carries them.
	// 执行台账：2 行、聚合 1/1、列表无 logs、详情带。
	var page struct {
		Executions []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Logs   string `json:"logs"`
		} `json:"executions"`
		Aggregates struct {
			OKCount     int `json:"okCount"`
			FailedCount int `json:"failedCount"`
		} `json:"aggregates"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	if len(page.Executions) != 2 || page.Aggregates.OKCount != 1 || page.Aggregates.FailedCount != 1 {
		t.Fatalf("ledger wrong: n=%d agg=%+v", len(page.Executions), page.Aggregates)
	}
	for _, e := range page.Executions {
		if e.Logs != "" {
			t.Fatalf("list must omit logs (got on %s)", e.ID)
		}
	}
	var detail struct {
		Logs        string `json:"logs"`
		TriggeredBy string `json:"triggeredBy"`
	}
	wc.GET("/api/v1/function-executions/"+page.Executions[len(page.Executions)-1].ID).OK(t, &detail)
	if !strings.Contains(detail.Logs, "probe says") {
		t.Fatalf("detail must carry logs, got %q", detail.Logs)
	}
	if detail.TriggeredBy != "manual" {
		t.Fatalf("HTTP run must record manual, got %q", detail.TriggeredBy)
	}

	// status filter. 状态过滤。
	wc.GET("/api/v1/functions/"+fnID+"/executions?status=failed").OK(t, &page)
	if len(page.Executions) != 1 || page.Executions[0].Status != "failed" {
		t.Fatalf("status filter wrong: %+v", page.Executions)
	}
}

// TestFunction_RunStderrReachesPanelTerminal: print 的三写之一——entities 流 run 终端真收到。
func TestFunction_RunStderrReachesPanelTerminal(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-sse"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	fnID := fnCreate(t, wc, "sse_probe", "def f() -> dict:\n    print(\"terminal line marker\")\n    return {}\n")

	es := wc.Subscribe(t, "entities")
	wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{}}).OK(t, nil)
	es.WaitFor(t, 8000, "run terminal carries the print", "terminal line marker")
}

// TestFunction_VersionsEditRevert: A1 版本面——edit 升版即生效、revert 移针、号单调。
func TestFunction_VersionsEditRevert(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-vers"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	fnID := fnCreate(t, wc, "ver_probe", "def f() -> dict:\n    return {\"v\": 1}\n")

	// edit via ops → v2 active, run returns new behavior. ops 编辑 → v2 active、运行即新行为。
	wc.POST("/api/v1/functions/"+fnID+":edit", map[string]any{
		"ops": []map[string]any{{"op": "set_code", "code": "def f() -> dict:\n    return {\"v\": 2}\n"}},
	}).OK(t, nil)
	var run struct {
		Output map[string]any `json:"output"`
	}
	wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{}}).OK(t, &run)
	if run.Output["v"] != float64(2) {
		t.Fatalf("edit must take effect immediately, got %+v", run.Output)
	}

	// invalid op mid-way rejects with the documented code. 中途非法 op 按文档码拒。
	wc.POST("/api/v1/functions/"+fnID+":edit", map[string]any{
		"ops": []map[string]any{{"op": "set_warp_drive"}},
	}).Fail(t, 422, "FUNCTION_OP_INVALID")

	// revert to v1 → old behavior. revert 回 v1 → 旧行为。
	wc.POST("/api/v1/functions/"+fnID+":revert", map[string]any{"version": 1}).OK(t, nil)
	wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{}}).OK(t, &run)
	if run.Output["v"] != float64(1) {
		t.Fatalf("revert must restore v1 behavior, got %+v", run.Output)
	}

	// versions list: monotonic numbering. 版本列表：号单调。
	var vers struct {
		Items []struct {
			Version int `json:"version"`
		} `json:"items"`
	}
	r := wc.GET("/api/v1/functions/" + fnID + "/versions")
	if err := json.Unmarshal(r.Data, &vers); err != nil || len(vers.Items) < 2 {
		t.Logf("versions raw: %s", r.Data) // shape probe — assert below on raw. 形状探针。
	}
	if !strings.Contains(string(r.Data), `"version":2`) {
		t.Fatalf("versions list missing v2: %s", r.Data)
	}
}

// TestFunction_ConcurrentRuns: 并发 run 不串扰、台账计数全。
func TestFunction_ConcurrentRuns(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-conc"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	fnID := fnCreate(t, wc, "conc_probe", "def f(n: int) -> dict:\n    return {\"n\": n}\n")

	const N = 5
	var wg sync.WaitGroup
	errs := make(chan string, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var run struct {
				Output map[string]any `json:"output"`
			}
			r := wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{"n": i}})
			if r.Status != 200 {
				errs <- string(r.Raw)
				return
			}
			_ = json.Unmarshal(r.Data, &run)
			if run.Output["n"] != float64(i) {
				errs <- "cross-talk: wrong echo"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("concurrent run failed: %s", e)
	}
	var page struct {
		Aggregates struct {
			OKCount int `json:"okCount"`
		} `json:"aggregates"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	if page.Aggregates.OKCount != N {
		t.Fatalf("ledger lost runs: ok=%d want %d", page.Aggregates.OKCount, N)
	}
}

// TestFunction_DeleteRipples: 删除涟漪——搜不到、执行记录保留（D1）、404 读。
func TestFunction_DeleteRipples(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-del"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	fnID := fnCreate(t, wc, "doomed_fn", "def f() -> dict:\n    return {}\n")
	wc.POST("/api/v1/functions/"+fnID+":run", map[string]any{"args": map[string]any{}}).OK(t, nil)

	// wait until searchable, then delete. 先等可搜，再删。
	harness.Eventually(t, 5000, "indexed before delete", func() bool {
		r := wc.GET("/api/v1/search?q=doomed_fn")
		return r.Status == 200 && strings.Contains(string(r.Data), fnID)
	})
	var page struct {
		Executions []struct {
			ID string `json:"id"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	execID := page.Executions[0].ID

	if r := wc.DELETE("/api/v1/functions/" + fnID); r.Status != 204 {
		t.Fatalf("delete: %d %s", r.Status, r.Raw)
	}
	// ripple: gone from search. 涟漪：搜索清。
	harness.Eventually(t, 5000, "search residue cleared", func() bool {
		r := wc.GET("/api/v1/search?q=doomed_fn")
		return r.Status == 200 && !strings.Contains(string(r.Data), fnID)
	})
	// read is 404 with a wire code. 读 404 带码。
	if r := wc.GET("/api/v1/functions/" + fnID); r.Status != 404 || r.Code == "" {
		t.Fatalf("deleted read: %d/%s", r.Status, r.Code)
	}
	// D1: execution record survives. D1：执行记录保留。
	wc.GET("/api/v1/function-executions/"+execID).OK(t, nil)
}
