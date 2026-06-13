// platform_test.go — W5 平台域 A9 + 涟漪矩阵 A10。
//
// workspace（CRUD/校验/activate/最后一个拒删/删除级联）、apikey（探活三态/被引用拒删/创建校验）、
// model（scenarios/capabilities/默认模型校验）、limits（每字段 PATCH→运行时热换真生效，含 promptdump
// 验 tool_result 上限）、notification（生命周期事件→list/未读/已读流转）、sandbox（bootstrap/runtimes/
// disk-usage/envs ownerKind 守卫/销毁）、relation（equip 边 + 改名读侧名字跟随 + 删除清边——A10 涟漪）。
//
// 三列判定：用户面（HTTP 语义 + wire code）/ 产品逻辑（级联/记账/状态流转）/ LLM 面（limits 热换经
// promptdump 实证模型真看到被截断的 tool_result）。
package scenarios

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// ── workspace ───────────────────────────────────────────────────────────────

// TestPlatform_WorkspaceLifecycle: CRUD + 校验面 + activate + 最后一个拒删。
func TestPlatform_WorkspaceLifecycle(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t) // workspace CRUD 豁免 workspace 头（onboarding 在选定前运行）

	// 创建（扁平形——AC-1 定性：workspace 无版本故扁平，与版本实体的 {entity,version} 嵌套形并存）。
	var created struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		WebFetchMode string `json:"webFetchMode"`
	}
	c.POST("/api/v1/workspaces", map[string]any{"name": "platform-ws"}).OK(t, &created)
	if created.ID == "" || created.Name != "platform-ws" {
		t.Fatalf("create returned %+v", created)
	}

	// 校验面：空名 / 重名 / 非法语言 / 非法 webFetchMode 各自精确 wire code。
	c.POST("/api/v1/workspaces", map[string]any{"name": ""}).Fail(t, 400, "WORKSPACE_NAME_REQUIRED")
	c.POST("/api/v1/workspaces", map[string]any{"name": "platform-ws"}).Fail(t, 409, "WORKSPACE_NAME_CONFLICT")
	c.POST("/api/v1/workspaces", map[string]any{"name": "bad-lang", "language": "klingon"}).
		Fail(t, 400, "WORKSPACE_LANGUAGE_INVALID")
	c.PATCH("/api/v1/workspaces/"+created.ID, map[string]any{"webFetchMode": "telepathy"}).
		Fail(t, 400, "WORKSPACE_WEB_FETCH_MODE_INVALID")

	// 合法更新生效：webFetchMode jina 回显。
	var updated struct {
		WebFetchMode string `json:"webFetchMode"`
	}
	c.PATCH("/api/v1/workspaces/"+created.ID, map[string]any{"webFetchMode": "jina"}).OK(t, &updated)
	if updated.WebFetchMode != "jina" {
		t.Fatalf("webFetchMode not applied: %q", updated.WebFetchMode)
	}

	// :activate 刷 lastUsedAt（切换语义）。
	var activated struct {
		LastUsedAt string `json:"lastUsedAt"`
	}
	c.POST("/api/v1/workspaces/"+created.ID+":activate", nil).OK(t, &activated)
	if activated.LastUsedAt == "" {
		t.Fatal(":activate did not set lastUsedAt")
	}

	// 最后一个拒删：把现存全删到只剩 1 个，最后一删必撞 CANNOT_DELETE_LAST_WORKSPACE。
	var all []struct {
		ID string `json:"id"`
	}
	c.GET("/api/v1/workspaces").OK(t, &all)
	guardHit := false
	for i, ws := range all {
		r := c.Do("DELETE", "/api/v1/workspaces/"+ws.ID, nil)
		if r.Status == 422 && r.Code == "CANNOT_DELETE_LAST_WORKSPACE" {
			guardHit = true
			if i != len(all)-1 {
				t.Fatalf("last-workspace guard fired early at #%d of %d", i, len(all))
			}
			break
		}
		if r.Status != 204 {
			t.Fatalf("delete ws %s: want 204, got %d/%s", ws.ID, r.Status, r.Code)
		}
	}
	if !guardHit {
		t.Fatal("CANNOT_DELETE_LAST_WORKSPACE never fired — last workspace was deletable")
	}
}

