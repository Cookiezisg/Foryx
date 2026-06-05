// Package document owns the Service for the Notion-style document tree (CRUD + path
// computation + move) plus the adapters that join it to catalog / mention / relation.
// Workspace isolation is automatic at the orm layer, so the Service holds no workspace
// id — it just calls the repo and lets the store filter from ctx.
//
// Package document 持有 Notion-style 文档树的 Service（CRUD + path 计算 + move），以及把它
// 接入 catalog / mention / relation 的适配器。workspace 隔离在 orm 层自动完成，故 Service 不
// 持 workspace id——直接调 repo，由 store 据 ctx 过滤。
package document

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// Service is the document tree application façade.
//
// Service 是文档树应用 façade。
type Service struct {
	repo    documentdomain.Repository
	emitter notificationdomain.Emitter
	log     *zap.Logger

	// relations is the optional relation-sync hook; nil disables wikilink edge sync.
	// relations 是可选的 relation 同步钩子；nil 时禁用 wikilink 边同步。
	relations RelationSyncer
}

// New constructs a Service. A nil emitter disables notifications (best-effort).
//
// New 构造 Service。emitter 为 nil 时禁用通知（best-effort）。
func New(repo documentdomain.Repository, emitter notificationdomain.Emitter, log *zap.Logger) *Service {
	if log == nil {
		panic("documentapp.New: nil logger")
	}
	return &Service{repo: repo, emitter: emitter, log: log}
}

// SetRelationSyncer installs the relation Service post-construction (avoids an
// init cycle: relation needs document's Namer, document needs relation's syncer).
//
// SetRelationSyncer 装配后注入 relation Service（避免 init 环：relation 要 document 的
// Namer，document 要 relation 的 syncer）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

type (
	CreateInput = documentdomain.CreateInput
	UpdateInput = documentdomain.UpdateInput
	MoveInput   = documentdomain.MoveInput
)

// Create inserts a new document under parentID (nil = root). On a name collision it
// auto-uniquifies ("foo" → "foo 2" …) — a POST should never fail just because the
// name exists; an explicit rename (PATCH) keeps the strict conflict error.
//
// Create 在 parentID 下（nil = root）插入新文档。重名自动加后缀（"foo" → "foo 2" …）——POST
// 不该因重名失败；显式改名（PATCH）保留严格冲突错误。
func (s *Service) Create(ctx context.Context, in CreateInput) (*documentdomain.Document, error) {
	name := strings.TrimSpace(in.Name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateContent(in.Content); err != nil {
		return nil, err
	}

	parentPath := ""
	if in.ParentID != nil {
		parent, err := s.repo.Get(ctx, *in.ParentID)
		if errors.Is(err, documentdomain.ErrNotFound) {
			return nil, documentdomain.ErrParentNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("documentapp.Create: parent lookup: %w", err)
		}
		parentPath = parent.Path
	}

	maxPos, err := s.repo.MaxSiblingPosition(ctx, in.ParentID)
	if err != nil {
		return nil, fmt.Errorf("documentapp.Create: MaxSiblingPosition: %w", err)
	}

	tags := in.Tags
	if tags == nil {
		tags = []string{}
	}
	d := &documentdomain.Document{
		ID:          idgenpkg.New("doc"),
		ParentID:    in.ParentID,
		Description: strings.TrimSpace(in.Description),
		Content:     in.Content,
		Tags:        tags,
		Position:    maxPos + 1,
		SizeBytes:   int64(len(in.Content)),
	}

	const nameConflictRetryCap = 100
	attempted := name
	var insertErr error
	for retry := 1; retry <= nameConflictRetryCap; retry++ {
		d.Name = attempted
		d.Path = parentPath + "/" + attempted
		insertErr = s.repo.Insert(ctx, d)
		if insertErr == nil {
			break
		}
		if !errors.Is(insertErr, documentdomain.ErrNameConflict) {
			return nil, fmt.Errorf("documentapp.Create: %w", insertErr)
		}
		attempted = fmt.Sprintf("%s %d", name, retry+1)
	}
	if insertErr != nil {
		return nil, fmt.Errorf("documentapp.Create: %w", insertErr)
	}

	s.publish(ctx, "created", d)
	s.log.Info("document created", zap.String("doc_id", d.ID), zap.String("path", d.Path))
	s.syncRelationsForDocumentBody(ctx, d.ID, d.Content)
	return d, nil
}

func (s *Service) Get(ctx context.Context, id string) (*documentdomain.Document, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) GetBatch(ctx context.Context, ids []string) ([]*documentdomain.Document, error) {
	return s.repo.GetBatch(ctx, ids)
}

// ListByParent lists direct children of parentID (nil = root), position ASC.
//
// ListByParent 列 parentID 直接子节点（nil = root），position ASC。
func (s *Service) ListByParent(ctx context.Context, parentID *string) ([]*documentdomain.Document, error) {
	return s.repo.ListByParent(ctx, parentID)
}

// ListAll returns every live document (tree endpoint + catalog source).
//
// ListAll 返所有活跃文档（树端点 + catalog source）。
func (s *Service) ListAll(ctx context.Context) ([]*documentdomain.Document, error) {
	return s.repo.ListAll(ctx)
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]*documentdomain.Document, error) {
	return s.repo.Search(ctx, query, limit)
}

