package function

import (
	"encoding/json"
	"fmt"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	jsonrepairpkg "github.com/sunweilin/forgify/backend/internal/pkg/jsonrepair"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// Op is a JSON-discriminated forge op; Type lives in the `op` field, Raw holds the body.
//
// Op 是 JSON 判别式锻造 op；Type 在 `op` 字段，Raw 存 body。
type Op struct {
	Type string
	Raw  json.RawMessage
}

// VersionDraft is the in-memory snapshot accumulated while applying ops.
//
// VersionDraft 是应用 ops 时累积的内存快照。
type VersionDraft struct {
	Name          string
	Description   string
	Tags          []string
	Code          string
	Inputs        []schemapkg.Field
	Outputs       []schemapkg.Field
	Dependencies  []string
	PythonVersion string
}

// OpResult is one applied op's outcome, surfaced back to the LLM.
//
// OpResult 是单个 op 的应用结果，回呈给 LLM。
type OpResult struct {
	Index int    `json:"index"`
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
}

// ApplyOps applies ops to a base draft, validating after each. It is pure (no
// streaming): progress narration is the tool layer's job (it pushes env-fix attempts
// onto the SSE stream; op application is fast and not worth streaming per-op).
//
// ApplyOps 把 ops 套到 base 草稿，每步后校验。它是纯函数（不推流）：进度叙述是 tool 层的事
// （它把 env-fix 尝试推到 SSE 流；op 应用很快，不值得逐个推）。
func (s *Service) ApplyOps(base *VersionDraft, ops []Op) (*VersionDraft, []OpResult, error) {
	state := cloneDraft(base)
	results := make([]OpResult, 0, len(ops))
	for i, op := range ops {
		if err := applyOne(state, op); err != nil {
			return nil, results, fmt.Errorf("functionapp.ApplyOps: ops[%d] type=%q: %w: %v", i, op.Type, functiondomain.ErrOpInvalid, err)
		}
		if err := validateIncremental(state); err != nil {
			return nil, results, fmt.Errorf("functionapp.ApplyOps: ops[%d] left state invalid: %w: %v", i, functiondomain.ErrOpInvalid, err)
		}
		results = append(results, OpResult{Index: i, Type: op.Type, OK: true})
	}
	if err := validateFinal(state); err != nil {
		return nil, results, fmt.Errorf("functionapp.ApplyOps: final validation: %w: %v", functiondomain.ErrInvalidCode, err)
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
	case "set_inputs":
		var p struct {
			Inputs []schemapkg.Field `json:"inputs"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_inputs unmarshal: %w", err)
		}
		state.Inputs = p.Inputs
	case "set_outputs":
		var p struct {
			Outputs []schemapkg.Field `json:"outputs"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_outputs unmarshal: %w", err)
		}
		state.Outputs = p.Outputs
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

// ParseOps decodes the LLM wire format (JSON array with `op` discriminator) into []Op,
// repairing malformed JSON first.
//
// ParseOps 把 LLM 线上格式（带 `op` 判别字段的 JSON 数组）解码为 []Op，先修畸形 JSON。
func ParseOps(raw json.RawMessage) ([]Op, error) {
	raw = jsonrepairpkg.RepairBytes(raw)
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("functionapp.ParseOps: ops array unmarshal: %w", err)
	}
	ops := make([]Op, 0, len(arr))
	for i, r := range arr {
		var disc struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal(r, &disc); err != nil {
			return nil, fmt.Errorf("functionapp.ParseOps: ops[%d]: %w", i, err)
		}
		if disc.Op == "" {
			return nil, fmt.Errorf("functionapp.ParseOps: ops[%d]: missing 'op' discriminator", i)
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
	out.Inputs = append([]schemapkg.Field(nil), d.Inputs...)
	out.Outputs = append([]schemapkg.Field(nil), d.Outputs...)
	out.Dependencies = append([]string(nil), d.Dependencies...)
	return &out
}