// TestPlatform_WorkspaceCascadeDelete: 删除级联——含 function（落盘 env）+ 对话的 workspace
// 被删后 204 收口（Reaper 跑完、不挂死），ws 行消失（404），其下数据不可达。
func TestPlatform_WorkspaceCascadeDelete(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	// A fresh data dir boots with zero workspaces (onboarding creates the first), so keep a
	// second one alive — else the cascade target is "the last workspace" and the guard fires.
	c.POST("/api/v1/workspaces", map[string]any{"name": "keeper"}).OK(t, nil)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "doomed"}).Field(t, "id")
	wc := c.WS(wsID)

	// 在该 ws 里堆资产：一个 function（创建即同步物化 env 落盘）+ 一个对话。
	fnID := fnCreate(t, wc, "doomed_fn", "def f() -> dict:\n    return {\"ok\": True}\n")
	convID := convCreate(t, wc, "doomed conv")
	wc.GET("/api/v1/functions/" + fnID).OK(t, nil) // 删前确在

	// 删除整 workspace：必须及时 204（Reaper 摘监听/停进程/删盘/删行全跑完，不死锁）。
	c.Do("DELETE", "/api/v1/workspaces/"+wsID, nil).OK(t, nil)

	// ws 行消失。
	c.Do("GET", "/api/v1/workspaces/"+wsID, nil).Fail(t, 404, "WORKSPACE_NOT_FOUND")

	// 其下数据不可达：带已删 ws 头取 function/对话不再 200（隔离根没了）。
	if r := wc.Do("GET", "/api/v1/functions/"+fnID, nil); r.Status == 200 {
		t.Fatalf("function still reachable after workspace delete: %s", r.Raw)
	}
	if r := wc.Do("GET", "/api/v1/conversations/"+convID+"/messages", nil); r.Status == 200 {
		t.Fatalf("conversation still reachable after workspace delete: %s", r.Raw)
	}
}

// ── apikey ──────────────────────────────────────────────────────────────────

// TestPlatform_APIKeyProbeAndGuards: 创建校验 + :test 探活两态 + 被引用拒删 + 重名冲突。
func TestPlatform_APIKeyProbeAndGuards(t *testing.T) {
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "key-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	// 创建校验：未知 provider / 空 key 各自精确码。
	wc.POST("/api/v1/api-keys", map[string]any{"provider": "nonesuch", "displayName": "x", "key": "k"}).
		Fail(t, 400, "API_KEY_INVALID_PROVIDER")
	wc.POST("/api/v1/api-keys", map[string]any{"provider": "openai", "displayName": "x", "key": ""}).
		Fail(t, 400, "API_KEY_VALUE_REQUIRED")

	// 真创建（指向 mock）。
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "live", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")

	// 重名冲突。
	wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "live", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Fail(t, 409, "API_KEY_DISPLAY_NAME_CONFLICT")

	// :test 探活——活 key 200 {ok:true, latencyMs}。
	var probe struct {
		OK        bool `json:"ok"`
		LatencyMs int  `json:"latencyMs"`
	}
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, &probe)
	if !probe.OK {
		t.Fatal(":test on live key returned ok=false")
	}

	// 死 baseUrl 的 key → :test 422 API_KEY_TEST_FAILED。
	deadID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "dead", "key": "sk-x", "baseUrl": "http://127.0.0.1:1",
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+deadID+":test", nil).Fail(t, 422, "API_KEY_TEST_FAILED")

	// 被引用拒删——三个引用来源逐个验（RefScanner 三分支：workspace 默认模型 / 默认搜索 key /
	// agent modelOverride）。缺接线时 API_KEY_IN_USE 永不触发（AC-21）。
	// ① workspace dialogue 默认模型。
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": keyID, "modelId": "gpt-4o"}).OK(t, nil)
	wc.Do("DELETE", "/api/v1/api-keys/"+keyID, nil).Fail(t, 422, "API_KEY_IN_USE")

	// ② workspace 默认搜索 key。
	searchKey := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "search", "key": "sk-s", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-search", map[string]any{"apiKeyId": searchKey}).OK(t, nil)
	wc.Do("DELETE", "/api/v1/api-keys/"+searchKey, nil).Fail(t, 422, "API_KEY_IN_USE")

	// ③ agent modelOverride 钉死某 key。
	ovKey := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "override", "key": "sk-o", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/agents", map[string]any{
		"name": "ov_agent", "description": "验收用", "prompt": "x",
		"modelOverride": map[string]any{"apiKeyId": ovKey, "modelId": "gpt-4o"},
	}).OK(t, nil)
	wc.Do("DELETE", "/api/v1/api-keys/"+ovKey, nil).Fail(t, 422, "API_KEY_IN_USE")

	// 未被引用的 dead key 可删（守卫不误伤无引用 key）。
	wc.Do("DELETE", "/api/v1/api-keys/"+deadID, nil).OK(t, nil)
}

