// mcp_test.go — W3 集成域：MCP server 真装真连真调。
//
// 两条真实链：脚本 stdio server（纯 python JSON-RPC，确定性控制协议边角——进度通知、
// isError、stderr、degraded 翻转）+ 官方 filesystem server（npx 真装 node runtime、真调
// 真文件）。registry / import / reconnect / 调用台账 / 错误路径全覆盖。
package scenarios

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// scriptedMCP is a dependency-free python MCP stdio server: JSON-RPC 2.0, newline-delimited.
// Tools: echo (emits one progress notification when the caller minted a token), boom
// (isError result + a stderr line). A startup banner lands in the stderr ring.
//
// scriptedMCP 是零依赖 python MCP stdio server：JSON-RPC 2.0、按行分帧。工具：echo（调用方
// 铸了 token 就发一条进度通知）、boom（isError 结果 + 一行 stderr）。启动横幅进 stderr ring。
const scriptedMCP = `import sys, json

def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()

print("scripted mcp server starting", file=sys.stderr, flush=True)

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    msg = json.loads(line)
    method = msg.get("method")
    mid = msg.get("id")
    if method == "initialize":
        send({"jsonrpc": "2.0", "id": mid, "result": {
            "protocolVersion": msg["params"]["protocolVersion"],
            "capabilities": {"tools": {}},
            "serverInfo": {"name": "scripted", "version": "1.0.0"}}})
    elif method == "tools/list":
        send({"jsonrpc": "2.0", "id": mid, "result": {"tools": [
            {"name": "echo", "description": "echo text back",
             "inputSchema": {"type": "object", "properties": {"text": {"type": "string"}}, "required": ["text"]}},
            {"name": "boom", "description": "always fails",
             "inputSchema": {"type": "object", "properties": {}}},
        ]}})
    elif method == "tools/call":
        params = msg.get("params") or {}
        name = params.get("name")
        token = (params.get("_meta") or {}).get("progressToken")
        if name == "echo":
            if token is not None:
                send({"jsonrpc": "2.0", "method": "notifications/progress",
                      "params": {"progressToken": token, "progress": 1, "total": 2, "message": "echo halfway"}})
            text = (params.get("arguments") or {}).get("text", "")
            send({"jsonrpc": "2.0", "id": mid, "result": {"content": [{"type": "text", "text": "echo:" + text}]}})
        elif name == "boom":
            print("boom tool exploding on purpose", file=sys.stderr, flush=True)
            send({"jsonrpc": "2.0", "id": mid, "result": {"content": [{"type": "text", "text": "kaboom"}], "isError": True}})
        else:
            send({"jsonrpc": "2.0", "id": mid, "error": {"code": -32602, "message": "unknown tool " + str(name)}})
    elif mid is not None:
        send({"jsonrpc": "2.0", "id": mid, "result": {}})
`

// writeScriptedMCP drops the scripted server into a temp dir and returns its path.
//
// writeScriptedMCP 把脚本 server 落进临时目录并返回路径。
func writeScriptedMCP(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "scripted_mcp.py")
	if err := os.WriteFile(p, []byte(scriptedMCP), 0o644); err != nil {
		t.Fatalf("write scripted mcp: %v", err)
	}
	return p
}

// mcpStatus is the ServerStatus wire shape the scenarios assert on.
//
// mcpStatus 是场景断言用的 ServerStatus 线缆形状。
type mcpStatus struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	LastError string `json:"lastError"`
	Tools     []struct {
		Name        string          `json:"name"`
		InputSchema json.RawMessage `json:"inputSchema"`
	} `json:"tools"`
}

// mcpCallsPage is GET /mcp-servers/{name}/calls.
type mcpCallsPage struct {
	Calls []struct {
		ID          string `json:"id"`
		Tool        string `json:"tool"`
		Status      string `json:"status"`
		TriggeredBy string `json:"triggeredBy"`
		Logs        string `json:"logs"`
	} `json:"calls"`
	Aggregates struct {
		OKCount     int `json:"okCount"`
		FailedCount int `json:"failedCount"`
	} `json:"aggregates"`
}

