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

const readMemoryDescription = `Load one memory's full markdown body by name. The system prompt already lists available memories (pinned ones in full, the rest as a name+description index) — use this to pull the full text of a non-pinned memory before relying on it.`

var readMemorySchema = json.RawMessage(`{
	"type": "object",
	"required": ["name"],
	"properties": {
		"name": {"type": "string", "description": "Memory name (slug) from the system-prompt index."}
	}
}`)

// ReadMemory implements the read_memory system tool.
//
// ReadMemory 是 read_memory 系统工具的实现。
type ReadMemory struct{ svc *memoryapp.Service }

func (t *ReadMemory) Name() string                { return "read_memory" }
func (t *ReadMemory) Description() string         { return readMemoryDescription }
func (t *ReadMemory) Parameters() json.RawMessage { return readMemorySchema }

func (t *ReadMemory) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("read_memory: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrEmptyName
	}
	return nil
}

func (t *ReadMemory) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("read_memory: %w", err)
	}
	m, err := t.svc.Get(ctx, a.Name)
	if err != nil {
		if errors.Is(err, memorydomain.ErrNotFound) {
			return fmt.Sprintf("Memory %q not found. The system-prompt memory index lists the available names.", a.Name), nil
		}
		return "", err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s (source: %s)\n", m.Name, m.Source)
	if m.Description != "" {
		fmt.Fprintf(&sb, "%s\n", m.Description)
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(m.Content)
	return sb.String(), nil
}