// ── model ───────────────────────────────────────────────────────────────────

// TestPlatform_ModelConfig: scenarios 白名单 + 默认模型校验 + capabilities 经探测聚合。
func TestPlatform_ModelConfig(t *testing.T) {
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "model-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	// 固定 scenario 白名单。
	var scenarios []struct {
		Name string `json:"name"`
	}
	wc.GET("/api/v1/scenarios").OK(t, &scenarios)
	got := map[string]bool{}
	for _, s := range scenarios {
		got[s.Name] = true
	}
	for _, want := range []string{"dialogue", "utility", "agent"} {
		if !got[want] {
			t.Fatalf("scenario %q missing from %+v", want, scenarios)
		}
	}

	// 默认模型校验：非法 scenario / 残缺 ref。
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/wizard",
		map[string]any{"apiKeyId": "key_x", "modelId": "m"}).Fail(t, 400, "MODEL_SCENARIO_INVALID")
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": "", "modelId": ""}).Fail(t, 400, "MODEL_REF_INVALID")

	// capabilities 经 apikey 探测档案聚合：未探测 → 空；探测后 mock 的模型目录现身。
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "caps", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	caps := wc.GET("/api/v1/model-capabilities")
	caps.OK(t, nil)
	if !strings.Contains(string(caps.Raw), "gpt-4o") {
		t.Fatalf("model-capabilities did not surface probed model gpt-4o: %s", caps.Raw)
	}
}

// ── limits（运行时热换真生效）───────────────────────────────────────────────

type wireLimits struct {
	Agent struct {
		MaxSteps int `json:"maxSteps"`
	} `json:"agent"`
	Tools struct {
		ToolResultCapKB int `json:"toolResultCapKB"`
	} `json:"tools"`
}