// TestMCP_ScriptedServerLifecycle: A6 主链——PUT 装 stdio server、tools 缓存、:invoke 真调、
// 进度通知进调用 logs、连续失败翻 degraded、成功回 ready、stderr 尾、reconnect、删干净。
func TestMCP_ScriptedServerLifecycle(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "mcp-lifecycle"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	script := writeScriptedMCP(t)

	// PUT installs + connects synchronously (python runtime rides the harness cache).
	// PUT 同步装+连（python runtime 吃 harness 缓存）。
	var st mcpStatus
	wc.PUT("/api/v1/mcp-servers/scripted", map[string]any{
		"description": "验收脚本 server",
		"command":     "python3",
		"args":        []string{script},
	}).OK(t, &st)
	if st.Status != "ready" || len(st.Tools) != 2 {
		t.Fatalf("want ready with 2 tools, got %s lastError=%q tools=%d", st.Status, st.LastError, len(st.Tools))
	}
	// InputSchema passes through verbatim (we never invent schemas). InputSchema 原样透传。
	for _, tool := range st.Tools {
		if tool.Name == "echo" && !strings.Contains(string(tool.InputSchema), `"required"`) {
			t.Fatalf("echo schema must pass through verbatim, got %s", tool.InputSchema)
		}
	}

	// invoke echo → result; the progress notification must land in the call's logs.
	// 调 echo → 结果；进度通知必须落进该次调用的 logs。
	var inv struct {
		Result string `json:"result"`
	}
	wc.POST("/api/v1/mcp-servers/scripted/tools/echo:invoke", map[string]any{
		"args": map[string]any{"text": "hello"},
	}).OK(t, &inv)
	if inv.Result != "echo:hello" {
		t.Fatalf("echo result wrong: %q", inv.Result)
	}

	var page mcpCallsPage
	wc.GET("/api/v1/mcp-servers/scripted/calls").OK(t, &page)
	if len(page.Calls) != 1 || page.Calls[0].Status != "ok" || page.Calls[0].TriggeredBy != "manual" {
		t.Fatalf("ledger after echo wrong: %+v", page)
	}
	if page.Calls[0].Logs != "" {
		t.Fatal("list rows must omit logs")
	}
	var detail struct {
		Logs string `json:"logs"`
	}
	wc.GET("/api/v1/mcp-calls/"+page.Calls[0].ID).OK(t, &detail)
	if !strings.Contains(detail.Logs, "echo halfway") {
		t.Fatalf("progress notification must land in call logs, got %q", detail.Logs)
	}

	// 3 consecutive failures flip ready → degraded; degraded still serves; one success → ready.
	// 连续 3 失败翻 degraded；degraded 仍可服务；一次成功回 ready。
	for i := 0; i < 3; i++ {
		wc.Do("POST", "/api/v1/mcp-servers/scripted/tools/boom:invoke", map[string]any{}).
			Fail(t, 502, "MCP_RPC_ERROR")
	}
	wc.GET("/api/v1/mcp-servers/scripted").OK(t, &st)
	if st.Status != "degraded" {
		t.Fatalf("3 failures must degrade, got %s", st.Status)
	}
	wc.POST("/api/v1/mcp-servers/scripted/tools/echo:invoke", map[string]any{
		"args": map[string]any{"text": "revive"},
	}).OK(t, nil)
	wc.GET("/api/v1/mcp-servers/scripted").OK(t, &st)
	if st.Status != "ready" {
		t.Fatalf("success must restore ready, got %s", st.Status)
	}

	// Failed-call detail carries the server stderr tail; ?status filter + aggregates add up.
	// 失败调用详情带 server stderr 尾；?status 过滤 + 聚合对账。
	wc.GET("/api/v1/mcp-servers/scripted/calls?status=failed").OK(t, &page)
	if len(page.Calls) != 3 || page.Aggregates.OKCount != 2 || page.Aggregates.FailedCount != 3 {
		t.Fatalf("failed filter/aggregates wrong: n=%d agg=%+v", len(page.Calls), page.Aggregates)
	}
	wc.GET("/api/v1/mcp-calls/"+page.Calls[0].ID).OK(t, &detail)
	if !strings.Contains(detail.Logs, "server stderr tail") || !strings.Contains(detail.Logs, "boom tool exploding") {
		t.Fatalf("failed call logs must append stderr tail, got %q", detail.Logs)
	}

	// stderr surface + reconnect (fresh process → banner again) + delete.
	// stderr 面 + reconnect（新进程 → 横幅再现）+ 删除。
	var serr struct {
		Stderr string `json:"stderr"`
	}
	wc.GET("/api/v1/mcp-servers/scripted/stderr").OK(t, &serr)
	if !strings.Contains(serr.Stderr, "scripted mcp server starting") {
		t.Fatalf("stderr ring must hold the banner, got %q", serr.Stderr)
	}
	wc.POST("/api/v1/mcp-servers/scripted:reconnect", nil).OK(t, &st)
	if st.Status != "ready" {
		t.Fatalf("reconnect must restore ready, got %s lastError=%q", st.Status, st.LastError)
	}

	wc.DELETE("/api/v1/mcp-servers/scripted")
	wc.Do("GET", "/api/v1/mcp-servers/scripted", nil).Fail(t, 404, "MCP_SERVER_NOT_FOUND")
	wc.Do("POST", "/api/v1/mcp-servers/scripted/tools/echo:invoke", map[string]any{}).
		Fail(t, 404, "MCP_SERVER_NOT_FOUND")
}

