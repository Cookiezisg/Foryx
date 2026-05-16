package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)


func TestSearchMCP_Identity(t *testing.T) {
	tt := &SearchMCP{}
	if tt.Name() != "search_mcp_tools" {
		t.Errorf("Name() = %q", tt.Name())
	}
	if tt.Description() == "" {
		t.Error("Description empty")
	}
	if len(tt.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
}

func TestSearchMCP_StaticMetadata(t *testing.T) {
	tt := &SearchMCP{}
	if !tt.IsReadOnly() {
		t.Error("search_mcp should be IsReadOnly=true (discovery only)")
	}
	if tt.NeedsReadFirst() {
		t.Error("NeedsReadFirst should be false")
	}
	if tt.RequiresWorkspace() {
		t.Error("RequiresWorkspace should be false")
	}
}

func TestSearchMCP_Schema_RequiresQuery(t *testing.T) {
	var schema map[string]any
	_ = json.Unmarshal((&SearchMCP{}).Parameters(), &schema)
	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("required = %v, want [query]", required)
	}
}


func TestSearchMCP_ValidateInput_Happy(t *testing.T) {
	if err := (&SearchMCP{}).ValidateInput(json.RawMessage(`{"query":"github pr"}`)); err != nil {
		t.Errorf("happy: %v", err)
	}
}

func TestSearchMCP_ValidateInput_EmptyQuery(t *testing.T) {
	err := (&SearchMCP{}).ValidateInput(json.RawMessage(`{"query":""}`))
	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("err = %v, want ErrEmptyQuery", err)
	}
}

func TestSearchMCP_ValidateInput_WhitespaceQuery(t *testing.T) {
	err := (&SearchMCP{}).ValidateInput(json.RawMessage(`{"query":"   \t\n  "}`))
	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("err = %v, want ErrEmptyQuery", err)
	}
}

func TestSearchMCP_ValidateInput_MalformedJSON(t *testing.T) {
	err := (&SearchMCP{}).ValidateInput(json.RawMessage(`not-json`))
	if err == nil {
		t.Error("malformed JSON should error")
	}
}

func TestSearchMCP_CheckPermissions_Allow(t *testing.T) {
	for _, mode := range []toolapp.PermissionMode{
		toolapp.PermissionModeDefault,
		toolapp.PermissionModeAcceptEdits,
		toolapp.PermissionModePlan,
	} {
		if got := (&SearchMCP{}).CheckPermissions(json.RawMessage(`{}`), mode); got != toolapp.PermissionAllow {
			t.Errorf("mode %v: got %v, want PermissionAllow", mode, got)
		}
	}
}


func TestSearchMCP_Execute_MalformedArgsJSON(t *testing.T) {
	tt := &SearchMCP{} // svc nil; Execute returns parse error before reaching svc
	_, err := tt.Execute(context.Background(), `not-json`)
	if err == nil || !strings.Contains(err.Error(), "parse args") {
		t.Errorf("want parse-args error, got %v", err)
	}
}


func TestCallMCP_Identity(t *testing.T) {
	tt := &CallMCP{}
	if tt.Name() != "call_mcp_tool" {
		t.Errorf("Name() = %q", tt.Name())
	}
	if tt.IsReadOnly() {
		t.Error("CallMCP IsReadOnly should be false (MCP tool may write)")
	}
}

func TestCallMCP_Schema_RequiresServerToolArgs(t *testing.T) {
	var schema map[string]any
	_ = json.Unmarshal((&CallMCP{}).Parameters(), &schema)
	required, _ := schema["required"].([]any)
	got := map[string]bool{}
	for _, r := range required {
		got[r.(string)] = true
	}
	for _, want := range []string{"server", "tool", "args"} {
		if !got[want] {
			t.Errorf("required missing %q (got %v)", want, required)
		}
	}
}


func TestCallMCP_ValidateInput_Happy(t *testing.T) {
	err := (&CallMCP{}).ValidateInput(json.RawMessage(`{"server":"gh","tool":"list_prs","args":{}}`))
	if err != nil {
		t.Errorf("happy: %v", err)
	}
}

