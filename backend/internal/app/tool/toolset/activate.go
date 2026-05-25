// Package toolset provides the activate_tools system tool for on-demand lazy group loading.
//
// Package toolset 提供按需加载 lazy 组的 activate_tools 系统工具。
package toolset

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ValidCategories is the closed enum of categories accepted by activate_tools.
//
// ValidCategories 是 activate_tools 接受的封闭 category 枚举。
var ValidCategories = []string{"function", "handler", "workflow", "mcp", "document", "skill"}

var activateToolsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["category"],
	"properties": {
		"category": {
			"type": "string",
			"enum": ["function","handler","workflow","mcp","document","skill"],
			"description": "Tool group to activate for the rest of this conversation."
		}
	}
}`)

// ActivateTools activates a lazy tool group and records it in AgentState.
// It is RESIDENT so the LLM always sees it regardless of what groups are loaded.
//
// ActivateTools 激活一个 lazy 工具组并记录到 AgentState；
// 作为 RESIDENT 工具，LLM 在任意 group 加载状态下都能看到它。
type ActivateTools struct {
	// toolsByGroup maps each lazy category to its tool names for the Execute response.
	toolsByGroup map[string][]string
}

// NewActivateTools constructs ActivateTools with a name-list snapshot from the Toolset.
//
// NewActivateTools 从 Toolset 构造 ActivateTools，快照各组工具名列表。
func NewActivateTools(ts toolapp.Toolset) *ActivateTools {
	byGroup := make(map[string][]string, len(ts.Lazy))
	for cat, tools := range ts.Lazy {
		names := make([]string, len(tools))
		for i, t := range tools {
			names[i] = t.Name()
		}
		byGroup[cat] = names
	}
	return &ActivateTools{toolsByGroup: byGroup}
}

func (t *ActivateTools) Name() string                { return "activate_tools" }
func (t *ActivateTools) IsReadOnly() bool            { return true }
func (t *ActivateTools) NeedsReadFirst() bool        { return false }
func (t *ActivateTools) RequiresWorkspace() bool     { return false }
func (t *ActivateTools) Parameters() json.RawMessage { return activateToolsSchema }

func (t *ActivateTools) Description() string {
	return "Activate a tool group for on-demand use. Call this once per category before using any tool from that group. Categories: function, handler, workflow, mcp, document, skill."
}

func (t *ActivateTools) ValidateInput(args json.RawMessage) error {
	var a struct {
		Category string `json:"category"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("activate_tools.ValidateInput: %w", err)
	}
	for _, valid := range ValidCategories {
		if a.Category == valid {
			return nil
		}
	}
	return fmt.Errorf("activate_tools.ValidateInput: unknown category %q; must be one of %s",
		a.Category, strings.Join(ValidCategories, ", "))
}

func (t *ActivateTools) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute records the group activation on AgentState and returns the tool names for that category.
//
// Execute 在 AgentState 记录组激活，返回该 category 的工具名列表。
func (t *ActivateTools) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("activate_tools.Execute: parse args: %w", err)
	}

	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		state.ActivateGroup(args.Category)
	}

	names := t.toolsByGroup[args.Category]
	if len(names) == 0 {
		return fmt.Sprintf("Activated %s: (no tools in this group)", args.Category), nil
	}
	return fmt.Sprintf("Activated %s: %s", args.Category, strings.Join(names, ", ")), nil
}

var _ toolapp.Tool = (*ActivateTools)(nil)
