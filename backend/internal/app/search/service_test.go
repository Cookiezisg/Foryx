package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeRepo records writes and serves canned lexical hits — app-layer tests pin
// ranking/folding/pagination logic, the SQL layer has its own tests.
//
// fakeRepo 记录写入并返回预置词法命中——app 层测试钉排序/折叠/分页逻辑，SQL 层有
// 自己的测试。
type fakeRepo struct {
	mu       sync.Mutex
	hits     []*searchdomain.DocHit
	replaced map[string][]searchdomain.SourceDoc // key: type/id
	upserted map[string]searchdomain.SourceDoc   // key: type/id#chunk
	stamps   map[searchdomain.EntityType]map[string]time.Time
	meta     map[string]string
	purged   []string
	purgeGo  chan struct{} // non-nil → PurgeWorkspace blocks until closed. 非 nil → PurgeWorkspace 阻塞到关闭。

	embedded   map[string][]float32 // key: model/docID
	embedQueue []searchdomain.EmbedDoc
	docsByID   map[string]*searchdomain.DocHit
	bodies     map[string]string
	blockRows  []*searchdomain.DocHit
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		replaced: map[string][]searchdomain.SourceDoc{},
		upserted: map[string]searchdomain.SourceDoc{},
		stamps:   map[searchdomain.EntityType]map[string]time.Time{},
		meta:     map[string]string{},
	}
}

func (f *fakeRepo) ReplaceDocs(_ context.Context, t searchdomain.EntityType, id string, docs []searchdomain.SourceDoc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaced[string(t)+"/"+id] = docs
	return nil
}

func (f *fakeRepo) UpsertDocAt(_ context.Context, t searchdomain.EntityType, id string, d searchdomain.SourceDoc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted[string(t)+"/"+id+"#"+d.Anchor] = d
	return nil
}

func (f *fakeRepo) DeleteEntity(ctx context.Context, t searchdomain.EntityType, id string) error {
	return f.ReplaceDocs(ctx, t, id, nil)
}

func (f *fakeRepo) PurgeWorkspace(_ context.Context, ws string) error {
	if f.purgeGo != nil {
		<-f.purgeGo
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purged = append(f.purged, ws)
	return nil
}

func (f *fakeRepo) SearchLexical(_ context.Context, _ searchdomain.LexicalQuery) ([]*searchdomain.DocHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits, nil
}

func (f *fakeRepo) EntityStamps(_ context.Context, t searchdomain.EntityType) (map[string]time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]time.Time{}
	for id, ts := range f.stamps[t] {
		out[id] = ts
	}
	return out, nil
}

func (f *fakeRepo) GetMeta(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.meta[key], nil
}

func (f *fakeRepo) SetMeta(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.meta[key] = value
	return nil
}

func (f *fakeRepo) DropAll(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits = nil
	return nil
}

var _ searchdomain.Repository = (*fakeRepo)(nil)

func dh(t searchdomain.EntityType, id string, chunk int, anchor, title string, score float64) *searchdomain.DocHit {
	return &searchdomain.DocHit{
		EntityType: t, EntityID: id, ChunkNo: chunk, Anchor: anchor, Title: title,
		Score: score, UpdatedAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
	}
}

func ctxWS(ws string) context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), ws) }

func TestSearch_BoostRelativeOrder(t *testing.T) {
	repo := newFakeRepo()
	// Body-only hit scores highest lexically — exact/prefix name must still win.
	// 仅正文命中词法分最高——exact/prefix 名仍必须排前。
	repo.hits = []*searchdomain.DocHit{
		dh(searchdomain.TypeDocument, "doc_body", 0, "", "无关标题", 9.0),
		dh(searchdomain.TypeFunction, "fn_prefix", 0, "", "天气预报增强版", 2.0),
		dh(searchdomain.TypeFunction, "fn_exact", 0, "", "天气预报", 1.0),
	}
	svc := New(repo, nil)
	page, err := svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "天气预报", IncludeArchived: true})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	got := []string{page.Hits[0].EntityID, page.Hits[1].EntityID, page.Hits[2].EntityID}
	want := []string{"fn_exact", "fn_prefix", "doc_body"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("relative order broken: got %v want %v", got, want)
		}
	}
	if page.Hits[0].RefHint != "fn_exact" {
		t.Fatalf("block refHint missing: %+v", page.Hits[0])
	}
	if page.Hits[2].RefHint != "" {
		t.Fatalf("content hit must have no refHint: %+v", page.Hits[2])
	}
}