// TestPlatform_LimitsHotSwap: PATCH /limits 持久化并热换——消费方下次读取即见新值。
// 三证：① 非法值 400；② maxSteps=2 真让 ReAct 循环 2 步即触顶；③ toolResultCapKB=1 经 promptdump
// 实证模型真收到被截断的 tool_result（loop 消费方读活动上限）。
func TestPlatform_LimitsHotSwap(t *testing.T) {
	wc, mock := chatSetup(t, false)

	// ① 非法 triggerRatio（须 0<r<1）→ 400 SETTINGS_LIMITS_INVALID。
	wc.PATCH("/api/v1/limits", map[string]any{"context": map[string]any{"triggerRatio": 5}}).
		Fail(t, 400, "SETTINGS_LIMITS_INVALID")

	// ② maxSteps=2：PATCH→GET 回读为 2（持久化），再驱动一个会无限点工具的循环 → 2 步触顶。
	wc.PATCH("/api/v1/limits", map[string]any{"agent": map[string]any{"maxSteps": 2}}).OK(t, nil)
	var lim wireLimits
	wc.GET("/api/v1/limits").OK(t, &lim)
	if lim.Agent.MaxSteps != 2 {
		t.Fatalf("maxSteps hot-swap not persisted: %d", lim.Agent.MaxSteps)
	}
	loopFn := fnCreate(t, wc, "loop_fn", "def f() -> dict:\n    return {\"again\": True}\n")
	for range 6 { // 排 6 个工具回合——若未受限会一直跑
		mock.Enqueue(dlgModel, harness.LLMTurn{
			ToolCalls: []harness.MockToolCall{{Name: "run_function", Args: map[string]any{
				"functionId": loopFn, "args": map[string]any{},
				"summary": "loop", "danger": "safe", "execution_group": 1,
			}}},
		})
	}
	before := len(mock.DumpsFor(dlgModel))
	conv1 := convCreate(t, wc, "maxsteps")
	msg1 := sendMsg(t, wc, conv1, "go")
	turn1 := waitTurn(t, wc, conv1, msg1, 15000)
	if turn1.ErrorCode != "MAX_STEPS_REACHED" {
		t.Fatalf("want MAX_STEPS_REACHED, got errorCode=%q status=%q", turn1.ErrorCode, turn1.Status)
	}
	steps := len(mock.DumpsFor(dlgModel)) - before
	if steps < 2 || steps > 3 { // 2 步（+至多 1 容差）；远小于排的 6，证明上限生效
		t.Fatalf("maxSteps=2 not enforced: %d dialogue requests (enqueued 6)", steps)
	}
	mock.Clear(dlgModel)

	// ③ toolResultCapKB=1：函数返回 ~5KB，回喂模型的 tool_result 必被截到 ~1KB。
	wc.PATCH("/api/v1/limits", map[string]any{"tools": map[string]any{"toolResultCapKB": 1}}).OK(t, nil)
	bigFn := fnCreate(t, wc, "big_fn", "def f() -> dict:\n    return {\"blob\": \"x\" * 5000}\n")
	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "run_function", Args: map[string]any{
			"functionId": bigFn, "args": map[string]any{},
			"summary": "big", "danger": "safe", "execution_group": 1,
		}}}},
		harness.LLMTurn{Text: "done."},
	)
	base := len(mock.DumpsFor(dlgModel))
	conv2 := convCreate(t, wc, "toolcap")
	msg2 := sendMsg(t, wc, conv2, "go")
	waitTurn(t, wc, conv2, msg2, 15000)
	ds := mock.DumpsFor(dlgModel)
	if len(ds)-base < 2 {
		t.Fatalf("expected 2 dialogue requests for tool roundtrip, got %d", len(ds)-base)
	}
	req2 := ds[base+1] // 回喂请求：带 tool 结果
	var toolContent string
	for _, m := range req2.Messages {
		if m.Role == "tool" {
			toolContent = m.Content
		}
	}
	if toolContent == "" {
		t.Fatalf("no tool message in follow-up request: %+v", req2.Messages)
	}
	if len(toolContent) > 1500 { // 1KB 上限 + 截断标记容差；未受限会是 ~5KB
		t.Fatalf("tool_result not capped at 1KB: %d bytes fed to model", len(toolContent))
	}
}

// ── notification ─────────────────────────────────────────────────────────────

// TestPlatform_NotificationFlow: 生命周期事件真落通知中心 → list / 未读计数 / 标已读 / 全标已读。
func TestPlatform_NotificationFlow(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "notif-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	// 建 function → 发 function.created；异步落库，轮询到达。
	fnCreate(t, wc, "notif_fn", "def f() -> dict:\n    return {}\n")

	type notif struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	var firstID string
	harness.Eventually(t, 6000, "function.created notification lands", func() bool {
		var items []notif
		wc.GET("/api/v1/notifications").OK(t, &items)
		for _, n := range items {
			if n.Type == "function.created" {
				firstID = n.ID
				return true
			}
		}
		return false
	})

	// 未读计数 ≥1。
	var unread struct {
		Unread int `json:"unread"`
	}
	wc.GET("/api/v1/notifications/unread-count").OK(t, &unread)
	if unread.Unread < 1 {
		t.Fatalf("unread count %d after a fresh notification", unread.Unread)
	}

	// 标该条已读 → 未读减少。
	wc.POST("/api/v1/notifications/"+firstID+":mark-read", nil).OK(t, nil) // :action(MD5)
	var afterOne struct {
		Unread int `json:"unread"`
	}
	wc.GET("/api/v1/notifications/unread-count").OK(t, &afterOne)
	if afterOne.Unread >= unread.Unread {
		t.Fatalf("mark-read did not lower unread: %d → %d", unread.Unread, afterOne.Unread)
	}

	// 全标已读 → 0。
	wc.POST("/api/v1/notifications:mark-all-read", nil).OK(t, nil) // 集合级 :action(MD5)
	var afterAll struct {
		Unread int `json:"unread"`
	}
	wc.GET("/api/v1/notifications/unread-count").OK(t, &afterAll)
	if afterAll.Unread != 0 {
		t.Fatalf("read-all left %d unread", afterAll.Unread)
	}
}

// ── sandbox ──────────────────────────────────────────────────────────────────

