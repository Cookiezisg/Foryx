// levels_test.go — coverage + fallback semantics for the toolName →
// DangerLevel table. The "ProductionToolsRegistered" contract catches
// silent omissions when a new tool is added without a level entry.
//
// levels_test.go ——toolName → DangerLevel 表的覆盖 + fallback 语义。
// ProductionToolsRegistered 契约捕获新 tool 加入但漏登记 level 的情况。
package permissionsgate

import (
	"context"
	"encoding/json"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

// fakeTool minimal Tool impl for fallback test.
type fakeTool struct{ readOnly bool }

func (f *fakeTool) Name() string                                                  { return "fake" }
func (f *fakeTool) Description() string                                           { return "" }
func (f *fakeTool) Parameters() json.RawMessage                                   { return json.RawMessage(`{}`) }
func (f *fakeTool) IsReadOnly() bool                                              { return f.readOnly }
func (f *fakeTool) NeedsReadFirst() bool                                          { return false }
func (f *fakeTool) RequiresWorkspace() bool                                       { return false }
func (f *fakeTool) ValidateInput(json.RawMessage) error                           { return nil }
func (f *fakeTool) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (f *fakeTool) Execute(context.Context, string) (string, error) { return "", nil }

func TestLookupLevel_RegisteredHit(t *testing.T) {
	if got := LookupLevel("Bash", nil); got != permdomain.LevelDangerFullAccess {
		t.Errorf("Bash level = %q, want %q", got, permdomain.LevelDangerFullAccess)
	}
	if got := LookupLevel("Read", nil); got != permdomain.LevelReadOnly {
		t.Errorf("Read level = %q, want %q", got, permdomain.LevelReadOnly)
	}
	if got := LookupLevel("Edit", nil); got != permdomain.LevelWorkspaceWrite {
		t.Errorf("Edit level = %q, want %q", got, permdomain.LevelWorkspaceWrite)
	}
}

func TestLookupLevel_UnregisteredReadOnlyFallback(t *testing.T) {
	if got := LookupLevel("dynamic-mcp-readonly", &fakeTool{readOnly: true}); got != permdomain.LevelReadOnly {
		t.Errorf("unregistered + readonly = %q, want %q", got, permdomain.LevelReadOnly)
	}
}

func TestLookupLevel_UnregisteredWritableDefault(t *testing.T) {
	if got := LookupLevel("dynamic-mcp-writable", &fakeTool{readOnly: false}); got != permdomain.LevelWorkspaceWrite {
		t.Errorf("unregistered + non-readonly = %q, want %q", got, permdomain.LevelWorkspaceWrite)
	}
}

func TestLookupLevel_UnregisteredNilToolDefault(t *testing.T) {
	if got := LookupLevel("totally-unknown", nil); got != permdomain.LevelWorkspaceWrite {
		t.Errorf("unregistered + nil tool = %q, want %q", got, permdomain.LevelWorkspaceWrite)
	}
}

// TestToolLevels_AllValid sanity-checks every registered level is one
// of the 3 enumerated values (typo guard).
//
// TestToolLevels_AllValid 检查每个登记的 level 是 3 种枚举之一（防打错）。
func TestToolLevels_AllValid(t *testing.T) {
	for name, level := range toolLevels {
		if !permdomain.IsValidDangerLevel(level) {
			t.Errorf("tool %q has invalid level %q", name, level)
		}
	}
}

func TestRegisteredTools_NonEmpty(t *testing.T) {
	tools := RegisteredTools()
	if len(tools) < 20 {
		t.Errorf("RegisteredTools returned %d entries, expected substantially more", len(tools))
	}
}
