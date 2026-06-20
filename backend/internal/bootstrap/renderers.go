package bootstrap

import (
	"context"

	agentapp "github.com/sunweilin/anselm/backend/internal/app/agent"
	attachmentapp "github.com/sunweilin/anselm/backend/internal/app/attachment"
	chatapp "github.com/sunweilin/anselm/backend/internal/app/chat"
	documentapp "github.com/sunweilin/anselm/backend/internal/app/document"
	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
	documentdomain "github.com/sunweilin/anselm/backend/internal/domain/document"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
)

// AttachmentParts is the slice of attachment.Service the chat renderer needs (ids + the model's
// content caps → provider-agnostic LLM content parts). *attachmentapp.Service satisfies it.
//
// AttachmentParts 是 chat renderer 需要的 attachment.Service 切片（ids + 模型内容能力 → provider 无关
// 的 LLM 内容部件）。*attachmentapp.Service 满足它。
type AttachmentParts interface {
	ToContentParts(ctx context.Context, ids []string, caps attachmentapp.Capabilities) ([]llminfra.ContentPart, error)
}

// attachmentRenderer adapts attachment.Service to chat's AttachmentRenderer port, bridging chat's
// ContentCapabilities to the attachment package's own Capabilities type (so neither imports the
// other's).
//
// attachmentRenderer 把 attachment.Service 适配成 chat 的 AttachmentRenderer 端口，把 chat 的
// ContentCapabilities 桥接到 attachment 包自己的 Capabilities 类型（互不 import）。
type attachmentRenderer struct{ svc AttachmentParts }

// NewAttachmentRenderer wraps attachment.Service as chat's AttachmentRenderer.
//
// NewAttachmentRenderer 把 attachment.Service 包成 chat 的 AttachmentRenderer。
func NewAttachmentRenderer(svc AttachmentParts) chatapp.AttachmentRenderer {
	return attachmentRenderer{svc: svc}
}

var _ chatapp.AttachmentRenderer = attachmentRenderer{}

func (a attachmentRenderer) ToContentParts(ctx context.Context, ids []string, caps chatapp.ContentCapabilities) ([]llminfra.ContentPart, error) {
	return a.svc.ToContentParts(ctx, ids, attachmentapp.Capabilities{Vision: caps.Vision, NativeDocs: caps.NativeDocs})
}

// DocStore is the slice of document.Service the document/knowledge renderers need.
// *documentapp.Service satisfies it.
//
// DocStore 是 document/knowledge renderer 需要的 document.Service 切片。*documentapp.Service 满足它。
type DocStore interface {
	ResolveAttached(ctx context.Context, atts []documentdomain.AttachedDocument) ([]*documentdomain.Document, error)
	GetBatch(ctx context.Context, ids []string) ([]*documentdomain.Document, error)
}

// documentRenderer adapts document.Service to chat's DocumentRenderer port: resolve a
// conversation's attached-document refs to full docs, then serialize them as the <documents> XML
// block chat prepends to the system prompt.
//
// documentRenderer 把 document.Service 适配成 chat 的 DocumentRenderer 端口：把对话挂载的文档引用
// 解析为完整 doc，再序列化成 chat 前置进 system prompt 的 <documents> XML 块。
type documentRenderer struct{ svc DocStore }

// NewDocumentRenderer wraps document.Service as chat's DocumentRenderer.
//
// NewDocumentRenderer 把 document.Service 包成 chat 的 DocumentRenderer。
func NewDocumentRenderer(svc DocStore) chatapp.DocumentRenderer {
	return documentRenderer{svc: svc}
}

var _ chatapp.DocumentRenderer = documentRenderer{}

func (d documentRenderer) RenderAttached(ctx context.Context, atts []documentdomain.AttachedDocument) (string, error) {
	docs, err := d.svc.ResolveAttached(ctx, atts)
	if err != nil {
		return "", err
	}
	return documentapp.RenderAttachedAsXML(docs), nil
}

// knowledgeProvider adapts document.Service to agent's KnowledgeProvider port: load the agent's
// mounted document ids and serialize them as the same <documents> XML prefix.
//
// knowledgeProvider 把 document.Service 适配成 agent 的 KnowledgeProvider 端口：加载 agent 挂载的
// 文档 id，序列化成同样的 <documents> XML 前缀。
type knowledgeProvider struct{ svc DocStore }

// NewKnowledgeProvider wraps document.Service as agent's KnowledgeProvider.
//
// NewKnowledgeProvider 把 document.Service 包成 agent 的 KnowledgeProvider。
func NewKnowledgeProvider(svc DocStore) agentapp.KnowledgeProvider {
	return knowledgeProvider{svc: svc}
}

var _ agentapp.KnowledgeProvider = knowledgeProvider{}

func (k knowledgeProvider) BuildKnowledgePrefix(ctx context.Context, docIDs []string) (string, error) {
	docs, err := k.svc.GetBatch(ctx, docIDs)
	if err != nil {
		return "", err
	}
	// GetBatch's WhereIn silently drops missing ids — surface them so a dangling/deleted knowledge doc
	// fails LOUD (at create-validation and at invoke) instead of the agent running with a silently-empty
	// grounding while reporting ok (F98: strictly worse than skill's loud dead-on-arrival, F96).
	//
	// GetBatch 的 WhereIn 静默丢缺失 id——浮出它们，使 dangling/已删 knowledge doc **大声**失败（create 校验
	// 期 + invoke 期），而非 agent 静默丢失 grounding 却报 ok（F98：比 skill 的 loud DOA(F96) 更糟）。
	got := make(map[string]bool, len(docs))
	for _, d := range docs {
		got[d.ID] = true
	}
	var missing []string
	for _, id := range docIDs {
		if !got[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return "", agentdomain.ErrKnowledgeNotFound.WithDetails(map[string]any{"missing": missing})
	}
	return documentapp.RenderAttachedAsXML(docs), nil
}
