package document

import (
	"context"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// AsMentionResolver exposes this Service as the @-mention resolver for documents:
// @-mentioning a doc snapshots its description + body as the injected content.
//
// AsMentionResolver 把本 Service 暴露为 document 的 @ resolver：@ 一篇文档时，快照其
// description + 正文作为注入内容。
func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

type mentionResolver struct{ svc *Service }

var _ mentiondomain.Resolver = (*mentionResolver)(nil)

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionDocument }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	d, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("document.mentionResolver.Resolve %s: %w", id, err)
	}
	content := d.Description
	if d.Content != "" {
		if content != "" {
			content += "\n\n"
		}
		content += d.Content
	}
	return &mentiondomain.Reference{
		Type:    mentiondomain.MentionDocument,
		ID:      d.ID,
		Name:    d.Name,
		Content: content,
	}, nil
}
