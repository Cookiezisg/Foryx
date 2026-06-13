package search

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	embedBatch     = 32
	vecTopK        = 100
	rrfK           = 60.0
	embedKickQueue = 64
)

// StatusReporter is the optional provider capability behind the settings
// surface (the builtin engine reports absent/downloading/ready/error).
//
// StatusReporter 是 provider 的可选能力，服务 settings 面（builtin 引擎报
// absent/downloading/ready/error）。
type StatusReporter interface {
	Status() (status, lastErr string)
}

// ProviderCloser is the optional shutdown capability (the builtin engine stops
// its subprocess).
//
// ProviderCloser 是可选关停能力（builtin 引擎停其子进程）。
type ProviderCloser interface {
	Close()
}

// SetEmbeddingProviders wires the builtin adapter + the Ollama factory (bootstrap). The
// ACTIVE provider is chosen per call from search_meta — switching needs no rewiring.
//
// SetEmbeddingProviders 接入 builtin 适配器 + Ollama 工厂（bootstrap）。**生效** provider
// 按 search_meta 逐次解析——切换无需重接线。
func (s *Service) SetEmbeddingProviders(builtin searchdomain.EmbeddingProvider, ollama OllamaFactory) {
	s.builtinProv, s.ollamaFactory = builtin, ollama
}

// OllamaFactory builds an Ollama adapter for the given connection params — injected by
// bootstrap so the app layer never imports the engine. The service rebuilds (and caches)
// the adapter whenever the stored params change, so a PATCH takes effect on the next call.
//
// OllamaFactory 按给定连接参数构造 Ollama 适配器——bootstrap 注入，app 层不 import engine。
// 存储参数变化时 service 重建（并缓存）适配器，PATCH 下一次调用即生效。
type OllamaFactory func(baseURL, model string) searchdomain.EmbeddingProvider

// ollamaProvider returns the Ollama adapter for the CURRENT stored params, rebuilding on
// change. Meta read failures fall back to defaults (same posture as provider()).
//
// ollamaProvider 返回**当前**存储参数对应的 Ollama 适配器，参数变化即重建。meta 读失败回落
// 默认值（与 provider() 同姿态）。
func (s *Service) ollamaProvider(ctx context.Context) searchdomain.EmbeddingProvider {
	if s.ollamaFactory == nil {
		return nil
	}
	baseURL, model := s.ollamaParams(ctx)
	key := baseURL + "\x00" + model
	s.ollamaMu.Lock()
	defer s.ollamaMu.Unlock()
	if s.ollamaKey != key || s.ollamaProv == nil {
		s.ollamaProv, s.ollamaKey = s.ollamaFactory(baseURL, model), key
	}
	return s.ollamaProv
}

// ollamaParams reads the stored connection params and resolves defaults.
//
// ollamaParams 读存储连接参数并解析默认值。
func (s *Service) ollamaParams(ctx context.Context) (string, string) {
	baseURL, err := s.repo.GetMeta(ctx, metaOllamaURLKey)
	if err != nil {
		s.log.Warn("search: ollama url meta read failed", zap.Error(err))
	}
	model, err := s.repo.GetMeta(ctx, metaOllamaModelKey)
	if err != nil {
		s.log.Warn("search: ollama model meta read failed", zap.Error(err))
	}
	return searchdomain.EffectiveOllama(baseURL, model)
}

// provider resolves the active embedder; nil = semantic layer off (pure
// lexical). Meta read failures resolve to builtin — the default must not
// depend on a healthy meta row.
//
// provider 解析生效 embedder；nil = 语义层关（纯词法）。meta 读失败按 builtin——
// 默认值不能依赖 meta 行健康。
func (s *Service) provider(ctx context.Context) searchdomain.EmbeddingProvider {
	stored, err := s.repo.GetMeta(ctx, metaEmbedderKey)
	if err != nil {
		s.log.Warn("search: embedder meta read failed", zap.Error(err))
	}
	switch searchdomain.EffectiveEmbedder(stored) {
	case searchdomain.EmbedderOllama:
		return s.ollamaProvider(ctx)
	case searchdomain.EmbedderOff:
		return nil
	default:
		return s.builtinProv
	}
}

// --- vector cache -----------------------------------------------------------

