package search

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	// fusionWindow is the materialized result window: RRF/boost scores are not
	// stable across separate queries, so pagination slices ONE deterministic
	// top-200 instead of re-ranking per page (§6.2).
	// fusionWindow 是物化结果窗口：RRF/boost 分跨查询不稳定，分页在单次确定性的
	// top-200 上切片而非逐页重排（§6.2）。
	fusionWindow = 200
	defaultLimit = 20
	maxLimit     = 50

	// schemaVersion guards the index layout: a mismatch at boot drops and
	// rebuilds — the index is derived data, never migrated in place.
	// schemaVersion 守护索引布局：boot 不匹配即清空重建——索引是派生数据，从不原地迁移。
	schemaVersion      = "1"
	metaSchemaKey      = "fts_schema_version"
	metaEmbedderKey    = "embedder"
	metaOllamaURLKey   = "ollama_base_url"
	metaOllamaModelKey = "ollama_model"
)

// §6.3 ranking constants — initial values, tests assert relative order only.
//
// §6.3 排序常量——初始值，测试只断言相对序。
const (
	boostExactName  = 3.0
	boostNamePrefix = 1.5
	boostBlockType  = 0.3
)

// Page is one search result page (N4 cursor pagination over the fused window).
//
// Page 是一页检索结果（N4 cursor 分页，作用在融合窗口上）。
type Page struct {
	Hits       []*searchdomain.Hit `json:"hits"`
	NextCursor string              `json:"nextCursor,omitempty"`
	Total      int                 `json:"total"`
}

// Service is the one search engine behind every surface.
//
// Service 是所有出口背后的同一个检索引擎。
type Service struct {
	repo    searchdomain.Repository
	sources map[searchdomain.EntityType]Source
	indexer *Indexer
	log     *zap.Logger

	reindexing atomic.Bool

	// Semantic layer (§8): the builtin adapter + an Ollama factory; the active one is
	// resolved per call from search_meta, and the Ollama adapter is rebuilt (cached under
	// ollamaMu) whenever the stored connection params change. Vectors cached per
	// workspace; backfill runs on its own worker.
	// 语义层（§8）：builtin 适配器 + Ollama 工厂；生效者按 search_meta 逐次解析，Ollama
	// 适配器在存储连接参数变化时重建（ollamaMu 下缓存）。向量按 workspace 缓存；补算走独立 worker。
	builtinProv   searchdomain.EmbeddingProvider
	ollamaFactory OllamaFactory
	ollamaMu      sync.Mutex
	ollamaProv    searchdomain.EmbeddingProvider // cache for the current params key. 当前参数键的缓存。
	ollamaKey     string
	sifter        Sifter // nil → precision chain tier 3 only. nil → 精度链只剩第三档。
	vectors       *vecCache
	embedKick     chan string
	embedQuit     chan struct{}
}

// New builds the Service; register sources before Start.
//
// New 构造 Service；Start 前注册 source。
func New(repo searchdomain.Repository, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		repo:      repo,
		sources:   map[searchdomain.EntityType]Source{},
		log:       log,
		vectors:   newVecCache(),
		embedKick: make(chan string, embedKickQueue),
		embedQuit: make(chan struct{}),
	}
	s.indexer = newIndexer(repo, s.sources, log)
	s.indexer.onApplied = s.kickEmbed
	return s
}

// RegisterSource plugs one entity projection in (bootstrap, before Start).
//
// RegisterSource 接入一个实体投影（bootstrap 期、Start 前）。
func (s *Service) RegisterSource(src Source) {
	s.sources[src.Type()] = src
}

// Notifier exposes the write-side hook entity Services hold.
//
// Notifier 暴露实体 Service 持有的写侧钩子。
func (s *Service) Notifier() searchdomain.Notifier { return s.indexer }

// Start brings the worker up, rebuilds on schema-version mismatch, then
// reconciles every workspace in the background — boot is never blocked.
//
// Start 拉起 worker，schema 版本不匹配则重建，然后后台对账所有 workspace——
// 绝不阻塞 boot。
func (s *Service) Start(workspaceIDs []string) {
	s.indexer.start()
	go s.embedWorker()
	go func() {
		ctx := reqctxpkg.Detached("")
		v, err := s.repo.GetMeta(ctx, metaSchemaKey)
		if err != nil {
			s.log.Warn("search: schema version read failed", zap.Error(err))
		}
		if v != schemaVersion {
			if err := s.repo.DropAll(ctx); err != nil {
				s.log.Warn("search: drop-all failed", zap.Error(err))
				return
			}
			if err := s.repo.SetMeta(ctx, metaSchemaKey, schemaVersion); err != nil {
				s.log.Warn("search: schema version write failed", zap.Error(err))
			}
		}
		for _, ws := range workspaceIDs {
			s.indexer.reconcile(reqctxpkg.Detached(ws), ws)
			s.kickEmbed(ws)
		}
	}()
}

