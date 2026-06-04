// Package relation (app layer) implements relationdomain.Service: idempotent
// diff-sync edge writes, cascade purge, neighborhood BFS, and the relgraph
// snapshot. Read paths return hydrated views — display names are looked up fresh
// in memory (per-kind batch via injected Namers), never stored, so a renamed
// entity always shows current. Workspace isolation is handled by the orm layer,
// so this layer passes no workspace id.
//
// Package relation（app 层）实现 relationdomain.Service：幂等 diff-sync 边写入、级联 purge、
// 邻域 BFS、relgraph 快照。读路径返回 hydrate 视图——显示名读时在内存现查（经注入的 Namer 按
// kind 批量），从不入库，故改名后永远显示最新。workspace 隔离由 orm 层处理，本层不传 workspace id。
package relation

import (
	"context"
	"fmt"
	"maps"
	"reflect"

	"go.uber.org/zap"

	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// Namer resolves display names for a batch of one kind's entity ids. Each source
// domain implements it (one query: WHERE id IN …, pluck name) and is injected
// post-construction (波次 3). A missing/nil namer for a kind contributes no names,
// and those ids fall back to displaying themselves.
//
// Namer 批量解析某一类实体 id 的显示名。各 source domain 实现（一条查询：WHERE id IN… 取
// name），装配后注入（波次 3）。某 kind 无 namer 时不贡献名字，这些 id 回退为显示自身。
type Namer interface {
	NamesByIDs(ctx context.Context, ids []string) (map[string]string, error)
}

// Service implements relationdomain.Service.
//
// Service 实现 relationdomain.Service。
type Service struct {
	repo   relationdomain.Repository
	namers map[string]Namer // EntityKind → Namer; missing = no names for that kind
	log    *zap.Logger
}

// Config bundles dependencies; Namers may be partial or nil (kinds without a namer
// just show ids).
//
// Config 打包依赖；Namers 可部分或 nil（无 namer 的 kind 仅显示 id）。
type Config struct {
	Repo   relationdomain.Repository
	Namers map[string]Namer
	Log    *zap.Logger
}

// NewService wires the Service; Repo + Log required.
//
// NewService 装配 Service；Repo + Log 必填。
func NewService(cfg Config) *Service {
	if cfg.Repo == nil {
		panic("relationapp.NewService: Repo is nil")
	}
	if cfg.Log == nil {
		panic("relationapp.NewService: Log is nil")
	}
	namers := cfg.Namers
	if namers == nil {
		namers = map[string]Namer{}
	}
	return &Service{repo: cfg.Repo, namers: namers, log: cfg.Log}
}

var _ relationdomain.Service = (*Service)(nil)

// --- write side (called by source-domain sync hooks, 波次 2/3/5) ---

func (s *Service) SyncOutgoing(ctx context.Context, fromKind, fromID string, kindScope []string, edges []relationdomain.SyncEdge) error {
	if err := s.validateSync(fromKind, fromID, kindScope, edges); err != nil {
		return fmt.Errorf("relationapp.SyncOutgoing: %w", err)
	}
	existing, err := s.repo.ListByFromAndKinds(ctx, fromKind, fromID, kindScope)
	if err != nil {
		return fmt.Errorf("relationapp.SyncOutgoing: %w", err)
	}
	return s.diffSync(ctx, existing, edges, fromKind, fromID, true)
}

func (s *Service) SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []relationdomain.SyncEdge) error {
	if err := s.validateSync(toKind, toID, kindScope, edges); err != nil {
		return fmt.Errorf("relationapp.SyncIncoming: %w", err)
	}
	existing, err := s.repo.ListByToAndKinds(ctx, toKind, toID, kindScope)
	if err != nil {
		return fmt.Errorf("relationapp.SyncIncoming: %w", err)
	}
	return s.diffSync(ctx, existing, edges, toKind, toID, false)
}

func (s *Service) PurgeEntity(ctx context.Context, kind, id string) error {
	if err := validateEntityRef(kind, id); err != nil {
		return fmt.Errorf("relationapp.PurgeEntity: %w", err)
	}
	n, err := s.repo.PurgeEntity(ctx, kind, id)
	if err != nil {
		return fmt.Errorf("relationapp.PurgeEntity: %w", err)
	}
	if n > 0 {
		s.log.Info("relation purge", zap.String("kind", kind), zap.String("id", id), zap.Int64("removed", n))
	}
	return nil
}

// validateSync checks the fixed ref, the kind scope, and every edge (ref, kind, no
// self-loop) up front, before any DB work.
//
// validateSync 在碰 DB 前先校验固定端 ref、kind scope、以及每条边（ref、kind、无自环）。
func (s *Service) validateSync(fixedKind, fixedID string, kindScope []string, edges []relationdomain.SyncEdge) error {
	if err := validateEntityRef(fixedKind, fixedID); err != nil {
		return err
	}
	for _, k := range kindScope {
		if !relationdomain.IsValidKind(k) {
			return relationdomain.ErrInvalidKind
		}
	}
	for _, e := range edges {
		if err := validateEntityRef(e.OtherKind, e.OtherID); err != nil {
			return err
		}
		if !relationdomain.IsValidKind(e.Kind) {
			return relationdomain.ErrInvalidKind
		}
		if fixedKind == e.OtherKind && fixedID == e.OtherID {
			return relationdomain.ErrSelfLoop
		}
	}
	return nil
}