// TestMCP_ErrorPaths: A6 出错列——未知工具、坏 command 连不上仍留 server（reconnect 可救语义）、
// 不可达 remote、未知 action。
func TestMCP_ErrorPaths(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "mcp-errors"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	script := writeScriptedMCP(t)

	var st mcpStatus
	wc.PUT("/api/v1/mcp-servers/errsrv", map[string]any{
		"command": "python3", "args": []string{script},
	}).OK(t, &st)

	// unknown tool on a ready server → RPC error, not a 500.
	// ready server 上调未知工具 → RPC 错误、不是 500。
	wc.Do("POST", "/api/v1/mcp-servers/errsrv/tools/nope:invoke", map[string]any{}).
		Fail(t, 502, "MCP_RPC_ERROR")

	// broken stdio command: PUT persists the server with status=failed (reconnect-recoverable),
	// not a transactional reject — install must survive a flaky first connect.
	// 坏 stdio command：PUT 以 status=failed 留住 server（reconnect 可救）、非事务性拒绝——
	// 安装必须扛住首连失败。
	wc.PUT("/api/v1/mcp-servers/deadsrv", map[string]any{
		"command": "python3", "args": []string{"/nonexistent/mcp_server.py"},
	}).OK(t, &st)
	if st.Status != "failed" || st.LastError == "" {
		t.Fatalf("dead command must persist as failed+lastError, got %s %q", st.Status, st.LastError)
	}
	wc.POST("/api/v1/mcp-servers/deadsrv:reconnect", nil).OK(t, &st)
	if st.Status != "failed" {
		t.Fatalf("reconnect on a dead command stays failed, got %s", st.Status)
	}
	wc.Do("POST", "/api/v1/mcp-servers/deadsrv/tools/x:invoke", map[string]any{}).
		Fail(t, 503, "MCP_SERVER_DOWN")

	// unreachable remote → same persist-as-failed semantics. 不可达 remote → 同语义。
	wc.PUT("/api/v1/mcp-servers/deadremote", map[string]any{
		"url": "http://127.0.0.1:9/mcp",
	}).OK(t, &st)
	if st.Status != "failed" {
		t.Fatalf("unreachable remote must be failed, got %s", st.Status)
	}

	// unknown :action. 未知 action。
	wc.Do("POST", "/api/v1/mcp-servers/errsrv:explode", nil).Fail(t, 400, "INVALID_REQUEST")
}