// TestPlatform_SandboxGovernance: bootstrap-status / runtimes / disk-usage / envs ownerKind 守卫 /
// 单 env 销毁。
func TestPlatform_SandboxGovernance(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "sbx-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	// bootstrap-status：{ok:bool}。
	var boot struct {
		OK bool `json:"ok"`
	}
	wc.GET("/api/v1/sandbox/bootstrap-status").OK(t, &boot)

	// runtimes 列表（缓存预置后非空）。
	wc.GET("/api/v1/sandbox/runtimes").OK(t, nil)

	// 建 function 物化一个 env（落盘）→ disk-usage > 0。
	fnCreate(t, wc, "sbx_fn", "def f() -> dict:\n    return {}\n")
	var disk struct {
		TotalBytes int64 `json:"totalBytes"`
	}
	wc.GET("/api/v1/sandbox/disk-usage").OK(t, &disk)
	if disk.TotalBytes <= 0 {
		t.Fatalf("disk-usage totalBytes=%d after materializing an env", disk.TotalBytes)
	}

	// envs ownerKind 守卫：缺失 → 400；非法 → 400（空 list 永不被误读为「没数据」）。
	wc.Do("GET", "/api/v1/sandbox/envs", nil).Fail(t, 400, "SANDBOX_OWNER_KIND_REQUIRED")
	wc.Do("GET", "/api/v1/sandbox/envs?ownerKind=wizard", nil).Fail(t, 400, "SANDBOX_INVALID_OWNER_KIND")

	// function env 列出 → 取其一销毁 → 204。
	var envs []struct {
		ID string `json:"id"`
	}
	wc.GET("/api/v1/sandbox/envs?ownerKind=function").OK(t, &envs)
	if len(envs) == 0 {
		t.Fatal("no function env listed after materialization")
	}
	wc.Do("DELETE", "/api/v1/sandbox/envs/"+envs[0].ID, nil).OK(t, nil)
}

// ── relation（A10 涟漪：equip 边 + 读侧名字跟随 + 删除清边）─────────────────────

// TestPlatform_RelationRipple: agent 挂载 function → neighborhood 现 equip 边（toName 读时 hydrate）；
// 改 function 名 → 边的 toName 自动跟随（图存 id、名读时取）；删 function → PurgeEntity 清边。
func TestPlatform_RelationRipple(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "rel-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	fnID := fnCreate(t, wc, "rel_target_old", "def f() -> dict:\n    return {}\n")

	// agent 挂载该 function（equip 出边）。create 现返裸实体(MD1):data 顶层即 id。
	agID := wc.POST("/api/v1/agents", map[string]any{
		"name": "rel_agent", "description": "验收用",
		"prompt": "you mount a function",
		"tools":  []map[string]any{{"ref": fnID, "name": "rel_target_old"}},
	}).Field(t, "id")

	neigh := func() string {
		r := wc.GET(fmt.Sprintf("/api/v1/relations/neighborhood?kind=agent&id=%s&depth=1", agID))
		r.OK(t, nil)
		return string(r.Raw)
	}

	// equip 边在场，toName=旧名（hydrate）。
	harness.Eventually(t, 5000, "equip edge appears with old name", func() bool {
		raw := neigh()
		return strings.Contains(raw, fnID) && strings.Contains(raw, "rel_target_old") &&
			strings.Contains(raw, "equip")
	})

	// 改 function 名 → 读侧 toName 跟随（无需改图：Namers 读时取）。
	wc.PATCH("/api/v1/functions/"+fnID, map[string]any{"name": "rel_target_new"}).OK(t, nil)
	harness.Eventually(t, 5000, "edge toName follows rename", func() bool {
		raw := neigh()
		return strings.Contains(raw, "rel_target_new") && !strings.Contains(raw, "rel_target_old")
	})

	// relgraph 全景含该 agent。
	rg := wc.GET("/api/v1/relgraph")
	rg.OK(t, nil)
	if !strings.Contains(string(rg.Raw), agID) {
		t.Fatalf("relgraph snapshot missing agent %s", agID)
	}

	// 删 function → PurgeEntity 清边 → neighborhood 不再含 fnID。
	wc.Do("DELETE", "/api/v1/functions/"+fnID, nil).OK(t, nil)
	harness.Eventually(t, 5000, "equip edge purged on entity delete", func() bool {
		return !strings.Contains(neigh(), fnID)
	})
}
