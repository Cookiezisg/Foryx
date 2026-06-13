package scenarios

import (
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// TestControl_CRUDAndCELValidation: A3 控制实体——创建/读/版本编辑生效/坏 CEL 拒。
// 真实路由战场（port 选边+emit 下游可读）在 W2 workflow 内验。
func TestControl_CRUDAndCELValidation(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "ctl"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// Create 现返裸实体(MD1):data 顶层即 id。
	ctlID := wc.POST("/api/v1/controls", map[string]any{
		"name":        "amount_gate",
		"description": "按金额分流",
		"inputs":      []map[string]any{{"name": "amount", "type": "number"}},
		"branches": []map[string]any{
			{"port": "big", "when": "input.amount > 100", "emit": map[string]string{"tier": "'vip'"}},
			{"port": "small", "when": "true"},
		},
	}).Field(t, "id")
	if !strings.HasPrefix(ctlID, "ctl_") {
		t.Fatalf("control id shape: %s", ctlID)
	}

	// bad CEL must reject with a wire code at create time, not explode at run time.
	// 坏 CEL 必须创建时按码拒，而非运行时爆。
	r := wc.POST("/api/v1/controls", map[string]any{
		"name": "broken_gate",
		"branches": []map[string]any{
			{"port": "a", "when": "input.amount >>> oops"},
		},
	})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("bad CEL must reject with a wire code, got %d %s", r.Status, r.Raw)
	}

	// edit produces v2 and reads back. 编辑出 v2 并可读回。
	wc.POST("/api/v1/controls/"+ctlID+":edit", map[string]any{
		"branches": []map[string]any{
			{"port": "big", "when": "input.amount > 200"},
			{"port": "small", "when": "true"},
		},
	}).OK(t, nil)
	var detail struct {
		ActiveVersion struct {
			Version int `json:"version"`
		} `json:"activeVersion"`
	}
	wc.GET("/api/v1/controls/"+ctlID).OK(t, &detail)
	if detail.ActiveVersion.Version != 2 {
		t.Fatalf("edit must activate v2, got %d", detail.ActiveVersion.Version)
	}
}

// TestApproval_CRUDAndTemplate: A3 审批实体——创建（模板+超时政策）/读回/编辑。
// park→决策→超时三政策在 W2 workflow 内真验。
func TestApproval_CRUDAndTemplate(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "apf"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// Create 现返裸实体(MD1):data 顶层即 id。
	apfID := wc.POST("/api/v1/approvals", map[string]any{
		"name":            "spend_check",
		"description":     "花钱要批",
		"template":        "approve spending {{ input.amount }}?",
		"allowReason":     true,
		"timeout":         "2d",
		"timeoutBehavior": "reject",
	}).Field(t, "id")
	if !strings.HasPrefix(apfID, "apf_") {
		t.Fatalf("approval id shape: %s", apfID)
	}

	var detail struct {
		ActiveVersion struct {
			Template        string `json:"template"`
			TimeoutBehavior string `json:"timeoutBehavior"`
		} `json:"activeVersion"`
	}
	wc.GET("/api/v1/approvals/"+apfID).OK(t, &detail)
	if !strings.Contains(detail.ActiveVersion.Template, "input.amount") || detail.ActiveVersion.TimeoutBehavior != "reject" {
		t.Fatalf("approval config readback wrong: %+v", detail.ActiveVersion)
	}

	// bad timeout behavior rejects. 坏超时政策拒。
	r := wc.POST("/api/v1/approvals", map[string]any{
		"name": "bad_apf", "template": "x", "timeoutBehavior": "explode",
	})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("bad timeoutBehavior must reject, got %d %s", r.Status, r.Raw)
	}
}