// Update applies a partial change; renaming triggers a subtree path cascade.
//
// Update 部分更新；改名触发整子树 path 级联。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*documentdomain.Document, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	renamed := false
	if in.Name != nil {
		newName := strings.TrimSpace(*in.Name)
		if err := validateName(newName); err != nil {
			return nil, err
		}
		if newName != d.Name {
			d.Name = newName
			renamed = true
		}
	}
	if in.Content != nil {
		if err := validateContent(*in.Content); err != nil {
			return nil, err
		}
		d.Content = *in.Content
		d.SizeBytes = int64(len(*in.Content))
	}
	if in.Description != nil {
		d.Description = strings.TrimSpace(*in.Description)
	}
	if in.Tags != nil {
		d.Tags = *in.Tags
	}

	if renamed {
		parentPath, err := s.parentPath(ctx, d.ParentID)
		if err != nil {
			return nil, fmt.Errorf("documentapp.Update: parent path: %w", err)
		}
		d.Path = parentPath + "/" + d.Name
	}

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("documentapp.Update: %w", err)
	}
	if renamed {
		if err := s.cascadePathSubtree(ctx, d.ID, d.Path); err != nil {
			s.log.Error("documentapp.Update: path cascade failed", zap.String("doc_id", d.ID), zap.Error(err))
		}
	}
	s.publish(ctx, "updated", d)
	if in.Content != nil {
		s.syncRelationsForDocumentBody(ctx, d.ID, d.Content)
	}
	return d, nil
}

// Move relocates the doc under a new parent (nil = root); refuses a cycle.
//
// Move 把 doc 挂到新父下（nil = root）；拒绝成环。
func (s *Service) Move(ctx context.Context, id string, in MoveInput) (*documentdomain.Document, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.ParentID != nil {
		if *in.ParentID == id {
			return nil, documentdomain.ErrInvalidParent
		}
		isDesc, err := s.repo.IsAncestor(ctx, id, *in.ParentID)
		if err != nil {
			return nil, fmt.Errorf("documentapp.Move: IsAncestor: %w", err)
		}
		if isDesc {
			return nil, documentdomain.ErrInvalidParent
		}
		if _, err := s.repo.Get(ctx, *in.ParentID); err != nil {
			if errors.Is(err, documentdomain.ErrNotFound) {
				return nil, documentdomain.ErrParentNotFound
			}
			return nil, fmt.Errorf("documentapp.Move: new parent lookup: %w", err)
		}
	}

	parentChanged := !samePtrString(d.ParentID, in.ParentID)
	d.ParentID = in.ParentID

	maxPos, err := s.repo.MaxSiblingPosition(ctx, in.ParentID)
	if err != nil {
		return nil, fmt.Errorf("documentapp.Move: MaxSiblingPosition: %w", err)
	}
	if in.Position == nil {
		d.Position = maxPos + 1
	} else {
		d.Position = *in.Position
	}

	if parentChanged {
		parentPath, err := s.parentPath(ctx, d.ParentID)
		if err != nil {
			return nil, fmt.Errorf("documentapp.Move: parent path: %w", err)
		}
		d.Path = parentPath + "/" + d.Name
	}

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("documentapp.Move: %w", err)
	}
	if parentChanged {
		if err := s.cascadePathSubtree(ctx, d.ID, d.Path); err != nil {
			s.log.Error("documentapp.Move: path cascade failed", zap.String("doc_id", d.ID), zap.Error(err))
		}
	}
	s.publish(ctx, "moved", d)
	return d, nil
}

// Delete soft-deletes the doc and all descendants atomically, then purges their edges.
//
// Delete 软删 doc 及全部后裔（原子），再清它们的边。
func (s *Service) Delete(ctx context.Context, id string) (int64, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	// Collect descendant IDs BEFORE soft-delete (after, they're filtered out).
	// 软删前收集后裔 ID（软删后会被过滤掉）。
	subtreeIDs, _ := s.repo.ListSubtreeIDs(ctx, id)
	n, err := s.repo.SoftDeleteSubtree(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("documentapp.Delete: %w", err)
	}
	s.publish(ctx, "deleted", d)
	s.log.Info("document deleted", zap.String("doc_id", d.ID), zap.Int64("deletedCount", n))
	s.purgeRelationsForDocuments(ctx, subtreeIDs)
	return n, nil
}

