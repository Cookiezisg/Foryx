package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)


func TestSubagentTool_Identity(t *testing.T) {
	tt := &SubagentTool{}
	if tt.Name() != "Subagent" {
		t.Errorf("Name() = %q, want Subagent", tt.Name())
	}
	if tt.Description() == "" {
		t.Error("Description() empty")
	}
	if len(tt.Parameters()) == 0 {
		t.Error("Parameters() empty")
	}
}

func TestSubagentTool_StaticMetadata(t *testing.T) {
	tt := &SubagentTool{}
	if tt.IsReadOnly() {
		t.Error("IsReadOnly = true; want false (sub-runner can mutate)")
	}
	if tt.NeedsReadFirst() {
		t.Error("NeedsReadFirst = true; want false")
	}
	if tt.RequiresWorkspace() {
		t.Error("RequiresWorkspace = true; want false")
	}
}


func TestSubagentTool_Schema_DeclaresRequiredFields(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&SubagentTool{}).Parameters(), &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("schema missing required[]")
	}
	got := map[string]bool{}
	for _, r := range required {
		got[r.(string)] = true
	}
	if !got["subagent_type"] || !got["prompt"] {
		t.Errorf("required = %v, want subagent_type + prompt", got)
	}
}


func TestValidateInput_HappyPath(t *testing.T) {
	tt := &SubagentTool{}
	err := tt.ValidateInput(json.RawMessage(`{"subagent_type":"Explore","prompt":"find foo"}`))
	if err != nil {
		t.Errorf("ValidateInput happy: %v", err)
	}
}

func TestValidateInput_EmptyType(t *testing.T) {
	tt := &SubagentTool{}
	err := tt.ValidateInput(json.RawMessage(`{"subagent_type":"","prompt":"x"}`))
	if !errors.Is(err, ErrEmptyType) {
		t.Errorf("want ErrEmptyType, got %v", err)
	}
}

func TestValidateInput_WhitespaceType(t *testing.T) {
	tt := &SubagentTool{}
	err := tt.ValidateInput(json.RawMessage(`{"subagent_type":"   ","prompt":"x"}`))
	if !errors.Is(err, ErrEmptyType) {
		t.Errorf("want ErrEmptyType, got %v", err)
	}
}

func TestValidateInput_EmptyPrompt(t *testing.T) {
	tt := &SubagentTool{}
	err := tt.ValidateInput(json.RawMessage(`{"subagent_type":"Explore","prompt":""}`))
	if !errors.Is(err, ErrEmptyPrompt) {
		t.Errorf("want ErrEmptyPrompt, got %v", err)
	}
}

func TestValidateInput_MalformedJSON(t *testing.T) {
	tt := &SubagentTool{}
	err := tt.ValidateInput(json.RawMessage(`not-json`))
	if err == nil {
		t.Error("want error on malformed JSON")
	}
}


func TestCheckPermissions_AlwaysAllow(t *testing.T) {
	tt := &SubagentTool{}
	for _, mode := range []toolapp.PermissionMode{
		toolapp.PermissionModeDefault,
		toolapp.PermissionModeAcceptEdits,
		toolapp.PermissionModePlan,
	} {
		if got := tt.CheckPermissions(json.RawMessage(`{}`), mode); got != toolapp.PermissionAllow {
			t.Errorf("mode %v: got %v, want PermissionAllow", mode, got)
		}
	}
}


// TestExecute_RecursionRefused covers subagent.md §8 layer-2 defense:
// even if (somehow) a sub-runner ended up with a SubagentTool in its
// registry, the runtime depth check refuses the call. The structural
// defense (layer 1) lives in app/subagent.Service.filterTools and is
// covered there.
//
// TestExecute_RecursionRefused 覆盖 §8 第二层防御：即便（万一）sub-runner
// 拿到了带 SubagentTool 的注册表，运行时深度检查也会拒绝。结构性防御
// （第一层）在 app/subagent.Service.filterTools 已覆盖。
func TestExecute_RecursionRefused(t *testing.T) {
	tt := &SubagentTool{} // svc not needed — Execute returns before calling Spawn
	ctx := reqctxpkg.WithSubagentDepth(context.Background(), 1)
	_, err := tt.Execute(ctx, `{"subagent_type":"Explore","prompt":"x"}`)
	if !errors.Is(err, subagentdomain.ErrRecursionAttempt) {
		t.Errorf("want ErrRecursionAttempt, got %v", err)
	}
}

func TestExecute_RecursionRefused_DepthGreaterThanOne(t *testing.T) {
	tt := &SubagentTool{}
	ctx := reqctxpkg.WithSubagentDepth(context.Background(), 5)
	_, err := tt.Execute(ctx, `{"subagent_type":"Explore","prompt":"x"}`)
	if !errors.Is(err, subagentdomain.ErrRecursionAttempt) {
		t.Errorf("want ErrRecursionAttempt at depth 5, got %v", err)
	}
}


// Execute parses argsJSON (a string parameter) — distinct from
// ValidateInput which parses json.RawMessage. Both must reject malformed.
//
// Execute 解析 argsJSON 字符串参数——与 ValidateInput 解析 json.RawMessage
// 不同。两者都必须拒绝畸形 JSON。
func TestExecute_MalformedJSON_NoSpawnCall(t *testing.T) {
	tt := &SubagentTool{} // svc nil; if Spawn were reached, nil-deref panic
	_, err := tt.Execute(context.Background(), `not-json`)
	if err == nil {
		t.Fatal("want error on malformed argsJSON")
	}
	if !strings.Contains(err.Error(), "parse args") {
		t.Errorf("err should mention parse args, got %v", err)
	}
}


func TestAppendNote_NonEmptyBody(t *testing.T) {
	got := appendNote("here is the answer", "subagent hit max turns")
	if !strings.Contains(got, "here is the answer") {
		t.Error("body lost")
	}
	if !strings.Contains(got, "[note: subagent hit max turns]") {
		t.Error("note marker missing")
	}
	if !strings.Contains(got, "\n\n") {
		t.Error("blank-line separator missing")
	}
}

func TestAppendNote_EmptyBody(t *testing.T) {
	got := appendNote("", "subagent failed: panic")
	if got != "[note: subagent failed: panic]" {
		t.Errorf("empty-body output = %q", got)
	}
}

func TestAppendNote_WhitespaceBody(t *testing.T) {
	got := appendNote("   \n\t ", "x")
	if !strings.HasPrefix(got, "[note:") {
		t.Errorf("whitespace body should be treated as empty: %q", got)
	}
}
