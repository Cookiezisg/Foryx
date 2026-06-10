package control

import (
	"context"
	"encoding/json"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// AsMentionResolver exposes this service as the chat @-mention resolver for control logics: a
// reference snapshots the description + active version's branches. Enables AI :iterate (R0065).
//
// AsMentionResolver 把本 service 暴露为 control 的 @ resolver：引用快照 description + active 版本的 branches。
// 使 AI :iterate（R0065）可用。
func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

type mentionResolver struct{ svc *Service }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionControl }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	c, err := r.svc.Get(ctx, id) // Get attaches ActiveVersion
	if err != nil {
		return nil, fmt.Errorf("controlapp.mentionResolver.Resolve %s: %w", id, err)
	}
	content := c.Description
	if c.ActiveVersion != nil {
		b, _ := json.MarshalIndent(c.ActiveVersion.Branches, "", "  ")
		content += "\n\nBranches (first true When wins → Emit):\n" + string(b)
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionControl, ID: c.ID, Name: c.Name, Content: content,
	}, nil
}