// CountDescendants backs the "delete will affect N children" confirmation.
//
// CountDescendants 支撑"删将影响 N 个子节点"二次确认。
func (s *Service) CountDescendants(ctx context.Context, id string) (int64, error) {
	return s.repo.CountDescendants(ctx, id)
}

// ResolveAttached expands AttachedDocument entries into a deduped slice of full
// Documents in declaration order — one doc per entry, no subtree expansion. Missing
// docs are dropped silently (caller decides whether to warn).
//
// ResolveAttached 把 AttachedDocument 列表展开为去重后的完整 Document 切片（保留声明顺序）
// ——每条一篇，不展开子树。缺失的 doc 静默跳过（调用方决定是否 warn）。
func (s *Service) ResolveAttached(ctx context.Context, atts []documentdomain.AttachedDocument) ([]*documentdomain.Document, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(atts))
	ids := make([]string, 0, len(atts))
	for _, a := range atts {
		if a.DocumentID == "" || seen[a.DocumentID] {
			continue
		}
		seen[a.DocumentID] = true
		ids = append(ids, a.DocumentID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	docs, err := s.repo.GetBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("documentapp.ResolveAttached: %w", err)
	}
	return docs, nil
}

// RenderAttachedAsXML serializes resolved docs into the <documents>…</documents>
// segment shared by chat system prompt + workflow llm/agent dispatcher.
//
// RenderAttachedAsXML 把已 resolve 的 docs 渲染成 <documents>…</documents> 段，chat system
// prompt + workflow llm/agent dispatcher 共用。
func RenderAttachedAsXML(docs []*documentdomain.Document) string {
	if len(docs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<documents>\n")
	for _, d := range docs {
		fmt.Fprintf(&sb, "<document path=%q id=%q>\n", d.Path, d.ID)
		sb.WriteString(d.Content)
		sb.WriteString("\n</document>\n")
	}
	sb.WriteString("</documents>\n")
	return sb.String()
}

// parentPath returns the parent's path, or "" for a root-level doc.
//
// parentPath 返回父节点的 path，根级返 ""。
func (s *Service) parentPath(ctx context.Context, parentID *string) (string, error) {
	if parentID == nil {
		return "", nil
	}
	parent, err := s.repo.Get(ctx, *parentID)
	if err != nil {
		return "", err
	}
	return parent.Path, nil
}

// cascadePathSubtree rebuilds Path for every descendant after a rename / move.
//
// cascadePathSubtree 在 rename / move 后重算整子树 Path。
func (s *Service) cascadePathSubtree(ctx context.Context, rootID, rootPath string) error {
	type pathed struct{ id, path string }
	queue := []pathed{{id: rootID, path: rootPath}}
	var updates []*documentdomain.Document
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curID := cur.id
		kids, err := s.repo.ListByParent(ctx, &curID)
		if err != nil {
			return err
		}
		for _, kid := range kids {
			kid.Path = cur.path + "/" + kid.Name
			updates = append(updates, kid)
			queue = append(queue, pathed{id: kid.ID, path: kid.Path})
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return s.repo.UpdateBatch(ctx, updates)
}

// publish emits a document lifecycle notification (best-effort).
//
// publish 发送文档生命周期通知（best-effort）。
func (s *Service) publish(ctx context.Context, action string, d *documentdomain.Document) {
	if s.emitter == nil {
		return
	}
	payload := map[string]any{"documentId": d.ID, "path": d.Path}
	if d.ParentID != nil {
		payload["parentId"] = *d.ParentID
	}
	if err := s.emitter.Emit(ctx, "document."+action, payload); err != nil {
		s.log.Warn("document: emit notification failed", zap.String("action", action), zap.Error(err))
	}
}

func validateName(name string) error {
	if name == "" || len(name) > documentdomain.MaxNameLength {
		return documentdomain.ErrInvalidName
	}
	// A path separator would corrupt the dotted-path scheme.
	// 路径分隔符会污染 path 字段拼接。
	if strings.ContainsRune(name, '/') {
		return documentdomain.ErrInvalidName
	}
	return nil
}

func validateContent(content string) error {
	if len(content) > documentdomain.MaxContentBytes {
		return documentdomain.ErrContentTooLarge
	}
	return nil
}

func samePtrString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
