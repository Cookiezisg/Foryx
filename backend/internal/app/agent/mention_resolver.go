package agent

import (
	"context"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

type mentionResolver struct{ svc *Service }

// AsMentionResolver exposes this service as a chat @-mention resolver for agents (quadrinity parity
// with function/handler/workflow — @-mentioning an agent pulls its name + description + prompt).
//
// AsMentionResolver 把本 service 暴露为 agent 的 @ resolver（与 trinity 同等）。
func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionAgent }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	a, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agent.mentionResolver.Resolve %s: %w", id, err)
	}
	content := a.Description
	if prompt, _, _, _, cErr := r.svc.GetAgentConfig(ctx, id); cErr == nil && prompt != "" {
		if content != "" {
			content += "\n\n"
		}
		content += prompt
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionAgent, ID: a.ID, Name: a.Name, Content: content,
	}, nil
}
