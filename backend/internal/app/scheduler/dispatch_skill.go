package scheduler

import (
	"context"
	"fmt"

	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
)

// SkillDispatcher bridges workflow skill nodes to skillapp.Service.Activate.
//
// SkillDispatcher 把 workflow skill 节点桥接到 skillapp.Activate。
type SkillDispatcher struct {
	svc *skillapp.Service
}

// NewSkillDispatcher constructs SkillDispatcher.
//
// NewSkillDispatcher 构造 SkillDispatcher。
func NewSkillDispatcher(svc *skillapp.Service) *SkillDispatcher {
	return &SkillDispatcher{svc: svc}
}

// Dispatch reads skillName + arguments and resolves the skill body via Activate.
//
// Dispatch 读 skillName + arguments 并通过 Activate 解析 skill body。
func (d *SkillDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	name, _ := in.Node.Config["skillName"].(string)
	if name == "" {
		return DispatchOutput{Error: fmt.Errorf("skill node %q: skillName required", in.Node.ID)}
	}

	var args []string
	if raw, ok := in.Node.Config["arguments"].([]any); ok {
		args = make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				args = append(args, s)
			}
		}
	}

	body, err := d.svc.Activate(ctx, name, args)
	if err != nil {
		return DispatchOutput{Error: err}
	}
	return DispatchOutput{Outputs: map[string]any{"out": body}}
}