func TestCallMCP_ValidateInput_EmptyServer(t *testing.T) {
	err := (&CallMCP{}).ValidateInput(json.RawMessage(`{"server":"","tool":"x","args":{}}`))
	if !errors.Is(err, ErrEmptyServer) {
		t.Errorf("err = %v, want ErrEmptyServer", err)
	}
}

func TestCallMCP_ValidateInput_EmptyTool(t *testing.T) {
	err := (&CallMCP{}).ValidateInput(json.RawMessage(`{"server":"x","tool":"","args":{}}`))
	if !errors.Is(err, ErrEmptyTool) {
		t.Errorf("err = %v, want ErrEmptyTool", err)
	}
}

func TestCallMCP_ValidateInput_WhitespaceServer(t *testing.T) {
	err := (&CallMCP{}).ValidateInput(json.RawMessage(`{"server":"  \t","tool":"x","args":{}}`))
	if !errors.Is(err, ErrEmptyServer) {
		t.Errorf("err = %v, want ErrEmptyServer", err)
	}
}


func TestCallMCP_Execute_MalformedArgsJSON(t *testing.T) {
	tt := &CallMCP{}
	_, err := tt.Execute(context.Background(), `not-json`)
	if err == nil || !strings.Contains(err.Error(), "parse args") {
		t.Errorf("want parse-args error, got %v", err)
	}
}


func TestMapCallToolErrorToFriendly_AllSentinelsCovered(t *testing.T) {
	cases := []struct {
		err           error
		mustContain   string
		mustNotMatch  string // catches "default" path leakage when a sentinel was supposed to match
	}{
		{
			err:         mcpdomain.ErrServerNotFound,
			mustContain: "is not configured",
		},
		{
			err:         mcpdomain.ErrServerNotConnected,
			mustContain: "is not connected",
		},
		{
			err:         mcpdomain.ErrToolNotFound,
			mustContain: "does not exist on server",
		},
		{
			err:         mcpdomain.ErrToolCallTimeout,
			mustContain: "timed out",
		},
		{
			err:         mcpdomain.ErrToolCallFailed,
			mustContain: "failed:",
		},
		{
			err:          errors.New("some other random error"),
			mustContain:  "call_mcp gh/x failed",
			mustNotMatch: "is not configured", // ensure we don't false-positive a sentinel
		},
	}

	for _, c := range cases {
		got := mapCallToolErrorToFriendly("gh", "x", c.err)
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("err=%v: got %q, want substring %q", c.err, got, c.mustContain)
		}
		if c.mustNotMatch != "" && strings.Contains(got, c.mustNotMatch) {
			t.Errorf("err=%v: got %q, must not contain %q", c.err, got, c.mustNotMatch)
		}
	}
}

func TestMapCallToolErrorToFriendly_EmbedsServerToolNames(t *testing.T) {
	got := mapCallToolErrorToFriendly("playwright", "browser_open", mcpdomain.ErrToolNotFound)
	if !strings.Contains(got, "playwright") || !strings.Contains(got, "browser_open") {
		t.Errorf("missing server/tool names: %q", got)
	}
}


func TestMCPTools_ReturnsAllInOrder(t *testing.T) {
	// V3 (2026-05-09): MCPTools always returns 5 tools — list_mcp_marketplace
	// replaced V2's LLM-rerank search and no longer needs picker/keys/factory.
	//
	// V3：MCPTools 恒返 5 件——list_mcp_marketplace 替代 V2 LLM-rerank search 后
	// 不再要 picker/keys/factory。
	tools := MCPTools(nil)
	if len(tools) != 5 {
		t.Fatalf("len = %d, want 5 (search_mcp_tools / call_mcp_tool / list_mcp_marketplace / install_mcp_server / uninstall_mcp_server)", len(tools))
	}
	wantNames := []string{"search_mcp_tools", "call_mcp_tool", "list_mcp_marketplace", "install_mcp_server", "uninstall_mcp_server"}
	for i, want := range wantNames {
		if tools[i].Name() != want {
			t.Errorf("tools[%d] = %q, want %q", i, tools[i].Name(), want)
		}
	}
}
