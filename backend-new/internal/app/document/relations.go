package document

import (
	"context"

	"go.uber.org/zap"

	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	wikilinkpkg "github.com/sunweilin/forgify/backend/internal/pkg/wikilink"
)

// RelationSyncer is the subset of the relation Service that document consumes
// (nil-tolerant). The relation app's *Service satisfies it directly — same method
// signatures — so wiring is a plain injection with no adapter.
//
// RelationSyncer 是 document 消费的 relation Service 子集（nil-tolerant）。relation app 的
// *Service 直接满足它——签名一致——故装配是纯注入、无需适配器。
type RelationSyncer interface {
	SyncOutgoing(ctx context.Context, fromKind, fromID string, kindScope []string, edges []relationdomain.SyncEdge) error
	PurgeEntity(ctx context.Context, kind, id string) error
}

// syncRelationsForDocumentBody parses wikilinks in the body and re-syncs the
// document's outgoing `link` edges. wikilink yields ids only (no kind);
// relation.KindForID resolves the kind off the id prefix and filters unknown
// prefixes, and a self-link is skipped.
//
// syncRelationsForDocumentBody 解析 body 中 wikilink 并重 sync 文档的出向 `link` 边。wikilink
// 只给 id（无 kind）；relation.KindForID 据 id 前缀解析 kind 并过滤未知前缀，自链跳过。
func (s *Service) syncRelationsForDocumentBody(ctx context.Context, docID, body string) {
	if s.relations == nil {
		return
	}
	refs := wikilinkpkg.Parse(body)
	edges := make([]relationdomain.SyncEdge, 0, len(refs))
	for _, ref := range refs {
		kind, ok := relationdomain.KindForID(ref.ID)
		if !ok {
			continue // unknown prefix — not a real entity ref. 未知前缀——非真实体引用。
		}
		if kind == relationdomain.EntityKindDocument && ref.ID == docID {
			continue // self-link. 自链。
		}
		edges = append(edges, relationdomain.SyncEdge{
			OtherKind: kind,
			OtherID:   ref.ID,
			Kind:      relationdomain.KindLink,
			Attrs:     map[string]any{"count": ref.Count},
		})
	}
	if err := s.relations.SyncOutgoing(ctx, relationdomain.EntityKindDocument, docID,
		[]string{relationdomain.KindLink}, edges); err != nil {
		s.log.Warn("relation SyncOutgoing (doc links) failed",
			zap.String("documentId", docID), zap.Error(err))
	}
}

// purgeRelationsForDocuments cascade-purges all edges of every deleted doc.
//
// purgeRelationsForDocuments 级联清除每篇被删文档的所有边。
func (s *Service) purgeRelationsForDocuments(ctx context.Context, docIDs []string) {
	if s.relations == nil {
		return
	}
	for _, id := range docIDs {
		if err := s.relations.PurgeEntity(ctx, relationdomain.EntityKindDocument, id); err != nil {
			s.log.Warn("relation PurgeEntity failed",
				zap.String("documentId", id), zap.Error(err))
		}
	}
}

// NamesByIDs implements relation's Namer port for the document kind: id → display
// name (the doc Name). relation's read-time hydrate calls it to label document
// nodes/edges; a missing id simply gets no name (falls back to the raw id there).
//
// NamesByIDs 实现 relation 的 Namer 端口（document 类）：id → 显示名（文档 Name）。relation
// 读时 hydrate 调它给 document 节点/边贴名；缺失 id 直接无名（那边回退原始 id）。
func (s *Service) NamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	docs, err := s.repo.GetBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(docs))
	for _, d := range docs {
		out[d.ID] = d.Name
	}
	return out, nil
}
