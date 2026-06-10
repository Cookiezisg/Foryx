package approval

import (
	"context"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// AsMentionResolver exposes this service as the chat @-mention resolver for approval forms: a
// reference snapshots the description + active version's prompt template. Enables AI :iterate (R0065).
//
// AsMentionResolver 把本 service 暴露为 approval 的 @ resolver：引用快照 description + active 版本的 prompt
// 模板。使 AI :iterate（R0065）可用。
func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

type mentionResolver struct{ svc *Service }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionApproval }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	f, err := r.svc.Get(ctx, id) // Get attaches ActiveVersion
	if err != nil {
		return nil, fmt.Errorf("approvalapp.mentionResolver.Resolve %s: %w", id, err)
	}
	content := f.Description
	if f.ActiveVersion != nil {
		content += "\n\nTemplate (markdown, {{ input.* }} interpolation):\n" + f.ActiveVersion.Template
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionApproval, ID: f.ID, Name: f.Name, Content: content,
	}, nil
}
