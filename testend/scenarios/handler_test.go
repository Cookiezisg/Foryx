package scenarios

import (
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// hdCreate forges a handler over HTTP and returns its id.
//
// hdCreate 经 HTTP 锻造一个 handler 并返回 id。
func hdCreate(t *testing.T, wc *harness.Client, name string, body map[string]any) string {
	t.Helper()
	payload := map[string]any{"name": name, "description": "验收用"}
	for k, v := range body {
		payload[k] = v
	}
	r := wc.POST("/api/v1/handlers", payload)
	if r.Status >= 300 {
		t.Fatalf("handler create: %d %s", r.Status, r.Raw)
	}
	// Create 现返裸实体(MD1):data 顶层即 id + 内嵌 activeVersion。
	return r.Field(t, "id")
}

// TestHandler_ResidentLifecycleAndCalls: A2 核心——首调 spawn、状态保持（常驻的灵魂）、
// 调用台账 logs、restart 重置状态。
func TestHandler_ResidentLifecycleAndCalls(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "hd-life"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	hdID := hdCreate(t, wc, "counter_keeper", map[string]any{
		"initBody": "self.count = 0",
		"methods": []map[string]any{
			{"name": "bump", "inputs": []any{}, "body": "self.count += 1\nreturn {\"count\": self.count}"},
		},
	})

	// state persists across calls — the resident soul. 状态跨调用保持——常驻之魂。
	var out struct {
		Result map[string]any `json:"result"`
	}
	wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "bump", "args": map[string]any{}}).OK(t, &out)
	wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "bump", "args": map[string]any{}}).OK(t, &out)
	if out.Result["count"] != float64(2) {
		t.Fatalf("resident state lost: %+v", out.Result)
	}

	// restart resets in-memory state. restart 清内存状态。
	wc.POST("/api/v1/handlers/"+hdID+":restart", nil).OK(t, nil)
	wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "bump", "args": map[string]any{}}).OK(t, &out)
	if out.Result["count"] != float64(1) {
		t.Fatalf("restart must reset state: %+v", out.Result)
	}

	// unknown method rejects with the domain code. 未知方法按域码拒。
	r := wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "nope", "args": map[string]any{}})
	if r.Status < 400 || !strings.Contains(r.Code, "METHOD") {
		t.Fatalf("unknown method: %d/%s", r.Status, r.Code)
	}

	// calls ledger with aggregates; detail carries logs column. 调用台账+聚合；详情带 logs 列。
	var page struct {
		Calls []struct {
			ID string `json:"id"`
		} `json:"calls"`
		Aggregates struct {
			OKCount int `json:"okCount"`
		} `json:"aggregates"`
	}
	wc.GET("/api/v1/handlers/"+hdID+"/calls").OK(t, &page)
	if page.Aggregates.OKCount != 3 || len(page.Calls) < 3 {
		t.Fatalf("calls ledger wrong: %+v", page.Aggregates)
	}
	wc.GET("/api/v1/handler-calls/"+page.Calls[0].ID).OK(t, nil)
}

// TestHandler_PrintToStdout: A2 关键产品语义——用户代码 print 走 stdout（协议通道）时
// 会发生什么。真机观察并定性（finding 候选：function 已重定向、handler 是否同等保护）。
func TestHandler_PrintToStdout(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "hd-print"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	hdID := hdCreate(t, wc, "printy", map[string]any{
		"methods": []map[string]any{
			{"name": "speak", "inputs": []any{}, "body": "print(\"hello from handler\")\nreturn {\"ok\": True}"},
		},
	})
	// The driver shields stdout (AC-5 fix): print must NOT crash the protocol, and the
	// printed line must surface in the call's logs.
	// driver 护住 stdout（AC-5 修复）：print 绝不炸协议，且打印行须出现在调用 logs 里。
	var out struct {
		Result map[string]any `json:"result"`
	}
	wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "speak", "args": map[string]any{}}).OK(t, &out)
	if out.Result["ok"] != true {
		t.Fatalf("print method result wrong: %+v", out.Result)
	}
	var page struct {
		Calls []struct {
			ID string `json:"id"`
		} `json:"calls"`
	}
	wc.GET("/api/v1/handlers/"+hdID+"/calls").OK(t, &page)
	var detail struct {
		Logs string `json:"logs"`
	}
	wc.GET("/api/v1/handler-calls/"+page.Calls[0].ID).OK(t, &detail)
	if !strings.Contains(detail.Logs, "hello from handler") {
		t.Fatalf("print must land in call logs, got %q", detail.Logs)
	}
}

