// Package relation gives the LLM the relationship graph's read surface — "who uses this
// entity, what does it use" — so an edit or delete can check its impact surface first
// (the HTTP neighborhood endpoint's tool twin).
//
// Package relation 给 LLM 关系图读取面——「谁在用它、它在用谁」——编辑/删除前先查影响面
// （HTTP neighborhood 端点的工具孪生）。
package relation

import (
	"context"
	"encoding/json"
	"fmt"

	relationapp "github.com/sunweilin/forgify/backend/internal/app/relation"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
)

// RelationTools constructs the relation tool group (lazy).
//
// RelationTools 构造 relation 工具组（懒加载）。
func RelationTools(svc *relationapp.Service) []toolapp.Tool {
	return []toolapp.Tool{&GetRelations{svc: svc}}
}

type GetRelations struct{ svc *relationapp.Service }

func (t *GetRelations) Name() string { return "get_relations" }

func (t *GetRelations) Description() string {
	return "Look up an entity's relationship neighborhood: every edge in and out (uses / used-by, with entity names). Check this BEFORE deleting or reworking an entity to see what depends on it. depth 1-3 expands transitively (default 1)."
}

func (t *GetRelations) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["kind", "id"],
		"properties": {
			"kind": {"type": "string", "description": "Entity kind, e.g. function | handler | agent | workflow | trigger | control | approval | mcp | document | skill."},
			"id": {"type": "string", "description": "The entity id (fn_… / hd_… / wf_… …)."},
			"depth": {"type": "integer", "description": "Hops to expand (1-3, default 1)."}
		}
	}`)
}

func (t *GetRelations) ValidateInput(args json.RawMessage) error {
	var a struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("get_relations: bad args: %w", err)
	}
	if a.Kind == "" || a.ID == "" {
		return relationdomain.ErrInvalidRef
	}
	return nil
}

func (t *GetRelations) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Depth int    `json:"depth"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_relations: bad args: %w", err)
	}
	depth := args.Depth
	if depth == 0 {
		depth = relationdomain.MinNeighborhoodDepth
	}
	edges, err := t.svc.Neighborhood(ctx, args.Kind, args.ID, depth)
	if err != nil {
		return "", fmt.Errorf("get_relations: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"edges": edges, "count": len(edges)}), nil
}