// Close stops the index worker, the embed worker and any provider subprocess.
//
// Close 停索引 worker、嵌入 worker 与 provider 子进程。
func (s *Service) Close() {
	close(s.embedQuit)
	s.indexer.close()
	for _, p := range []searchdomain.EmbeddingProvider{s.builtinProv, s.ollamaProv} {
		if c, ok := p.(ProviderCloser); ok {
			c.Close()
		}
	}
}

// ReconcileWorkspace re-diffs one workspace on demand (workspace switch /
// tests).
//
// ReconcileWorkspace 按需重对账一个 workspace（切 workspace / 测试）。
func (s *Service) ReconcileWorkspace(ctx context.Context, wsID string) {
	s.indexer.reconcile(ctx, wsID)
}

// Reindex purges and rebuilds the ctx workspace asynchronously (202
// semantics); a second call while one runs is a conflict.
//
// Reindex 异步清空重建 ctx workspace（202 语义）；运行中再触发即冲突。
func (s *Service) Reindex(ctx context.Context) error {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return err
	}
	if !s.reindexing.CompareAndSwap(false, true) {
		return searchdomain.ErrReindexRunning
	}
	go func() {
		defer s.reindexing.Store(false)
		dctx := reqctxpkg.Detached(wsID)
		if err := s.repo.PurgeWorkspace(dctx, wsID); err != nil {
			s.log.Warn("search reindex: purge failed", zap.Error(err))
			return
		}
		s.vectors.invalidate(wsID)
		s.indexer.reconcile(dctx, wsID)
		s.kickEmbed(wsID)
	}()
	return nil
}

// PurgeWorkspace is the workspace-deletion cascade hook.
//
// PurgeWorkspace 是 workspace 删除级联钩子。
func (s *Service) PurgeWorkspace(ctx context.Context, wsID string) error {
	s.vectors.invalidate(wsID)
	return s.repo.PurgeWorkspace(ctx, wsID)
}

// Search runs the full §6 pipeline: validate → token routing → lexical top-N →
// boost → fold per entity → deterministic window → cursor slice.
//
// Search 跑完整 §6 管线：校验 → token 路由 → 词法 top-N → 加权 → 按实体折叠 →
// 确定性窗口 → cursor 切片。
func (s *Service) Search(ctx context.Context, q *searchdomain.Query) (*Page, error) {
	if strings.TrimSpace(q.Q) == "" {
		return nil, searchdomain.ErrQueryRequired
	}
	for _, t := range q.Types {
		if !searchdomain.IsValidEntityType(t) {
			return nil, searchdomain.ErrTypeInvalid
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := 0
	if q.Cursor != "" {
		o, err := decodeCursor(q.Cursor, queryHash(q))
		if err != nil {
			return nil, err
		}
		offset = o
	}

	hits, err := s.window(ctx, q, true)
	if err != nil {
		return nil, err
	}

	total := len(hits)
	if offset >= total {
		return &Page{Hits: []*searchdomain.Hit{}, Total: total}, nil
	}
	end := min(offset+limit, total)
	page := &Page{Hits: hits[offset:end], Total: total}
	if end < total {
		page.NextCursor = encodeCursor(queryHash(q), end)
	}
	return page, nil
}

// window produces the deterministic fused window: chunk hits → entity folding
// (or (entity, anchor) folding for the block palette) → boosts → stable order.
//
// window 产出确定性融合窗口：chunk 命中 → 实体折叠（积木面板按 (entity, anchor)
// 折叠）→ 加权 → 稳定排序。
func (s *Service) window(ctx context.Context, q *searchdomain.Query, foldByEntity bool) ([]*searchdomain.Hit, error) {
	parsed := searchdomain.ParseQuery(q.Q)
	lex, err := s.repo.SearchLexical(ctx, searchdomain.LexicalQuery{
		LongTokens:      parsed.Long,
		ShortTokens:     parsed.Short,
		Types:           q.Types,
		Tags:            q.Tags,
		IncludeArchived: q.IncludeArchived,
		UpdatedAfter:    q.UpdatedAfter,
		UpdatedBefore:   q.UpdatedBefore,
		Limit:           fusionWindow,
	})
	if err != nil {
		return nil, fmt.Errorf("search: lexical: %w", err)
	}
	lex = s.fuseSemantic(ctx, q, lex)
	hits := fold(lex, foldByEntity)
	boost(hits, q.Q)
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if !hits[i].UpdatedAt.Equal(hits[j].UpdatedAt) {
			return hits[i].UpdatedAt.After(hits[j].UpdatedAt)
		}
		return hits[i].EntityID < hits[j].EntityID
	})
	if len(hits) > fusionWindow {
		hits = hits[:fusionWindow]
	}
	return hits, nil
}

