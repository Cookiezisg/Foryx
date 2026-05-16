package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
)

const forgetMemoryDescription = `Delete a memory entry by name. Use when a memory is outdated (the underlying fact changed), incorrect (you got it wrong earlier), or the user explicitly asks you to forget something. Once deleted, the memory is no longer visible in the system prompt index.`

var forgetMemorySchema = json.RawMessage(`{
	"type": "object",
	"required": ["name"],
	"properties": {
		"name": {
			"type": "string",
			"description": "Memory name to delete."
		}
	}
}`)

// ForgetMemory implements the forget_memory system tool.
//
// ForgetMemory 是 forget_memory 系统工具的实现。
type ForgetMemory struct {
	svc *memoryapp.Service
}

func (t *ForgetMemory) Name() string                { return "forget_memory" }
func (t *ForgetMemory) Description() string         { return forgetMemoryDescription }
func (t *ForgetMemory) Parameters() json.RawMessage { return forgetMemorySchema }

func (t *ForgetMemory) IsReadOnly() bool        { return false }
func (t *ForgetMemory) NeedsReadFirst() bool    { return false }
func (t *ForgetMemory) RequiresWorkspace() bool { return false }

func (t *ForgetMemory) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("forget_memory.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("forget_memory: name is required")
	}
	return nil
}

func (t *ForgetMemory) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *ForgetMemory) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("forget_memory.Execute: %w", err)
	}
	if err := t.svc.Delete(ctx, a.Name); err != nil {
		if errors.Is(err, memorydomain.ErrNotFound) {
			return fmt.Sprintf("Memory %q not found; nothing to forget.", a.Name), nil
		}
		return "", err
	}
	return fmt.Sprintf("Forgotten memory %q.", a.Name), nil
}
