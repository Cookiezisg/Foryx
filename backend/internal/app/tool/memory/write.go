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

const writeMemoryDescription = `Save a fact to long-term memory that persists across conversations. Use when you learn something worth remembering: user preferences, their role/expertise, current project state, or external references.

Memory types:
- user: about the user themselves (role, expertise, long-term facts)
- feedback: their preferences or corrections (e.g. "don't use emojis")
- project: what they're currently working on
- reference: pointers to external systems (e.g. "bugs go to Linear FORGE project")

Calling write_memory with an existing name updates that entry; otherwise a new one is created. The system records that you (the AI) authored it; the user can review, pin, edit, or delete it in their UI.`

var writeMemorySchema = json.RawMessage(`{
	"type": "object",
	"required": ["name", "type", "description", "content"],
	"properties": {
		"name": {
			"type": "string",
			"description": "Stable identifier: lowercase letter start + lowercase/digit/underscore, ≤64 chars. Reuse an existing name to update."
		},
		"type": {
			"type": "string",
			"enum": ["user", "feedback", "project", "reference"],
			"description": "Category."
		},
		"description": {
			"type": "string",
			"description": "One-line summary (≤200 chars) shown in the system prompt memory index."
		},
		"content": {
			"type": "string",
			"description": "Full content in markdown."
		}
	}
}`)

// WriteMemory implements the write_memory system tool.
//
// WriteMemory 是 write_memory 系统工具的实现。
type WriteMemory struct {
	svc *memoryapp.Service
}

func (t *WriteMemory) Name() string                { return "write_memory" }
func (t *WriteMemory) Description() string         { return writeMemoryDescription }
func (t *WriteMemory) Parameters() json.RawMessage { return writeMemorySchema }

func (t *WriteMemory) IsReadOnly() bool        { return false }
func (t *WriteMemory) NeedsReadFirst() bool    { return false }
func (t *WriteMemory) RequiresWorkspace() bool { return false }

func (t *WriteMemory) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("write_memory.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("write_memory: name is required")
	}
	if !memorydomain.IsValidType(a.Type) {
		return fmt.Errorf("write_memory: invalid type %q (must be one of user / feedback / project / reference)", a.Type)
	}
	if strings.TrimSpace(a.Description) == "" {
		return errors.New("write_memory: description is required")
	}
	if a.Content == "" {
		return errors.New("write_memory: content is required")
	}
	return nil
}

func (t *WriteMemory) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute upserts the memory with source="ai"; pinned is not exposed (user control only).
//
// Execute 用 source="ai" upsert memory；pinned 不暴露给 LLM（用户控制）。
func (t *WriteMemory) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("write_memory.Execute: %w", err)
	}
	m, err := t.svc.Upsert(ctx, memoryapp.UpsertInput{
		Name:        a.Name,
		Type:        a.Type,
		Description: a.Description,
		Content:     a.Content,
		Source:      memorydomain.SourceAI,
	})
	if err != nil {
		if errors.Is(err, memorydomain.ErrInvalidName) {
			return fmt.Sprintf("Cannot save memory: name %q is invalid (use lowercase letters, digits, underscores; start with a letter; ≤64 chars).", a.Name), nil
		}
		return "", err
	}
	return fmt.Sprintf("Saved memory %q (type=%s). The user can pin or edit it in their UI.", m.Name, m.Type), nil
}
