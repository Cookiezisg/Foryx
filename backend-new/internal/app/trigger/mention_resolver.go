package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// AsMentionResolver exposes this service as the chat @-mention resolver for triggers: a reference
// snapshots the description + kind + config (triggers have no versions). Enables AI :iterate (R0065).
//
// AsMentionResolver 把本 service 暴露为 trigger 的 @ resolver：引用快照 description + kind + config
// （trigger 无版本）。使 AI :iterate（R0065）可用。
func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

type mentionResolver struct{ svc *Service }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionTrigger }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	t, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("triggerapp.mentionResolver.Resolve %s: %w", id, err)
	}
	content := t.Description
	cfg, _ := json.MarshalIndent(t.Config, "", "  ")
	content += fmt.Sprintf("\n\nKind: %s\nConfig:\n%s", t.Kind, cfg)
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionTrigger, ID: t.ID, Name: t.Name, Content: content,
	}, nil
}
