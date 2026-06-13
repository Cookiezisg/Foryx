package scenarios

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// trgCreate creates a trigger and returns its id.
//
// trgCreate 建一个 trigger 并返回 id。
func trgCreate(t *testing.T, wc *harness.Client, name, kind string, config map[string]any) string {
	t.Helper()
	// Create 现返裸实体(MD1):data 顶层即 id。
	return wc.POST("/api/v1/triggers", map[string]any{"name": name, "kind": kind, "config": config}).Field(t, "id")
}

// wfWithTrigger builds fn + workflow wired to the given trigger and activates it.
//
// wfWithTrigger 建 fn + 接到给定 trigger 的 workflow 并激活。
func wfWithTrigger(t *testing.T, wc *harness.Client, name, trgID string) (wfID, fnID string) {
	t.Helper()
	fnID = fnCreate(t, wc, name+"_step", "def f() -> dict:\n    return {\"fired\": True}\n")
	wfID = wfCreate(t, wc, name, []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "start", "kind": "trigger", "ref": trgID}},
		{"op": "add_node", "node": map[string]any{"id": "step", "kind": "action", "ref": fnID}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "start", "to": "step"}},
	})
	wc.POST("/api/v1/workflows/"+wfID+":activate", map[string]any{}).OK(t, nil)
	return wfID, fnID
}

// waitRunCompleted polls until the workflow has ≥1 completed run.
//
// waitRunCompleted 轮询直到 workflow 有 ≥1 个 completed run。
func waitRunCompleted(t *testing.T, wc *harness.Client, wfID string, timeoutMS int) {
	t.Helper()
	harness.Eventually(t, timeoutMS, "a run completes for "+wfID, func() bool {
		r := wc.GET("/api/v1/flowruns?workflowId=" + wfID + "&status=completed")
		return r.Status == 200 && strings.Contains(string(r.Data), `"status":"completed"`)
	})
}

// TestTrigger_WebhookFiresAndVerifies: webhook 真触发——HMAC 正签过、坏签 401、run 真跑。
func TestTrigger_WebhookFiresAndVerifies(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "trg-hook"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	secret := "hooksecret"
	trgID := trgCreate(t, wc, "gh_hook", "webhook", map[string]any{
		"path": "incoming", "secret": secret, "signatureAlgo": "hmac-sha256-hex",
	})
	wfID, _ := wfWithTrigger(t, wc, "hook_pipe", trgID)

	body := []byte(`{"event":"push"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	url := srv.BaseURL + "/api/v1/webhooks/" + trgID + "/incoming"

	// bad signature → 401, no run. 坏签 → 401、无 run。
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bad-sig post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("bad signature must 401, got %d", resp.StatusCode)
	}

	// good signature → accepted → activation 记录 + run completed. 正签 → 触发 → run 完成。
	req, _ = http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sig)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("good-sig post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("good signature must accept, got %d", resp.StatusCode)
	}
	waitRunCompleted(t, wc, wfID, 30000)

	// activation ledger saw it. 活动台账有记录。
	r := wc.GET("/api/v1/triggers/" + trgID + "/activations")
	if !strings.Contains(string(r.Data), `"fired":true`) {
		t.Fatalf("activation ledger missing the fire: %s", r.Data)
	}
	// firings inbox: started. firing 收件箱：started。
	r = wc.GET("/api/v1/triggers/" + trgID + "/firings")
	if !strings.Contains(string(r.Data), `"status":"started"`) {
		t.Fatalf("firing must be started: %s", r.Data)
	}
}

// TestTrigger_CronEveryFires: cron @every 真等到点触发（也顺带探明 sub-minute dedup 行为）。
func TestTrigger_CronEveryFires(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "trg-cron"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// 5-field cron = minute granularity (ParseStandard; @every/seconds rejected — see AC-11).
	// Real-fire waits for the next minute boundary, so the timeout is generous.
	// 5 段 cron = 分钟粒度（ParseStandard；@every/秒级被拒——见 AC-11）。真触发等下一个分钟边界，超时放宽。
	trgID := trgCreate(t, wc, "tick", "cron", map[string]any{"expression": "* * * * *"})
	wfID, _ := wfWithTrigger(t, wc, "cron_pipe", trgID)
	waitRunCompleted(t, wc, wfID, 75000)
	_ = wfID
}

// TestTrigger_SensorPollsCEL: sensor 真轮询——CEL 真→触发跑 run；activation 记录 ReturnValue。
func TestTrigger_SensorPollsCEL(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "trg-sensor"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	probeFn := fnCreate(t, wc, "probe_fn", "def f() -> dict:\n    return {\"level\": 42}\n")
	trgID := trgCreate(t, wc, "level_watch", "sensor", map[string]any{
		"targetKind": "function", "targetId": probeFn, "intervalSec": 5,
		"condition": "payload.level > 10", "output": "{\"level\": payload.level}",
	})
	wfID, _ := wfWithTrigger(t, wc, "sensor_pipe", trgID)
	waitRunCompleted(t, wc, wfID, 40000)

	// activation carries the probe's return value. activation 带探测返回值。
	r := wc.GET("/api/v1/triggers/" + trgID + "/activations")
	if !strings.Contains(string(r.Data), `"level":42`) && !strings.Contains(string(r.Data), `"level": 42`) {
		t.Fatalf("activation must carry probe return: %.400s", r.Data)
	}
}