func TestSearch_FoldsChunksPerEntity(t *testing.T) {
	repo := newFakeRepo()
	repo.hits = []*searchdomain.DocHit{
		dh(searchdomain.TypeDocument, "doc_1", 2, "h2", "设计稿", 5.0),
		dh(searchdomain.TypeDocument, "doc_1", 0, "h0", "设计稿", 8.0),
		dh(searchdomain.TypeDocument, "doc_1", 1, "h1", "设计稿", 3.0),
		dh(searchdomain.TypeDocument, "doc_2", 0, "", "另一篇", 4.0),
	}
	svc := New(repo, nil)
	page, err := svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "设计", IncludeArchived: true})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Hits) != 2 {
		t.Fatalf("fold broken: %d hits", len(page.Hits))
	}
	if page.Hits[0].MatchedChunks != 3 || page.Hits[0].Anchor != "h0" {
		t.Fatalf("best chunk must win with sibling count: %+v", page.Hits[0])
	}
}

func TestSearch_CursorPagination(t *testing.T) {
	repo := newFakeRepo()
	for i := range 30 {
		repo.hits = append(repo.hits, dh(searchdomain.TypeFunction, "fn_"+string(rune('a'+i)), 0, "", "工具函数", float64(30-i)))
	}
	svc := New(repo, nil)
	q := &searchdomain.Query{Q: "工具", Limit: 10, IncludeArchived: true}
	p1, err := svc.Search(ctxWS("ws_a"), q)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(p1.Hits) != 10 || p1.NextCursor == "" || p1.Total != 30 {
		t.Fatalf("page1 wrong: %d hits, cursor %q, total %d", len(p1.Hits), p1.NextCursor, p1.Total)
	}
	q.Cursor = p1.NextCursor
	p2, err := svc.Search(ctxWS("ws_a"), q)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(p2.Hits) != 10 || p2.Hits[0].EntityID == p1.Hits[0].EntityID {
		t.Fatalf("page2 must continue, not repeat: %+v", p2.Hits[0])
	}
	// A cursor from a different query is stale — reject, don't mis-slice.
	// 来自不同查询的 cursor 已过期——拒绝而非切错。
	q2 := &searchdomain.Query{Q: "别的查询", Cursor: p1.NextCursor}
	if _, err := svc.Search(ctxWS("ws_a"), q2); !errors.Is(err, searchdomain.ErrCursorInvalid) {
		t.Fatalf("stale cursor must be ErrCursorInvalid, got %v", err)
	}
}

func TestSearch_Validation(t *testing.T) {
	svc := New(newFakeRepo(), nil)
	if _, err := svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "  "}); !errors.Is(err, searchdomain.ErrQueryRequired) {
		t.Fatalf("blank query: %v", err)
	}
	if _, err := svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "x", Types: []searchdomain.EntityType{"nope"}}); !errors.Is(err, searchdomain.ErrTypeInvalid) {
		t.Fatalf("bad type: %v", err)
	}
}

// fakeSource is a scriptable Source (+IncrementalSource).
//
// fakeSource 是可编排的 Source（+IncrementalSource）。
type fakeSource struct {
	t      searchdomain.EntityType
	mu     sync.Mutex
	docs   map[string][]searchdomain.SourceDoc
	atDocs map[string]searchdomain.SourceDoc // key: id#anchor
	stamps map[string]time.Time
}

func newFakeSource(t searchdomain.EntityType) *fakeSource {
	return &fakeSource{t: t, docs: map[string][]searchdomain.SourceDoc{}, atDocs: map[string]searchdomain.SourceDoc{}, stamps: map[string]time.Time{}}
}

func (f *fakeSource) Type() searchdomain.EntityType { return f.t }

func (f *fakeSource) Docs(_ context.Context, id string) ([]searchdomain.SourceDoc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.docs[id], nil
}

