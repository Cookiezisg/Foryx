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

const forgetMemoryDescription = `Delete a memory by name when the fact is obsolete or wrong. Irreversible — the markdown file is removed.`

var forgetMemorySchema = json.RawMessage(`{
	"type": "object",
	"required": ["name"],
	"properties": {
		"name": {"type": "string", "description": "Memory name (slug) to delete."}
	}
}`)

// ForgetMemory implements the forget_memory system tool.
//
// ForgetMemory 是 forget_memory 系统工具的实现。
type ForgetMemory struct{ svc *memoryapp.Service }

func (t *ForgetMemory) Name() string                { return "forget_memory" }
func (t *ForgetMemory) Description() string         { return forgetMemoryDescription }
func (t *ForgetMemory) Parameters() json.RawMessage { return forgetMemorySchema }

func (t *ForgetMemory) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("forget_memory: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrEmptyName
	}
	return nil
}

func (t *ForgetMemory) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("forget_memory: %w", err)
	}
	if err := t.svc.Delete(ctx, a.Name); err != nil {
		if errors.Is(err, memorydomain.ErrNotFound) {
			return fmt.Sprintf("Memory %q not found (already gone?).", a.Name), nil
		}
		return "", err
	}
	return fmt.Sprintf("Forgot memory %q.", a.Name), nil
}