// diffSync is the shared diff-and-apply core. With outgoing=true the fixed ref is
// the FROM side and OtherKind/OtherID the TO; with outgoing=false it is reversed.
// New edges are inserted, attrs-changed edges updated, vanished edges deleted — the
// workspace's edges in scope end exactly matching `want`.
//
// diffSync 是共享的 diff-and-apply 内核。outgoing=true 时固定端为 FROM、Other 为 TO；
// outgoing=false 反之。新边插入、attrs 变更的更新、消失的删除——scope 内边集最终恰为 want。
func (s *Service) diffSync(ctx context.Context, existing []*relationdomain.Relation, want []relationdomain.SyncEdge, fixedKind, fixedID string, outgoing bool) error {
	type edgeKey struct{ otherKind, otherID, kind string }

	existingByKey := make(map[edgeKey]*relationdomain.Relation, len(existing))
	for _, r := range existing {
		if outgoing {
			existingByKey[edgeKey{r.ToKind, r.ToID, r.Kind}] = r
		} else {
			existingByKey[edgeKey{r.FromKind, r.FromID, r.Kind}] = r
		}
	}
	wantByKey := make(map[edgeKey]relationdomain.SyncEdge, len(want))
	for _, e := range want {
		wantByKey[edgeKey{e.OtherKind, e.OtherID, e.Kind}] = e
	}

	var (
		toInsert    []*relationdomain.Relation
		toUpdateID  []string
		toUpdateMap []map[string]any
		toDeleteIDs []string
	)
	for k, e := range wantByKey {
		if r, found := existingByKey[k]; found {
			if !attrsEqual(r.Attrs, e.Attrs) {
				toUpdateID = append(toUpdateID, r.ID)
				toUpdateMap = append(toUpdateMap, e.Attrs)
			}
			continue
		}
		nr := &relationdomain.Relation{ID: newID(), Kind: e.Kind, Attrs: e.Attrs}
		if outgoing {
			nr.FromKind, nr.FromID = fixedKind, fixedID
			nr.ToKind, nr.ToID = e.OtherKind, e.OtherID
		} else {
			nr.FromKind, nr.FromID = e.OtherKind, e.OtherID
			nr.ToKind, nr.ToID = fixedKind, fixedID
		}
		toInsert = append(toInsert, nr)
	}
	for k, r := range existingByKey {
		if _, keep := wantByKey[k]; !keep {
			toDeleteIDs = append(toDeleteIDs, r.ID)
		}
	}

	if len(toInsert) > 0 {
		if err := s.repo.InsertBatch(ctx, toInsert); err != nil {
			return fmt.Errorf("diffSync insert: %w", err)
		}
	}
	for i, id := range toUpdateID {
		if err := s.repo.UpdateAttrs(ctx, id, toUpdateMap[i]); err != nil {
			return fmt.Errorf("diffSync update: %w", err)
		}
	}
	if len(toDeleteIDs) > 0 {
		if err := s.repo.DeleteByIDs(ctx, toDeleteIDs); err != nil {
			return fmt.Errorf("diffSync delete: %w", err)
		}
	}
	return nil
}

// --- read side (HTTP endpoints) ---

func (s *Service) List(ctx context.Context, filter relationdomain.Filter, cursor string, limit int) ([]*relationdomain.RelationView, string, error) {
	if err := validateFilter(filter); err != nil {
		return nil, "", fmt.Errorf("relationapp.List: %w", err)
	}
	rows, next, err := s.repo.List(ctx, filter, cursor, limit)
	if err != nil {
		return nil, "", fmt.Errorf("relationapp.List: %w", err)
	}
	views, err := s.hydrateNames(ctx, rows)
	if err != nil {
		return nil, "", fmt.Errorf("relationapp.List: %w", err)
	}
	return views, next, nil
}