func (f *fakeSource) Stamps(context.Context) (map[string]time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]time.Time{}
	for k, v := range f.stamps {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSource) DocAt(_ context.Context, id, anchor string) (*searchdomain.SourceDoc, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.atDocs[id+"#"+anchor]
	if !ok {
		return nil, false, nil
	}
	return &d, true, nil
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

func TestIndexer_ChangedRoutesFullAndIncremental(t *testing.T) {
	repo := newFakeRepo()
	src := newFakeSource(searchdomain.TypeConversation)
	src.docs["cv_1"] = []searchdomain.SourceDoc{{ChunkNo: 0, Title: "会话", Body: "全文"}}
	src.atDocs["cv_1#msg_9"] = searchdomain.SourceDoc{ChunkNo: 9, Anchor: "msg_9", Title: "会话", Body: "新消息"}

	svc := New(repo, nil)
	svc.RegisterSource(src)
	svc.Start(nil)
	defer svc.Close()

	// Full re-projection.
	// 整体重投影。
	svc.Notifier().Changed(ctxWS("ws_a"), searchdomain.TypeConversation, "cv_1", "")
	waitFor(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.replaced["conversation/cv_1"]) == 1
	})

	// Incremental anchor path must NOT re-project the whole entity.
	// anchor 增量路径必须不整体重投影。
	svc.Notifier().Changed(ctxWS("ws_a"), searchdomain.TypeConversation, "cv_1", "msg_9")
	waitFor(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		_, ok := repo.upserted["conversation/cv_1#msg_9"]
		return ok
	})

	// Vanished entity → empty docs → delete (ReplaceDocs with nil).
	// 实体消失 → docs 空 → 删除（ReplaceDocs nil）。
	svc.Notifier().Changed(ctxWS("ws_a"), searchdomain.TypeConversation, "cv_gone", "")
	waitFor(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		docs, ok := repo.replaced["conversation/cv_gone"]
		return ok && len(docs) == 0
	})
}

func TestIndexer_ReconcileDiffsAndOrphans(t *testing.T) {
	repo := newFakeRepo()
	src := newFakeSource(searchdomain.TypeFunction)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	src.stamps["fn_new"] = now                      // not indexed → project. 未入索 → 投影。
	src.stamps["fn_stale"] = now.Add(2 * time.Hour) // indexed but older → re-project. 已入索但旧 → 重投影。
	src.stamps["fn_fresh"] = now                    // indexed and current → untouched. 已入索且新 → 不动。
	src.docs["fn_new"] = []searchdomain.SourceDoc{{Title: "n"}}
	src.docs["fn_stale"] = []searchdomain.SourceDoc{{Title: "s"}}
	src.docs["fn_fresh"] = []searchdomain.SourceDoc{{Title: "f"}}
	repo.stamps[searchdomain.TypeFunction] = map[string]time.Time{
		"fn_stale":  now,
		"fn_fresh":  now,
		"fn_orphan": now, // indexed but no longer live → delete. 已入索但已无 → 删。
	}

	svc := New(repo, nil)
	svc.RegisterSource(src)
	svc.Start([]string{"ws_a"})
	defer svc.Close()

	waitFor(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		_, newDone := repo.replaced["function/fn_new"]
		_, staleDone := repo.replaced["function/fn_stale"]
		orphan, orphanDone := repo.replaced["function/fn_orphan"]
		return newDone && staleDone && orphanDone && len(orphan) == 0
	})
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if _, touched := repo.replaced["function/fn_fresh"]; touched {
		t.Fatal("fresh entity must not be re-indexed")
	}
	if repo.meta["fts_schema_version"] != schemaVersion {
		t.Fatalf("schema version not stamped: %q", repo.meta["fts_schema_version"])
	}
}

func TestReindex_ConflictWhileRunning(t *testing.T) {
	repo := newFakeRepo()
	repo.purgeGo = make(chan struct{})
	svc := New(repo, nil)
	svc.Start(nil)
	defer svc.Close()

	if err := svc.Reindex(ctxWS("ws_a")); err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	if err := svc.Reindex(ctxWS("ws_a")); !errors.Is(err, searchdomain.ErrReindexRunning) {
		t.Fatalf("second reindex must conflict, got %v", err)
	}
	close(repo.purgeGo)
	waitFor(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.purged) == 1 && repo.purged[0] == "ws_a"
	})
	// After completion a new reindex is accepted again.
	// 完成后新的 reindex 再次可用。
	waitFor(t, func() bool { return svc.Reindex(ctxWS("ws_a")) == nil })
}