// TestMCP_ImportAndRegistry: A6 安装路径——Claude Desktop mcp.json 导入（skip/overwrite 语义）+
// 市场浏览 + 安装报错面（未知条目 / 缺必填 env 先于任何下载报出）。
func TestMCP_ImportAndRegistry(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "mcp-install"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	script := writeScriptedMCP(t)

	// import: connects + persists; same-name skipped unless ?overwrite=true.
	// 导入：连接+落盘；同名默认跳过、?overwrite=true 覆盖。
	frag := map[string]any{"mcpServers": map[string]any{
		"imported": map[string]any{"command": "python3", "args": []string{script}},
	}}
	var imp struct {
		Imported []string `json:"imported"`
		Skipped  []string `json:"skipped"`
	}
	wc.POST("/api/v1/mcp-servers:import", frag).OK(t, &imp)
	if len(imp.Imported) != 1 || imp.Imported[0] != "imported" {
		t.Fatalf("import wrong: %+v", imp)
	}
	var st mcpStatus
	wc.GET("/api/v1/mcp-servers/imported").OK(t, &st)
	if st.Status != "ready" {
		t.Fatalf("imported server must connect, got %s lastError=%q", st.Status, st.LastError)
	}
	wc.POST("/api/v1/mcp-servers:import", frag).OK(t, &imp)
	if len(imp.Skipped) != 1 {
		t.Fatalf("re-import without overwrite must skip, got %+v", imp)
	}
	wc.POST("/api/v1/mcp-servers:import?overwrite=true", frag).OK(t, &imp)
	if len(imp.Imported) != 1 {
		t.Fatalf("overwrite re-import must import, got %+v", imp)
	}

	// marketplace list is global + curated; every entry renders name/description.
	// 市场列表全局 curated；每条有 name/description。
	var entries []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	wc.GET("/api/v1/mcp-registry").OK(t, &entries)
	if len(entries) < 10 {
		t.Fatalf("curated registry suspiciously small: %d", len(entries))
	}
	for _, e := range entries {
		if e.Name == "" || e.Description == "" {
			t.Fatalf("registry entry missing name/description: %+v", e)
		}
	}

	// install error surface: unknown entry; required env enforced BEFORE any download.
	// 安装报错面：未知条目；必填 env 在任何下载前强制。
	wc.Do("POST", "/api/v1/mcp-registry:install", map[string]any{"name": "definitely/not-a-server"}).
		Fail(t, 404, "MCP_REGISTRY_NOT_FOUND")
	r := wc.Do("POST", "/api/v1/mcp-registry:install", map[string]any{"name": "firecrawl/firecrawl-mcp-server"})
	if r.Status != 422 || r.Code != "MCP_ENV_MISSING" {
		t.Fatalf("env-gated entry must 422 MCP_ENV_MISSING without keys, got %d/%s %s", r.Status, r.Code, r.Raw)
	}
}

// TestMCP_OfficialFilesystemServer: A6 官方真货——npx 装 @modelcontextprotocol/server-filesystem
// （首跑真下 node runtime）、真读真文件、台账记账。整链与 Claude Desktop 用户体验同构。
func TestMCP_OfficialFilesystemServer(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "mcp-fs"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// macOS 的 /var 是 /private/var 符号链——两侧都用真实路径，免得 allowed-dir 校验歧义。
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve tempdir: %v", err)
	}
	proof := "mcp-fs-proof-" + filepath.Base(dir)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(proof), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	var st mcpStatus
	wc.PUT("/api/v1/mcp-servers/fs", map[string]any{
		"description": "official filesystem server",
		"command":     "npx",
		"args":        []string{"-y", "@modelcontextprotocol/server-filesystem", dir},
	}).OK(t, &st)
	if st.Status != "ready" || len(st.Tools) == 0 {
		t.Fatalf("filesystem server must connect with tools, got %s lastError=%q", st.Status, st.LastError)
	}

	// Pick the read tool by what tools/list actually advertises (name drifted across versions).
	// 按 tools/list 实际广告挑读工具（不同版本名字有漂移）。
	readTool := ""
	for _, tool := range st.Tools {
		if tool.Name == "read_text_file" {
			readTool = tool.Name
			break
		}
		if tool.Name == "read_file" {
			readTool = tool.Name
		}
	}
	if readTool == "" {
		names := make([]string, 0, len(st.Tools))
		for _, tool := range st.Tools {
			names = append(names, tool.Name)
		}
		t.Fatalf("no read tool advertised; tools=%v", names)
	}

	var inv struct {
		Result string `json:"result"`
	}
	wc.POST(fmt.Sprintf("/api/v1/mcp-servers/fs/tools/%s:invoke", readTool), map[string]any{
		"args": map[string]any{"path": filepath.Join(dir, "hello.txt")},
	}).OK(t, &inv)
	if !strings.Contains(inv.Result, proof) {
		t.Fatalf("read via mcp must return the file content, got %q", inv.Result)
	}

	var page mcpCallsPage
	wc.GET("/api/v1/mcp-servers/fs/calls").OK(t, &page)
	if page.Aggregates.OKCount != 1 {
		t.Fatalf("fs call must be in the ledger: %+v", page.Aggregates)
	}
}