// fold groups chunk hits: per entity for the search box (best chunk wins,
// siblings counted), per (entity, anchor) for the block palette where each
// method/tool IS the result unit.
//
// fold 聚合 chunk 命中：综搜按实体（最高分 chunk 胜出、计同实体命中数），积木面板
// 按 (entity, anchor)——每个方法/工具本身就是结果单元。
func fold(lex []*searchdomain.DocHit, byEntity bool) []*searchdomain.Hit {
	type key struct {
		t      searchdomain.EntityType
		id     string
		anchor string
	}
	order := []key{}
	grouped := map[key]*searchdomain.Hit{}
	for _, dh := range lex {
		k := key{t: dh.EntityType, id: dh.EntityID}
		if !byEntity {
			k.anchor = dh.Anchor
		}
		if h, ok := grouped[k]; ok {
			h.MatchedChunks++
			if dh.Score > h.Score {
				h.Name, h.Snippet, h.Anchor, h.Score = dh.Title, dh.Snippet, dh.Anchor, dh.Score
				h.RefHint = searchdomain.RefHint(dh.EntityType, dh.EntityID, dh.Anchor)
			}
			if dh.UpdatedAt.After(h.UpdatedAt) {
				h.UpdatedAt = dh.UpdatedAt
			}
			continue
		}
		h := &searchdomain.Hit{
			EntityType:    dh.EntityType,
			EntityID:      dh.EntityID,
			Name:          dh.Title,
			Snippet:       dh.Snippet,
			Anchor:        dh.Anchor,
			Tags:          dh.Tags,
			Archived:      dh.Archived,
			Score:         dh.Score,
			MatchedChunks: 1,
			UpdatedAt:     dh.UpdatedAt,
			RefHint:       searchdomain.RefHint(dh.EntityType, dh.EntityID, dh.Anchor),
		}
		grouped[k] = h
		order = append(order, k)
	}
	out := make([]*searchdomain.Hit, 0, len(order))
	for _, k := range order {
		out = append(out, grouped[k])
	}
	return out
}

// boost applies §6.3: normalize the base to [0,1] (scale-free vs bm25/RRF),
// then exact-name > prefix > nothing, plus a small block-over-content nudge.
//
// boost 应用 §6.3：基底归一到 [0,1]（对 bm25/RRF 尺度无感），再叠 exact-name >
// prefix，外加积木类对内容类的小幅倾斜。
func boost(hits []*searchdomain.Hit, rawQuery string) {
	var maxScore float64
	for _, h := range hits {
		if h.Score > maxScore {
			maxScore = h.Score
		}
	}
	qn := strings.ToLower(strings.TrimSpace(rawQuery))
	for _, h := range hits {
		if maxScore > 0 {
			h.Score = h.Score / maxScore
		}
		name := strings.ToLower(h.Name)
		switch {
		case name == qn:
			h.Score += boostExactName
		case strings.HasPrefix(name, qn):
			h.Score += boostNamePrefix
		}
		if searchdomain.IsBlockEntityType(h.EntityType) {
			h.Score += boostBlockType
		}
	}
}

// queryHash fingerprints everything that shapes the window, so a cursor from a
// different query (or filter set) is rejected instead of slicing the wrong
// window.
//
// queryHash 给塑造窗口的全部参数取指纹，使来自不同查询（或过滤集）的 cursor 被拒，
// 而不是切错窗口。
func queryHash(q *searchdomain.Query) string {
	var sb strings.Builder
	sb.WriteString(q.Q)
	for _, t := range q.Types {
		sb.WriteString("|t:" + string(t))
	}
	for _, t := range q.Tags {
		sb.WriteString("|g:" + t)
	}
	if q.IncludeArchived {
		sb.WriteString("|a")
	}
	if q.UpdatedAfter != nil {
		sb.WriteString("|>" + q.UpdatedAfter.UTC().String())
	}
	if q.UpdatedBefore != nil {
		sb.WriteString("|<" + q.UpdatedBefore.UTC().String())
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:6])
}

type cursorPayload struct {
	H string `json:"h"`
	O int    `json:"o"`
}

func encodeCursor(hash string, offset int) string {
	b, _ := json.Marshal(cursorPayload{H: hash, O: offset})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(cursor, wantHash string) (int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, searchdomain.ErrCursorInvalid
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.H != wantHash || p.O < 0 {
		return 0, searchdomain.ErrCursorInvalid
	}
	return p.O, nil
}
