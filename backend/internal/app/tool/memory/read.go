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

const readMemoryDescription = `Retrieve a specific memory entry by name. Memories are persistent facts about the user, their preferences, current projects, or external references. Check the memory index in your system prompt to discover available memory names; call read_memory only when an indexed entry is directly relevant to the user's current request.`

var readMemorySchema = json.RawMessage(`{
	"type": "object",
	"required": ["name"],
	"properties": {
		"name": {
			"type": "string",
			"description": "Memory name (lowercase letters / digits / underscore; matches an entry shown in the system prompt index)."
		}
	}
}`)

// ReadMemory implements the read_memory system tool.
//
// ReadMemory 是 read_memory 系统工具的实现。
type ReadMemory struct {
	svc *memoryapp.Service
}

func (t *ReadMemory) Name() string                { return "read_memory" }
func (t *ReadMemory) Description() string         { return readMemoryDescription }
func (t *ReadMemory) Parameters() json.RawMessage { return readMemorySchema }

func (t *ReadMemory) IsReadOnly() bool        { return true }
func (t *ReadMemory) NeedsReadFirst() bool    { return false }
func (t *ReadMemory) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty name pre-Execute.
//
// ValidateInput 在 Execute 前拒绝空 name。
func (t *ReadMemory) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("read_memory.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("read_memory: name is required")
	}
	return nil
}

func (t *ReadMemory) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *ReadMemory) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("read_memory.Execute: %w", err)
	}
	m, err := t.svc.Get(ctx, a.Name)
	if err != nil {
		if errors.Is(err, memorydomain.ErrNotFound) {
			return fmt.Sprintf("Memory %q not found. Available memories are shown in the system prompt index.", a.Name), nil
		}
		return "", err
	}
	return fmt.Sprintf("# %s (type=%s, source=%s)\n\n%s", m.Name, m.Type, m.Source, m.Content), nil
}
