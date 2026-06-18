package document

import (
	"context"
	"encoding/json"
	"fmt"

	documentapp "github.com/sunweilin/anselm/backend/internal/app/document"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
)

const listDocumentsDescription = `List direct children one level under parentId (null/omit = root): id, name, description, path, position each (returned in sibling order). position is the 0-based sibling index (0 = first) — use it to see current ordering and to pick the target index for move_document. Walk the tree progressively; use search_documents for keyword search.`

var listDocumentsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"parentId": {"type": ["string", "null"], "description": "Parent doc ID; null/omit = root."}
	}
}`)

// ListDocuments implements the list_documents system tool.
//
// ListDocuments 是 list_documents 系统工具的实现。
type ListDocuments struct{ svc *documentapp.Service }

func (t *ListDocuments) Name() string                { return "list_documents" }
func (t *ListDocuments) Description() string         { return listDocumentsDescription }
func (t *ListDocuments) Parameters() json.RawMessage { return listDocumentsSchema }

func (t *ListDocuments) ValidateInput(args json.RawMessage) error {
	if len(args) == 0 {
		return nil
	}
	var a struct {
		ParentID *string `json:"parentId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("list_documents: bad args: %w", err)
	}
	return nil
}

func (t *ListDocuments) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		ParentID *string `json:"parentId"`
	}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "", fmt.Errorf("list_documents: %w", err)
		}
	}
	// Empty-string parentId is treated as null (root).
	//
	// 空字符串 parentId 视为 null（根级）。
	if a.ParentID != nil && *a.ParentID == "" {
		a.ParentID = nil
	}
	rows, err := t.svc.ListByParent(ctx, a.ParentID)
	if err != nil {
		return "", err
	}
	type slim struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Position    int    `json:"position"`
		Description string `json:"description,omitempty"`
	}
	out := make([]slim, 0, len(rows))
	for _, d := range rows {
		out = append(out, slim{ID: d.ID, Name: d.Name, Path: d.Path, Position: d.Position, Description: d.Description})
	}
	return toolapp.ToJSON(map[string]any{"count": len(out), "documents": out}), nil
}
