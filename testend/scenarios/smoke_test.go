// Package scenarios is the acceptance ledger: every test function is one row of the
// feature × situation matrix (PLAN.md), run against the REAL backend binary over pure
// HTTP — what passes here is what a user/frontend actually gets.
//
// Package scenarios 是验收台账：每个测试函数是 feature × 情况矩阵（PLAN.md）的一行，
// 打在**真实** backend 二进制的纯 HTTP 面上——这里过了的才是用户/前端真正拿到的。
package scenarios

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// TestSmoke_BootToSearchableEntity: the spine every wave stands on — boot the real
// binary, create a workspace, forge a function over HTTP, and see it ripple into search.
//
// TestSmoke_BootToSearchableEntity：所有波次站立的脊柱——拉起真二进制、建 workspace、
// 经 HTTP 锻造一个 function、看它涟漪进搜索。
func TestSmoke_BootToSearchableEntity(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)

	// workspace lifecycle entry. workspace 生命周期入口。
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "验收冒烟"}).OK(t, nil)
	wsID := ws.Field(t, "id")
	if !strings.HasPrefix(wsID, "ws_") {
		t.Fatalf("workspace id shape: %s", wsID)
	}
	wc := c.WS(wsID)

	// forge a function via the flat HTTP payload (buildOpsFromDirect path).
	// 经扁平 HTTP payload 锻造 function（buildOpsFromDirect 路径）。
	// create 现返裸实体(MD1):data 顶层即 id。
	fnID := wc.POST("/api/v1/functions", map[string]any{
		"name":        "greet_user",
		"description": "向用户问好的冒烟函数",
		"code":        "def greet(name: str) -> dict:\n    print(\"smoke print line\")\n    return {\"msg\": f\"hello {name}\"}\n",
	}).Field(t, "id")
	if fnID == "" {
		t.Fatal("create returned no function.id")
	}

	// product logic: v1 exists and is active. 产品逻辑：v1 存在且 active。
	var detail struct {
		ActiveVersionID string `json:"activeVersionId"`
	}
	wc.GET("/api/v1/functions/"+fnID).OK(t, &detail)
	if detail.ActiveVersionID == "" {
		t.Fatal("create must activate v1")
	}

	// ripple: the new entity becomes searchable (async indexer). 涟漪：新实体可被搜到（异步索引）。
	harness.Eventually(t, 5000, "function appears in omni search", func() bool {
		var page struct {
			Hits []struct {
				EntityID string `json:"entityId"`
			} `json:"hits"`
		}
		r := wc.GET("/api/v1/search?q=greet_user")
		if r.Status != 200 {
			return false
		}
		if err := json.Unmarshal(r.Data, &page); err != nil {
			return false
		}
		for _, h := range page.Hits {
			if h.EntityID == fnID {
				return true
			}
		}
		return false
	})

	// envelope error path: empty q rejects with the domain code. 信封错误路径：空 q 按域码拒。
	wc.GET("/api/v1/search?q=").Fail(t, 400, "SEARCH_QUERY_REQUIRED")

	// workspace isolation: a second workspace sees nothing. 隔离：第二个 workspace 看不见。
	ws2 := c.POST("/api/v1/workspaces", map[string]any{"name": "隔离对照"}).OK(t, nil)
	wc2 := c.WS(ws2.Field(t, "id"))
	var page2 struct {
		Total int `json:"total"`
	}
	wc2.GET("/api/v1/search?q=greet_user").OK(t, &page2)
	if page2.Total != 0 {
		t.Fatalf("workspace isolation broken: total=%d", page2.Total)
	}
}