func TestSearchBlocks_PaletteSemantics(t *testing.T) {
	repo := newFakeRepo()
	// Two methods of one handler + an mcp tool + an mcp server card (no ref).
	// 同一 handler 两个方法 + 一个 mcp 工具 + 一张无 ref 的 mcp server 卡。
	repo.hits = []*searchdomain.DocHit{
		dh(searchdomain.TypeHandler, "hd_x", 1, "sendMail", "邮件.sendMail", 9.0),
		dh(searchdomain.TypeHandler, "hd_x", 2, "sendSMS", "邮件.sendSMS", 8.0),
		dh(searchdomain.TypeMCP, "mcp_s", 1, "send_email", "srv/send_email", 7.0),
		dh(searchdomain.TypeMCP, "mcp_s", 0, "", "srv", 6.0),
	}
	svc := New(repo, nil)

	hits, err := svc.SearchBlocks(ctxWS("ws_a"), "发送", nil, 0)
	if err != nil {
		t.Fatalf("blocks: %v", err)
	}
	// (entity, anchor) folding: both methods are separate hits — palette unit is
	// the callable, and the ref wires straight into a node.
	// (entity, anchor) 折叠：两个方法各自成命中——面板单元是可调用体，ref 直接接线。
	if len(hits) != 3 {
		t.Fatalf("want 3 wireable hits (server card dropped), got %d: %+v", len(hits), hits)
	}
	if hits[0].Ref != "hd_x.sendMail" || hits[1].Ref != "hd_x.sendSMS" || hits[2].Ref != "mcp:mcp_s/send_email" {
		t.Fatalf("refs wrong: %+v", hits)
	}

	if _, err := svc.SearchBlocks(ctxWS("ws_a"), " ", nil, 0); !errors.Is(err, searchdomain.ErrQueryRequired) {
		t.Fatalf("empty query: %v", err)
	}
	if _, err := svc.SearchBlocks(ctxWS("ws_a"), "x", []searchdomain.EntityType{searchdomain.TypeConversation}, 0); !errors.Is(err, searchdomain.ErrTypeInvalid) {
		t.Fatalf("non-block kind must be rejected: %v", err)
	}
}

// --- semantic-layer fakes ----------------------------------------------------

func (f *fakeRepo) UpsertEmbedding(_ context.Context, docID, model string, vec []float32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.embedded == nil {
		f.embedded = map[string][]float32{}
	}
	f.embedded[model+"/"+docID] = vec
	return nil
}

func (f *fakeRepo) MissingEmbeddings(_ context.Context, model string, limit int) ([]searchdomain.EmbedDoc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []searchdomain.EmbedDoc
	for _, d := range f.embedQueue {
		if _, ok := f.embedded[model+"/"+d.DocID]; ok {
			continue
		}
		out = append(out, d)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepo) WorkspaceVectors(_ context.Context, model string) (map[string][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]float32{}
	for k, v := range f.embedded {
		if strings.HasPrefix(k, model+"/") {
			out[strings.TrimPrefix(k, model+"/")] = v
		}
	}
	return out, nil
}

func (f *fakeRepo) DocsByIDs(_ context.Context, ids []string) ([]*searchdomain.DocHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*searchdomain.DocHit
	for _, id := range ids {
		if dh, ok := f.docsByID[id]; ok {
			out = append(out, dh)
		}
	}
	return out, nil
}

// fakeProvider returns scripted vectors keyed by exact text.
//
// fakeProvider 按精确文本返回预置向量。
type fakeProvider struct {
	model string
	vecs  map[string][]float32
	fail  bool
}

func (f *fakeProvider) Model() string { return f.model }

func (f *fakeProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.fail {
		return nil, errors.New("provider down")
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, ok := f.vecs[t]
		if !ok {
			v = []float32{0, 0, 1}
		}
		out[i] = v
	}
	return out, nil
}

func TestHybrid_SemanticOnlyHitSurfaces(t *testing.T) {
	repo := newFakeRepo()
	// Lexical finds doc A; doc B matches only semantically (vector near query).
	// 词法命中 A；B 只有语义相近（向量贴近查询）。
	repo.hits = []*searchdomain.DocHit{dh(searchdomain.TypeDocument, "doc_a", 0, "", "天气预报文档", 5.0)}
	repo.hits[0].DocID = "sd_a"
	repo.docsByID = map[string]*searchdomain.DocHit{
		"sd_b": {DocID: "sd_b", EntityType: searchdomain.TypeDocument, EntityID: "doc_b", Title: "气象播报", Snippet: "正文头部", UpdatedAt: time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)},
	}
	repo.embedded = map[string][]float32{
		"m1/sd_b": {1, 0, 0},
		"m1/sd_a": {0, 1, 0},
	}
	svc := New(repo, nil)
	svc.SetEmbeddingProviders(&fakeProvider{model: "m1", vecs: map[string][]float32{"天气": {1, 0, 0}}}, nil)

	page, err := svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "天气", IncludeArchived: true})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	ids := map[string]bool{}
	for _, h := range page.Hits {
		ids[h.EntityID] = true
	}
	if !ids["doc_a"] || !ids["doc_b"] {
		t.Fatalf("hybrid must surface both lexical and semantic-only hits: %+v", page.Hits)
	}
}

