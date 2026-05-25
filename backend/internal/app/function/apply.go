package function

import (
	"context"
	"encoding/json"
	"fmt"

	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
)

// Op is a JSON-discriminated union; Type lives in the `op` field, Raw holds the full body.
//
// Op 是 JSON 判别式 union；Type 在 `op` 字段，Raw 存完整 body。
type Op struct {
	Type string          `json:"op"`
	Raw  json.RawMessage `json:"-"`
}

// VersionDraft is the in-memory snapshot accumulated during ops apply.
//
// VersionDraft 是 ops 应用过程中累积的内存快照。
type VersionDraft struct {
	Name          string
	Description   string
	Tags          []string
	Code          string
	Parameters    []functiondomain.ParameterSpec
	ReturnSchema  map[string]any
	Dependencies  []string
	PythonVersion string
}

// OpResult is the per-op outcome surfaced back to the LLM via the tool result.
//
// OpResult 是单 op 应用结果，经 tool result 返给 LLM。
type OpResult struct {
	Index int    `json:"index"`
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
}

// ApplyOps applies a series of ops to a base draft and emits one progress delta per op.
//
// ApplyOps 把一组 ops 应用到 base 草稿，每 op 推一个 progress delta。
func (s *Service) ApplyOps(ctx context.Context, base *VersionDraft, ops []Op, progressBlockID string) (*VersionDraft, []OpResult, error) {
	state := cloneDraft(base)
	results := make([]OpResult, 0, len(ops))
	em := eventlogpkg.From(ctx)

	for i, op := range ops {
		if err := applyOne(state, op); err != nil {
			return nil, results, fmt.Errorf("functionapp.ApplyOps: ops[%d] type=%q: %w: %v", i, op.Type, functiondomain.ErrOpInvalid, err)
		}
		if err := validateIncremental(state); err != nil {
			return nil, results, fmt.Errorf("functionapp.ApplyOps: ops[%d] left state invalid: %w: %v", i, functiondomain.ErrOpInvalid, err)
		}
		results = append(results, OpResult{Index: i, Type: op.Type, OK: true})
		if em != nil && progressBlockID != "" {
			payload, _ := json.Marshal(map[string]any{"op": op.Type, "index": i})
			em.DeltaBlock(ctx, progressBlockID, string(payload)+"\n")
		}
	}
	if err := validateFinal(state); err != nil {
		return nil, results, fmt.Errorf("functionapp.ApplyOps: final validation: %w: %v", functiondomain.ErrASTParseError, err)
	}
	return state, results, nil
}

func applyOne(state *VersionDraft, op Op) error {
	switch op.Type {
	case "set_meta":
		var p struct {
			Name        *string  `json:"name,omitempty"`
			Description *string  `json:"description,omitempty"`
			Tags        []string `json:"tags,omitempty"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_meta unmarshal: %w", err)
		}
		if p.Name != nil {
			state.Name = *p.Name
		}
		if p.Description != nil {
			state.Description = *p.Description
		}
		if p.Tags != nil {
			state.Tags = p.Tags
		}
	case "set_code":
		var p struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_code unmarshal: %w", err)
		}
		state.Code = p.Code
	case "set_parameters":
		var p struct {
			Parameters []functiondomain.ParameterSpec `json:"parameters"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_parameters unmarshal: %w", err)
		}
		state.Parameters = p.Parameters
	case "set_return_schema":
		var p struct {
			ReturnSchema map[string]any `json:"returnSchema"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_return_schema unmarshal: %w", err)
		}
		state.ReturnSchema = p.ReturnSchema
	case "set_dependencies":
		var p struct {
			Dependencies []string `json:"dependencies"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_dependencies unmarshal: %w", err)
		}
		state.Dependencies = p.Dependencies
	case "set_python_version":
		var p struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_python_version unmarshal: %w", err)
		}
		state.PythonVersion = p.Version
	default:
		return fmt.Errorf("unknown op type: %q", op.Type)
	}
	return nil
}

// ParseOps decodes the LLM wire format (JSON array with `op` discriminator) into []Op.
//
// ParseOps 把 LLM 线上格式（带 `op` 判别字段的 JSON 数组）解码为 []Op。
func ParseOps(raw json.RawMessage) ([]Op, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("ops array unmarshal: %w", err)
	}
	ops := make([]Op, 0, len(arr))
	for i, r := range arr {
		var disc struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal(r, &disc); err != nil {
			return nil, fmt.Errorf("ops[%d]: %w", i, err)
		}
		if disc.Op == "" {
			return nil, fmt.Errorf("ops[%d]: missing 'op' discriminator", i)
		}
		ops = append(ops, Op{Type: disc.Op, Raw: r})
	}
	return ops, nil
}

func cloneDraft(d *VersionDraft) *VersionDraft {
	if d == nil {
		return &VersionDraft{}
	}
	out := *d
	out.Tags = append([]string(nil), d.Tags...)
	out.Parameters = append([]functiondomain.ParameterSpec(nil), d.Parameters...)
	out.Dependencies = append([]string(nil), d.Dependencies...)
	if d.ReturnSchema != nil {
		out.ReturnSchema = make(map[string]any, len(d.ReturnSchema))
		for k, v := range d.ReturnSchema {
			out.ReturnSchema[k] = v
		}
	}
	return &out
}
