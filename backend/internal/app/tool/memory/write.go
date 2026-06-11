package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
)

const writeMemoryDescription = `Save a durable fact to remember across conversations (a user trait, a preference/correction, current-project context, or an external reference). Reusing an existing name updates that memory in place. Recorded as AI-authored and user-editable; pinning is the user's choice (not yours).`

var writeMemorySchema = json.RawMessage(`{
	"type": "object",
	"required": ["name", "description", "content"],
	"properties": {
		"name": {
			"type": "string",
			"description": "Stable slug identity: lowercase-letter start, then lowercase / digits / _ / -, up to 64 chars. Reuse an existing name to update it."
		},
		"description": {
			"type": "string",
			"description": "One-line summary shown in the system-prompt memory index."
		},
		"content": {
			"type": "string",
			"description": "Full memory body in markdown."
		}
	}
}`)

// WriteMemory implements the write_memory system tool.
//
// WriteMemory 是 write_memory 系统工具的实现。
type WriteMemory struct{ svc *memoryapp.Service }

func (t *WriteMemory) Name() string                { return "write_memory" }
func (t *WriteMemory) Description() string         { return writeMemoryDescription }
func (t *WriteMemory) Parameters() json.RawMessage { return writeMemorySchema }

func (t *WriteMemory) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("write_memory: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrEmptyName
	}
	if strings.TrimSpace(a.Description) == "" {
		return ErrEmptyDescription
	}
	if a.Content == "" {
		return ErrEmptyContent
	}
	return nil
}

// Execute upserts the memory as source=ai; pinned stays user-controlled (never exposed
// to the LLM, so a write always lands unpinned and the user opts into pinning).
//
// Execute 以 source=ai upsert；pinned 由用户控制（永不暴露给 LLM，故写入恒为非 pinned，
// 由用户选择置顶）。
func (t *WriteMemory) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("write_memory: %w", err)
	}
	m, err := t.svc.Upsert(ctx, memoryapp.UpsertInput{
		Name:        a.Name,
		Description: a.Description,
		Content:     a.Content,
		Source:      memorydomain.SourceAI,
	})
	if err != nil {
		switch {
		case errors.Is(err, memorydomain.ErrInvalidName):
			return fmt.Sprintf("Cannot save memory: name %q is invalid (lowercase slug — a-z start, then a-z/0-9/_/-, up to 64 chars).", a.Name), nil
		case errors.Is(err, memorydomain.ErrInvalidInput):
			return "Cannot save memory: both description and content are required.", nil
		default:
			return "", err
		}
	}
	return fmt.Sprintf("Saved memory %q. The user can pin or edit it in their UI.", m.Name), nil
}