// vecCache holds per-(workspace, model) vectors in memory: the cosine scan
// must not re-read megabytes of BLOBs per keystroke. Invalidation is
// workspace-coarse — local scale makes reloads cheap.
//
// vecCache 按 (workspace, model) 在内存持有向量：余弦扫描不能每次敲键重读数 MB
// BLOB。失效按 workspace 粗粒度——本地规模下重载廉价。
type vecCache struct {
	mu   sync.RWMutex
	data map[string]map[string][]float32 // key: ws + "\x00" + model
}

func newVecCache() *vecCache {
	return &vecCache{data: map[string]map[string][]float32{}}
}

func (c *vecCache) get(ctx context.Context, repo searchdomain.Repository, ws, model string) (map[string][]float32, error) {
	key := ws + "\x00" + model
	c.mu.RLock()
	v, ok := c.data[key]
	c.mu.RUnlock()
	if ok {
		return v, nil
	}
	loaded, err := repo.WorkspaceVectors(ctx, model)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.data[key] = loaded
	c.mu.Unlock()
	return loaded, nil
}

func (c *vecCache) invalidate(ws string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.data {
		if strings.HasPrefix(k, ws+"\x00") {
			delete(c.data, k)
		}
	}
}

// --- embed backfill worker ----------------------------------------------------

// kickEmbed schedules a workspace for vector backfill (non-blocking; a full
// queue is fine — the next kick re-covers it).
//
// kickEmbed 调度一个 workspace 的向量补算（非阻塞；队满无妨——下次 kick 再覆盖）。
func (s *Service) kickEmbed(ws string) {
	select {
	case s.embedKick <- ws:
	default:
	}
}

// embedWorker drains kicks: per workspace, batch-embed every row missing a
// vector under the ACTIVE model. A provider error stops this round and waits
// for the next kick — the engine may still be downloading; search stays
// lexical meanwhile.
//
// embedWorker 消化 kick：逐 workspace 把缺**生效**模型向量的行批量嵌入。provider
// 出错即停本轮、等下次 kick——引擎可能还在下载；期间检索保持纯词法。
func (s *Service) embedWorker() {
	for {
		select {
		case ws := <-s.embedKick:
			s.backfill(ws)
		case <-s.embedQuit:
			return
		}
	}
}

func (s *Service) backfill(ws string) {
	ctx := reqctxpkg.Detached(ws)
	prov := s.provider(ctx)
	if prov == nil {
		return
	}
	model := prov.Model()
	wrote := false
	for {
		select {
		case <-s.embedQuit:
			return
		default:
		}
		missing, err := s.repo.MissingEmbeddings(ctx, model, embedBatch)
		if err != nil {
			s.log.Warn("search embed: missing scan failed", zap.Error(err))
			break
		}
		if len(missing) == 0 {
			break
		}
		texts := make([]string, len(missing))
		for i, d := range missing {
			texts[i] = embedText(d.Title, d.Body)
		}
		vecs, err := prov.Embed(ctx, texts)
		if err != nil {
			// Expected while the engine downloads/boots — the next kick retries.
			// 引擎下载/启动期间属预期——下次 kick 重试。
			s.log.Info("search embed: provider unavailable, staying lexical", zap.Error(err))
			break
		}
		for i, d := range missing {
			if err := s.repo.UpsertEmbedding(ctx, d.DocID, model, vecs[i]); err != nil {
				s.log.Warn("search embed: upsert failed", zap.String("doc", d.DocID), zap.Error(err))
			} else {
				wrote = true
			}
		}
	}
	if wrote {
		s.vectors.invalidate(ws)
	}
}

func embedText(title, body string) string {
	return searchdomain.CapRunes(strings.TrimSpace(title + "\n" + body))
}

// --- hybrid fusion ------------------------------------------------------------

