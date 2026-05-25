package main

import (
	"context"
	"encoding/json"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// stubTool is a minimal Tool implementation used only for buildToolset classification tests.
type stubTool struct{ name string }

func (s *stubTool) Name() string                                                           { return s.name }
func (s *stubTool) Description() string                                                    { return "" }
func (s *stubTool) Parameters() json.RawMessage                                            { return json.RawMessage(`{"type":"object","properties":{}}`) }
func (s *stubTool) IsReadOnly() bool                                                       { return true }
func (s *stubTool) NeedsReadFirst() bool                                                   { return false }
func (s *stubTool) RequiresWorkspace() bool                                                { return false }
func (s *stubTool) ValidateInput(_ json.RawMessage) error                                  { return nil }
func (s *stubTool) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (s *stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func tool(name string) toolapp.Tool { return &stubTool{name: name} }

// TestBuildToolset_KnownLazyTool verifies that a tool in lazyGroups lands in the correct lazy category.
func TestBuildToolset_KnownLazyTool(t *testing.T) {
	ts := buildToolset([]toolapp.Tool{tool("create_function")})
	if len(ts.Resident) != 0 {
		t.Fatalf("expected no resident tools, got %d", len(ts.Resident))
	}
	group := ts.Lazy["function"]
	if len(group) != 1 || group[0].Name() != "create_function" {
		t.Fatalf("expected create_function in lazy[function], got %v", group)
	}
}

// TestBuildToolset_KnownResidentTool verifies that a tool in residentToolNames lands in Resident.
func TestBuildToolset_KnownResidentTool(t *testing.T) {
	ts := buildToolset([]toolapp.Tool{tool("Read")})
	if len(ts.Resident) != 1 || ts.Resident[0].Name() != "Read" {
		t.Fatalf("expected Read in Resident, got %v", ts.Resident)
	}
	for cat, group := range ts.Lazy {
		if len(group) > 0 {
			t.Fatalf("expected no lazy tools, got %v in %q", group, cat)
		}
	}
}

// TestBuildToolset_UnknownToolPanics asserts that a tool absent from both maps panics at startup.
func TestBuildToolset_UnknownToolPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unclassified tool, got none")
		}
	}()
	buildToolset([]toolapp.Tool{tool("totally_new_unregistered_tool")})
}