func TestHybrid_DegradesWhenProviderFails(t *testing.T) {
	repo := newFakeRepo()
	repo.hits = []*searchdomain.DocHit{dh(searchdomain.TypeDocument, "doc_a", 0, "", "天气预报文档", 5.0)}
	repo.hits[0].DocID = "sd_a"
	repo.embedded = map[string][]float32{"m1/sd_a": {0, 1, 0}}
	svc := New(repo, nil)
	svc.SetEmbeddingProviders(&fakeProvider{model: "m1", fail: true}, nil)

	page, err := svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "天气", IncludeArchived: true})
	if err != nil || len(page.Hits) != 1 {
		t.Fatalf("provider failure must degrade to lexical, got %v %+v", err, page)
	}

	// embedder=off skips fusion entirely.
	// embedder=off 完全跳过融合。
	repo.meta["embedder"] = searchdomain.EmbedderOff
	if page, err = svc.Search(ctxWS("ws_a"), &searchdomain.Query{Q: "天气", IncludeArchived: true}); err != nil || len(page.Hits) != 1 {
		t.Fatalf("off mode must stay lexical: %v %+v", err, page)
	}
}

func TestEmbedWorker_BackfillsAndInvalidates(t *testing.T) {
	repo := newFakeRepo()
	repo.embedQueue = []searchdomain.EmbedDoc{
		{DocID: "sd_1", Title: "标题", Body: "正文"},
		{DocID: "sd_2", Title: "另一", Body: "再来"},
	}
	svc := New(repo, nil)
	svc.SetEmbeddingProviders(&fakeProvider{model: "m1", vecs: map[string][]float32{}}, nil)
	svc.Start(nil)
	defer svc.Close()

	svc.kickEmbed("ws_a")
	waitFor(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.embedded) == 2
	})
}

func TestSettings_SwitchAndValidate(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo, nil)
	var gotBase, gotModel string
	svc.SetEmbeddingProviders(&fakeProvider{model: "m1"}, func(baseURL, model string) searchdomain.EmbeddingProvider {
		gotBase, gotModel = baseURL, model
		return &fakeProvider{model: "ollama:" + model}
	})

	view, err := svc.Settings(ctxWS("ws_a"))
	if err != nil || view.Embedder != searchdomain.EmbedderBuiltin {
		t.Fatalf("default must be builtin: %v %+v", err, view)
	}
	// Defaults surface on the view even before switching.
	// 切换前默认值就应出现在 view 上。
	if view.OllamaBaseURL != searchdomain.DefaultOllamaBaseURL || view.OllamaModel != searchdomain.DefaultOllamaModel {
		t.Fatalf("ollama defaults missing on view: %+v", view)
	}
	if _, err := svc.SetEmbedder(ctxWS("ws_a"), "nope"); !errors.Is(err, searchdomain.ErrEmbedderInvalid) {
		t.Fatalf("invalid embedder: %v", err)
	}
	view, err = svc.SetEmbedder(ctxWS("ws_a"), searchdomain.EmbedderOff)
	if err != nil || view.Embedder != searchdomain.EmbedderOff || view.Engine.Status != "off" {
		t.Fatalf("off switch: %v %+v", err, view)
	}
	view, err = svc.SetEmbedder(ctxWS("ws_a"), searchdomain.EmbedderOllama)
	if err != nil || view.Engine.Model != "ollama:"+searchdomain.DefaultOllamaModel {
		t.Fatalf("ollama switch: %v %+v", err, view)
	}
	if gotBase != searchdomain.DefaultOllamaBaseURL || gotModel != searchdomain.DefaultOllamaModel {
		t.Fatalf("factory got wrong defaults: %q %q", gotBase, gotModel)
	}
}