// fuseSemantic blends the lexical chunk hits with a cosine top-K over the
// workspace's vectors via RRF (k=60). Any failure returns the lexical list
// unchanged — hybrid is an upgrade, never a dependency.
//
// fuseSemantic 用 RRF（k=60）把词法 chunk 命中与 workspace 向量的余弦 top-K 融合。
// 任何失败原样返回词法列表——混合是增强，绝不是依赖。
func (s *Service) fuseSemantic(ctx context.Context, q *searchdomain.Query, lex []*searchdomain.DocHit) []*searchdomain.DocHit {
	prov := s.provider(ctx)
	if prov == nil {
		return lex
	}
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return lex
	}
	vectors, err := s.vectors.get(ctx, s.repo, wsID, prov.Model())
	if err != nil || len(vectors) == 0 {
		return lex
	}
	qvecs, err := prov.Embed(ctx, []string{q.Q})
	if err != nil || len(qvecs) != 1 {
		return lex
	}
	qvec := qvecs[0]

	type vecHit struct {
		id    string
		score float64
	}
	vhits := make([]vecHit, 0, len(vectors))
	for id, v := range vectors {
		if c := cosine(qvec, v); c > 0 {
			vhits = append(vhits, vecHit{id: id, score: c})
		}
	}
	sort.Slice(vhits, func(i, j int) bool { return vhits[i].score > vhits[j].score })
	if len(vhits) > vecTopK {
		vhits = vhits[:vecTopK]
	}

	// RRF over the two ranked lists.
	// 对两个排名表做 RRF。
	fused := map[string]float64{}
	byID := map[string]*searchdomain.DocHit{}
	for i, dh := range lex {
		fused[dh.DocID] += 1.0 / (rrfK + float64(i+1))
		byID[dh.DocID] = dh
	}
	var missingIDs []string
	for i, vh := range vhits {
		fused[vh.id] += 1.0 / (rrfK + float64(i+1))
		if _, ok := byID[vh.id]; !ok {
			missingIDs = append(missingIDs, vh.id)
		}
	}
	if len(missingIDs) > 0 {
		// Vector-only hits need rows — and must re-pass the query's filters,
		// which the lexical SQL applied but the cosine scan did not.
		// 纯向量命中要补行——且必须重过查询过滤器（词法 SQL 过了、余弦扫描没有）。
		rows, err := s.repo.DocsByIDs(ctx, missingIDs)
		if err == nil {
			for _, dh := range rows {
				if matchesFilters(dh, q) {
					byID[dh.DocID] = dh
				}
			}
		}
	}
	out := make([]*searchdomain.DocHit, 0, len(fused))
	for id, score := range fused {
		dh, ok := byID[id]
		if !ok {
			continue // filtered out or hydration failed. 被过滤或补行失败。
		}
		dh.Score = score
		out = append(out, dh)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].DocID < out[j].DocID
	})
	return out
}