func (s *Service) Neighborhood(ctx context.Context, kind, id string, depth int) ([]*relationdomain.RelationView, error) {
	if err := validateEntityRef(kind, id); err != nil {
		return nil, fmt.Errorf("relationapp.Neighborhood: %w", err)
	}
	if depth < relationdomain.MinNeighborhoodDepth || depth > relationdomain.MaxNeighborhoodDepth {
		return nil, fmt.Errorf("relationapp.Neighborhood: %w", relationdomain.ErrDepthOutOfRange)
	}

	type ref struct{ k, i string }
	visited := map[ref]bool{{k: kind, i: id}: true}
	edgesSeen := map[string]bool{}
	var collected []*relationdomain.Relation
	frontier := []ref{{k: kind, i: id}}

	for range depth {
		var nextFrontier []ref
		for _, e := range frontier {
			outgoing, err := s.repo.ListByFromAndKinds(ctx, e.k, e.i, nil)
			if err != nil {
				return nil, fmt.Errorf("relationapp.Neighborhood: %w", err)
			}
			incoming, err := s.repo.ListByToAndKinds(ctx, e.k, e.i, nil)
			if err != nil {
				return nil, fmt.Errorf("relationapp.Neighborhood: %w", err)
			}
			for _, r := range outgoing {
				if !edgesSeen[r.ID] {
					edgesSeen[r.ID] = true
					collected = append(collected, r)
				}
				if nb := (ref{k: r.ToKind, i: r.ToID}); !visited[nb] {
					visited[nb] = true
					nextFrontier = append(nextFrontier, nb)
				}
			}
			for _, r := range incoming {
				if !edgesSeen[r.ID] {
					edgesSeen[r.ID] = true
					collected = append(collected, r)
				}
				if nb := (ref{k: r.FromKind, i: r.FromID}); !visited[nb] {
					visited[nb] = true
					nextFrontier = append(nextFrontier, nb)
				}
			}
		}
		frontier = nextFrontier
		if len(frontier) == 0 {
			break
		}
	}
	return s.hydrateNames(ctx, collected)
}

func (s *Service) GetRelgraph(ctx context.Context) (*relationdomain.Snapshot, error) {
	edges, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("relationapp.GetRelgraph: %w", err)
	}
	views, err := s.hydrateNames(ctx, edges)
	if err != nil {
		return nil, fmt.Errorf("relationapp.GetRelgraph: %w", err)
	}

	// Nodes are the distinct entities at edge endpoints — isolated entities never appear.
	// 节点是边端点处的去重实体——孤立实体不出现。
	type nodeKey struct{ kind, id string }
	seen := map[nodeKey]bool{}
	nodes := make([]relationdomain.Node, 0)
	addNode := func(kind, id, name string) {
		k := nodeKey{kind, id}
		if seen[k] {
			return
		}
		seen[k] = true
		nodes = append(nodes, relationdomain.Node{Kind: kind, ID: id, Name: name})
	}
	for _, v := range views {
		addNode(v.FromKind, v.FromID, v.FromName)
		addNode(v.ToKind, v.ToID, v.ToName)
	}
	return &relationdomain.Snapshot{Nodes: nodes, Edges: views}, nil
}

// hydrateNames decorates edges with current display names, looked up per-kind in
// one batch each. A name missing (kind has no namer, or entity gone) falls back to
// the raw id — which also covers references to deleted entities.
//
// hydrateNames 给边补上当前显示名，按 kind 各批量查一次。缺名（该 kind 无 namer 或实体已删）
// 回退为原始 id——这也覆盖了指向已删实体的引用。
func (s *Service) hydrateNames(ctx context.Context, edges []*relationdomain.Relation) ([]*relationdomain.RelationView, error) {
	idsByKind := map[string]map[string]struct{}{}
	mark := func(kind, id string) {
		set := idsByKind[kind]
		if set == nil {
			set = map[string]struct{}{}
			idsByKind[kind] = set
		}
		set[id] = struct{}{}
	}
	for _, e := range edges {
		mark(e.FromKind, e.FromID)
		mark(e.ToKind, e.ToID)
	}

	nameOf := map[string]string{}
	for kind, set := range idsByKind {
		namer := s.namers[kind]
		if namer == nil {
			continue
		}
		ids := make([]string, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		names, err := namer.NamesByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("hydrate %s names: %w", kind, err)
		}
		maps.Copy(nameOf, names)
	}

	nameOrID := func(id string) string {
		if n, ok := nameOf[id]; ok && n != "" {
			return n
		}
		return id
	}
	views := make([]*relationdomain.RelationView, len(edges))
	for i, e := range edges {
		views[i] = &relationdomain.RelationView{
			Relation: *e,
			FromName: nameOrID(e.FromID),
			ToName:   nameOrID(e.ToID),
		}
	}
	return views, nil
}

// --- validation helpers ---

func validateEntityRef(kind, id string) error {
	if kind == "" || id == "" || !relationdomain.IsValidEntityKind(kind) {
		return relationdomain.ErrInvalidRef
	}
	return nil
}

func validateFilter(f relationdomain.Filter) error {
	if (f.FromKind == "") != (f.FromID == "") {
		return relationdomain.ErrIncompleteFilter
	}
	if (f.ToKind == "") != (f.ToID == "") {
		return relationdomain.ErrIncompleteFilter
	}
	if f.FromKind != "" && !relationdomain.IsValidEntityKind(f.FromKind) {
		return relationdomain.ErrInvalidRef
	}
	if f.ToKind != "" && !relationdomain.IsValidEntityKind(f.ToKind) {
		return relationdomain.ErrInvalidRef
	}
	if f.Kind != "" && !relationdomain.IsValidKind(f.Kind) {
		return relationdomain.ErrInvalidKind
	}
	return nil
}

func attrsEqual(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func newID() string { return idgenpkg.New("rel") }