// TestSettings_OllamaParamsPatch: patching the connection params rebuilds the adapter
// with the new values and echoes them on the view; "" resets to defaults.
//
// TestSettings_OllamaParamsPatch：修补连接参数即用新值重建适配器并回显；"" 重置默认。
func TestSettings_OllamaParamsPatch(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo, nil)
	var gotBase, gotModel string
	svc.SetEmbeddingProviders(nil, func(baseURL, model string) searchdomain.EmbeddingProvider {
		gotBase, gotModel = baseURL, model
		return &fakeProvider{model: "ollama:" + model}
	})

	emb, base, model := searchdomain.EmbedderOllama, "http://10.0.0.5:11434", "nomic-embed-text"
	view, err := svc.UpdateSettings(ctxWS("ws_a"), UpdateSettingsInput{Embedder: &emb, OllamaBaseURL: &base, OllamaModel: &model})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if view.OllamaBaseURL != base || view.OllamaModel != model {
		t.Fatalf("view did not echo params: %+v", view)
	}
	if gotBase != base || gotModel != model {
		t.Fatalf("factory got %q %q, want %q %q", gotBase, gotModel, base, model)
	}
	if view.Engine.Model != "ollama:"+model {
		t.Fatalf("engine model wrong: %+v", view.Engine)
	}

	// "" resets to default; only-URL patch keeps the model.
	// "" 重置默认；只补 URL 不动 model。
	empty := ""
	view, err = svc.UpdateSettings(ctxWS("ws_a"), UpdateSettingsInput{OllamaBaseURL: &empty})
	if err != nil || view.OllamaBaseURL != searchdomain.DefaultOllamaBaseURL || view.OllamaModel != model {
		t.Fatalf("URL reset wrong: %v %+v", err, view)
	}
}

func (f *fakeRepo) BodiesByIDs(_ context.Context, ids []string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if b, ok := f.bodies[id]; ok {
			out[id] = b
		}
	}
	return out, nil
}

func (f *fakeRepo) BlockRows(context.Context) ([]*searchdomain.DocHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blockRows, nil
}

// fakeSifter records what it saw and returns scripted picks.
//
// fakeSifter 记录所见并返回预置选择。
type fakeSifter struct {
	mu        sync.Mutex
	lastItems []string
	picks     []int
	fail      bool
	calls     int
}

func (f *fakeSifter) Sift(_ context.Context, _ string, items []string, _ int) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastItems = items
	if f.fail {
		return nil, errors.New("utility down")
	}
	return f.picks, nil
}

func blockRow(t searchdomain.EntityType, id, anchor, title, snip string) *searchdomain.DocHit {
	return &searchdomain.DocHit{
		DocID: "sd_" + id, EntityType: t, EntityID: id, Anchor: anchor, Title: title, Snippet: snip,
		UpdatedAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
	}
}

func TestSearchBlocks_TierOne_DirectSiftOverWholeCatalog(t *testing.T) {
	repo := newFakeRepo()
	repo.blockRows = []*searchdomain.DocHit{
		blockRow(searchdomain.TypeFunction, "fn_w", "", "天气查询", "查城市天气"),
		blockRow(searchdomain.TypeHandler, "hd_m", "sendMail", "邮件.sendMail", "发送邮件"),
		blockRow(searchdomain.TypeMCP, "mcp_s", "", "srv", "server card — no ref, must be filtered"),
	}
	sifter := &fakeSifter{picks: []int{1, 0}}
	svc := New(repo, nil)
	svc.SetSifter(sifter)

	hits, err := svc.SearchBlocks(ctxWS("ws_a"), "发邮件", nil, 0)
	if err != nil {
		t.Fatalf("blocks: %v", err)
	}
	// Whole wireable catalog (2 rows — the card dropped) went to the sifter; no
	// index retrieval involved.
	// 全部可接线目录（2 行——卡片被滤）直喂 sifter；不经索引检索。
	if len(sifter.lastItems) != 2 {
		t.Fatalf("sifter must see the whole wireable catalog, saw %d", len(sifter.lastItems))
	}
	if len(hits) != 2 || hits[0].Ref != "hd_m.sendMail" || hits[1].Ref != "fn_w" {
		t.Fatalf("sift order must win: %+v", hits)
	}
}