// matchesFilters re-applies Query filters to a hydrated row (the cosine path
// bypasses the lexical SQL's WHERE clause).
//
// matchesFilters 对补出的行重过 Query 过滤器（余弦路径绕过了词法 SQL 的 WHERE）。
func matchesFilters(dh *searchdomain.DocHit, q *searchdomain.Query) bool {
	if len(q.Types) > 0 {
		ok := false
		for _, t := range q.Types {
			if dh.EntityType == t {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if !q.IncludeArchived && dh.Archived {
		return false
	}
	if len(q.Tags) > 0 {
		ok := false
		for _, want := range q.Tags {
			for _, got := range dh.Tags {
				if got == want {
					ok = true
					break
				}
			}
		}
		if !ok {
			return false
		}
	}
	if q.UpdatedAfter != nil && dh.UpdatedAt.Before(*q.UpdatedAfter) {
		return false
	}
	if q.UpdatedBefore != nil && dh.UpdatedAt.After(*q.UpdatedBefore) {
		return false
	}
	return true
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// --- settings ----------------------------------------------------------------

// EngineStatus is the settings view of the active embedder.
//
// EngineStatus 是 settings 面看到的生效 embedder 状态。
type EngineStatus struct {
	Status    string `json:"status"` // ready | downloading | absent | error | off
	Model     string `json:"model,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// SettingsView is the GET/PATCH /api/v1/search/settings payload. The Ollama fields show
// the EFFECTIVE connection params (stored or default) regardless of the active embedder,
// so a settings page can render them before the user switches.
//
// SettingsView 是 GET/PATCH /api/v1/search/settings 载荷。Ollama 字段展示**生效**连接参数
// （存储值或默认），与当前 embedder 无关——设置页在用户切换前就能渲染。
type SettingsView struct {
	Embedder      string       `json:"embedder"`
	OllamaBaseURL string       `json:"ollamaBaseUrl"`
	OllamaModel   string       `json:"ollamaModel"`
	Engine        EngineStatus `json:"engine"`
}

// Settings reads the machine-level embedder choice + live engine status.
//
// Settings 读机器级 embedder 选择 + 引擎实时状态。
func (s *Service) Settings(ctx context.Context) (*SettingsView, error) {
	stored, err := s.repo.GetMeta(ctx, metaEmbedderKey)
	if err != nil {
		return nil, err
	}
	eff := searchdomain.EffectiveEmbedder(stored)
	view := &SettingsView{Embedder: eff}
	view.OllamaBaseURL, view.OllamaModel = s.ollamaParams(ctx)
	var prov searchdomain.EmbeddingProvider
	switch eff {
	case searchdomain.EmbedderBuiltin:
		prov = s.builtinProv
	case searchdomain.EmbedderOllama:
		prov = s.ollamaProvider(ctx)
	}
	switch {
	case eff == searchdomain.EmbedderOff:
		view.Engine = EngineStatus{Status: "off"}
	case prov == nil:
		view.Engine = EngineStatus{Status: "absent"}
	default:
		view.Engine = EngineStatus{Status: "ready", Model: prov.Model()}
		if sr, ok := prov.(StatusReporter); ok {
			st, lastErr := sr.Status()
			view.Engine.Status, view.Engine.LastError = st, lastErr
		}
	}
	return view, nil
}

// SetEmbedder stores the machine-level choice and kicks a backfill for the ctx
// workspace — rows embedded under the old model are invalid by the model
// column, never mixed.
//
// SetEmbedder 落机器级选择并 kick ctx workspace 补算——旧模型行按 model 列自然失效，
// 绝不混用。
func (s *Service) SetEmbedder(ctx context.Context, v string) (*SettingsView, error) {
	return s.UpdateSettings(ctx, UpdateSettingsInput{Embedder: &v})
}

// UpdateSettingsInput is the PATCH shape: nil = leave unchanged; empty string on an
// Ollama param = reset to its default.
//
// UpdateSettingsInput 是 PATCH 形状：nil = 不动；Ollama 参数传空串 = 重置回默认。
type UpdateSettingsInput struct {
	Embedder      *string
	OllamaBaseURL *string
	OllamaModel   *string
}

// UpdateSettings patches the machine-level search settings. Changing the Ollama model
// re-keys stored vectors (per-model accounting) so the backfill re-embeds automatically;
// changing only the base URL keeps the vectors (same model, new address).
//
// UpdateSettings 修补机器级搜索设置。改 Ollama model 即换向量记账键（按 model 记账），
// 补算自动重嵌；只改 base URL 向量保留（同模型、新地址）。
func (s *Service) UpdateSettings(ctx context.Context, in UpdateSettingsInput) (*SettingsView, error) {
	if in.Embedder != nil {
		if !searchdomain.IsValidEmbedder(*in.Embedder) {
			return nil, searchdomain.ErrEmbedderInvalid
		}
		if err := s.repo.SetMeta(ctx, metaEmbedderKey, *in.Embedder); err != nil {
			return nil, err
		}
	}
	if in.OllamaBaseURL != nil {
		if err := s.repo.SetMeta(ctx, metaOllamaURLKey, strings.TrimSpace(*in.OllamaBaseURL)); err != nil {
			return nil, err
		}
	}
	if in.OllamaModel != nil {
		if err := s.repo.SetMeta(ctx, metaOllamaModelKey, strings.TrimSpace(*in.OllamaModel)); err != nil {
			return nil, err
		}
	}
	if in.Embedder != nil || in.OllamaBaseURL != nil || in.OllamaModel != nil {
		// Drop the cached adapter (params may have changed) and re-kick the ctx workspace:
		// rows missing the now-active model re-embed in the background.
		// 丢弃缓存适配器（参数可能变了）并重 kick ctx workspace：缺当前生效模型向量的行后台重嵌。
		s.ollamaMu.Lock()
		s.ollamaProv, s.ollamaKey = nil, ""
		s.ollamaMu.Unlock()
		if wsID, err := reqctxpkg.RequireWorkspaceID(ctx); err == nil {
			s.vectors.invalidate(wsID)
			s.kickEmbed(wsID)
		}
	}
	return s.Settings(ctx)
}