// TestHandler_ConfigFlow: A2 config——必填缺失拒 spawn、PUT 后生效、掩码回显、清空停机。
func TestHandler_ConfigFlow(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "hd-config"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	hdID := hdCreate(t, wc, "configured", map[string]any{
		"initBody": "self.token = token",
		"initArgsSchema": []map[string]any{
			{"name": "token", "type": "string", "required": true, "sensitive": true},
		},
		"methods": []map[string]any{
			{"name": "show", "inputs": []any{}, "body": "return {\"token_len\": len(self.token)}"},
		},
	})

	// missing required config → call rejects with the documented code. 必填缺失 → 调用按码拒。
	r := wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "show", "args": map[string]any{}})
	if r.Status < 400 || !strings.Contains(r.Code, "CONFIG") {
		t.Fatalf("missing config must reject with a CONFIG code: %d/%s %s", r.Status, r.Code, r.Raw)
	}

	// configure → call works and saw the value. 配上 → 调用成功且拿到值。
	wc.PUT("/api/v1/handlers/"+hdID+"/config", map[string]any{"token": "secret-12345"}).OK(t, nil)
	var out struct {
		Result map[string]any `json:"result"`
	}
	wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "show", "args": map[string]any{}}).OK(t, &out)
	if out.Result["token_len"] != float64(12) {
		t.Fatalf("config not applied: %+v", out.Result)
	}

	// masked echo: sensitive value never returns in plain. 掩码回显：敏感值绝不明文回。
	cfg := wc.GET("/api/v1/handlers/" + hdID + "/config")
	if strings.Contains(string(cfg.Raw), "secret-12345") {
		t.Fatalf("sensitive config echoed in plaintext: %s", cfg.Raw)
	}

	// clear stops the instance; next call rejects again. 清空停机；再调又拒。
	wc.DELETE("/api/v1/handlers/"+hdID+"/config").OK(t, nil)
	r = wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "show", "args": map[string]any{}})
	if r.Status < 400 {
		t.Fatalf("cleared config must reject calls again: %d", r.Status)
	}
}

// TestHandler_MethodTimeout: 方法级超时真触发——卡死方法不拖死调用方。
func TestHandler_MethodTimeout(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "hd-timeout"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	hdID := hdCreate(t, wc, "sleepy", map[string]any{
		"imports": "import time",
		"methods": []map[string]any{
			{"name": "nap", "inputs": []any{}, "timeout": 1500, "body": "time.sleep(10)\nreturn {\"woke\": True}"},
		},
	})
	r := wc.POST("/api/v1/handlers/"+hdID+":call", map[string]any{"method": "nap", "args": map[string]any{}})
	if r.Status < 400 || !strings.Contains(r.Code+r.Msg, "TIMEOUT") {
		t.Fatalf("timeout must surface as a TIMEOUT code: %d/%s %s", r.Status, r.Code, r.Raw)
	}
	// failed call lands in the ledger as timeout. 失败调用以 timeout 入台账。
	var page struct {
		Calls []struct {
			Status string `json:"status"`
		} `json:"calls"`
	}
	wc.GET("/api/v1/handlers/"+hdID+"/calls?status=timeout").OK(t, &page)
	if len(page.Calls) != 1 {
		t.Fatalf("timeout call not in ledger: %+v", page.Calls)
	}
}