func TestSearchBlocks_TierTwo_RetrieveThenSift(t *testing.T) {
	repo := newFakeRepo()
	// Catalog far over the token budget → tier 1 skipped.
	// 目录远超 token 预算 → 跳过第一档。
	long := strings.Repeat("非常长的描述文本 ", 200)
	for i := 0; i < 40; i++ {
		repo.blockRows = append(repo.blockRows, blockRow(searchdomain.TypeFunction, fmt.Sprintf("fn_%02d", i), "", "函数", long))
	}
	repo.hits = []*searchdomain.DocHit{
		dh(searchdomain.TypeFunction, "fn_07", 0, "", "目标函数", 9.0),
		dh(searchdomain.TypeFunction, "fn_08", 0, "", "次选函数", 5.0),
	}
	sifter := &fakeSifter{picks: []int{1}}
	svc := New(repo, nil)
	svc.SetSifter(sifter)

	hits, err := svc.SearchBlocks(ctxWS("ws_a"), "目标", nil, 0)
	if err != nil {
		t.Fatalf("blocks: %v", err)
	}
	if len(sifter.lastItems) != 2 {
		t.Fatalf("tier 2 must sift the retrieved candidates, saw %d", len(sifter.lastItems))
	}
	if len(hits) != 1 || hits[0].EntityID != "fn_08" {
		t.Fatalf("sift pick must win: %+v", hits)
	}
}

func TestSearchBlocks_TierThree_SiftFailureFallsBack(t *testing.T) {
	repo := newFakeRepo()
	repo.blockRows = []*searchdomain.DocHit{
		blockRow(searchdomain.TypeFunction, "fn_w", "", "天气查询", "查城市天气"),
	}
	repo.hits = []*searchdomain.DocHit{dh(searchdomain.TypeFunction, "fn_w", 0, "", "天气查询", 3.0)}
	svc := New(repo, nil)
	svc.SetSifter(&fakeSifter{fail: true})

	hits, err := svc.SearchBlocks(ctxWS("ws_a"), "天气", nil, 0)
	if err != nil || len(hits) != 1 || hits[0].Ref != "fn_w" {
		t.Fatalf("sift failure must fall back to index ranking: %v %+v", err, hits)
	}
}

func TestRetrieve_ChunksWithFullBodies(t *testing.T) {
	repo := newFakeRepo()
	repo.hits = []*searchdomain.DocHit{
		dh(searchdomain.TypeDocument, "doc_1", 1, "概述", "设计稿", 9.0),
		dh(searchdomain.TypeDocument, "doc_2", 0, "", "另一篇", 4.0),
	}
	repo.hits[0].DocID, repo.hits[1].DocID = "sd_1", "sd_2"
	repo.bodies = map[string]string{
		"sd_1": "完整正文第一篇，比 snippet 长得多。",
		"sd_2": "第二篇完整正文。",
	}
	svc := New(repo, nil)
	chunks, err := svc.Retrieve(ctxWS("ws_a"), "设计", searchdomain.RetrieveOpts{TopK: 5})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(chunks) != 2 || chunks[0].Body != "完整正文第一篇，比 snippet 长得多。" || chunks[0].Anchor != "概述" {
		t.Fatalf("chunks wrong: %+v", chunks)
	}
	// MaxChars truncates the budgeted tail.
	// MaxChars 截断超预算尾部。
	chunks, err = svc.Retrieve(ctxWS("ws_a"), "设计", searchdomain.RetrieveOpts{TopK: 5, MaxChars: 5})
	if err != nil || len(chunks) != 1 || len([]rune(chunks[0].Body)) != 5 {
		t.Fatalf("budget truncation wrong: %v %+v", err, chunks)
	}
}
