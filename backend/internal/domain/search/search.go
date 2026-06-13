// Package search is the domain layer of the unified search service: one index,
// four surfaces (human omni-search, vertical filters, the LLM block palette,
// and RAG retrieval). The index is a pure projection of entity content — always
// rebuildable, never authoritative — so deletes are physical and D1 does not
// apply. Encrypted-at-rest fields must never reach this index: a plaintext
// index row would silently undo the at-rest encryption.
//
// Package search 是统一搜索服务的 domain 层：一套索引、四个出口（人的综搜、垂搜、
// LLM 搜积木、RAG 取数）。索引是实体内容的纯投影——永远可重建、绝非事实源——故删除
// 是物理删、D1 不适用。密文落盘字段永不入索：索引明文落盘等于悄悄废掉落盘加密。
package search

import (
	"context"
	"slices"
	"time"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
)

// EntityType enumerates the 12 indexed entity kinds. The string values are the
// physical entity_type column values and the API wire values — one vocabulary
// end to end.
//
// EntityType 枚举 12 类入索实体。字符串值即物理 entity_type 列值与 API 线缆值——
// 端到端一套词汇。
type EntityType string

const (
	TypeConversation EntityType = "conversation"
	TypeFunction     EntityType = "function"
	TypeHandler      EntityType = "handler"
	TypeAgent        EntityType = "agent"
	TypeMCP          EntityType = "mcp"
	TypeSkill        EntityType = "skill"
	TypeDocument     EntityType = "document"
	TypeWorkflow     EntityType = "workflow"
	TypeTrigger      EntityType = "trigger"
	TypeControl      EntityType = "control"
	TypeApproval     EntityType = "approval"
	TypeMemory       EntityType = "memory"
)

// AllEntityTypes is the complete index coverage, in display order.
//
// AllEntityTypes 是索引全覆盖面，按展示序。
var AllEntityTypes = []EntityType{
	TypeConversation, TypeFunction, TypeHandler, TypeAgent, TypeMCP, TypeSkill,
	TypeDocument, TypeWorkflow, TypeTrigger, TypeControl, TypeApproval, TypeMemory,
}

// BlockEntityTypes are the six kinds that wire directly into a workflow graph —
// the search_blocks palette. Conversations/documents/etc. are searchable but
// deliberately NOT exposed through the LLM's cross-entity tool.
//
// BlockEntityTypes 是能直接接进 workflow 图的六类——search_blocks 的积木面板。
// 对话/文档等可搜，但刻意不经 LLM 的跨实体工具暴露。
var BlockEntityTypes = []EntityType{
	TypeFunction, TypeHandler, TypeMCP, TypeAgent, TypeControl, TypeApproval,
}

// IsValidEntityType reports whether t is one of the 12 indexed kinds.
//
// IsValidEntityType 报告 t 是否为 12 类之一。
func IsValidEntityType(t EntityType) bool {
	return slices.Contains(AllEntityTypes, t)
}

// IsBlockEntityType reports whether t belongs to the workflow palette.
//
// IsBlockEntityType 报告 t 是否属于积木面板。
func IsBlockEntityType(t EntityType) bool {
	return slices.Contains(BlockEntityTypes, t)
}

// SourceDoc is one projection row a Source emits for an entity: ChunkNo keys the
// row inside the entity (stable for incremental anchors — e.g. a conversation
// message keeps its block seq), Anchor is the jump target (message id / method
// name / tool name / heading chain / node id).
//
// SourceDoc 是 Source 为实体产出的一行投影：ChunkNo 在实体内为行键（增量 anchor 场景
// 必须稳定——如对话 message 用其块 seq），Anchor 是跳转锚（message id/方法名/工具名/
// 标题链/节点 id）。
type SourceDoc struct {
	ChunkNo   int
	Anchor    string
	Title     string
	Body      string
	Tags      []string
	Archived  bool
	UpdatedAt time.Time
}

// DocHit is one chunk-level lexical hit from the repository, pre-fusion: Score
// is already "higher is better" (the store negates SQLite's ascending bm25).
//
// DocHit 是仓储返回的 chunk 级词法命中、未融合：Score 已是「越大越相关」（store 把
// SQLite 升序 bm25 取负）。
type DocHit struct {
	DocID      string
	EntityType EntityType
	EntityID   string
	ChunkNo    int
	Anchor     string
	Title      string
	Snippet    string
	Tags       []string
	Archived   bool
	UpdatedAt  time.Time
	Score      float64
}

// Hit is one entity-level result after folding (best chunk wins, MatchedChunks
// counts siblings). RefHint is the workflow wiring ref — block kinds only,
// empty for content kinds.
//
// Hit 是折叠后的实体级结果（最高分 chunk 胜出，MatchedChunks 计同实体命中数）。
// RefHint 是 workflow 接线 ref——仅积木六类有，内容类为空。
type Hit struct {
	EntityType    EntityType `json:"entityType"`
	EntityID      string     `json:"entityId"`
	Name          string     `json:"name"`
	Snippet       string     `json:"snippet"`
	Anchor        string     `json:"anchor,omitempty"`
	Tags          []string   `json:"tags,omitempty"`
	Archived      bool       `json:"archived"`
	Score         float64    `json:"score"`
	MatchedChunks int        `json:"matchedChunks"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	RefHint       string     `json:"refHint,omitempty"`
}

// Chunk is one RAG retrieval unit: full body (not a snippet) for context
// injection, plus the anchor for provenance.
//
// Chunk 是一个 RAG 取数单元：完整 body（非 snippet）供上下文注入，附 anchor 溯源。
type Chunk struct {
	EntityType EntityType `json:"entityType"`
	EntityID   string     `json:"entityId"`
	Anchor     string     `json:"anchor,omitempty"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Score      float64    `json:"score"`
}

// RetrieveOpts tunes the RAG surface: TopK chunks, MaxChars total budget.
//
// RetrieveOpts 调 RAG 面：TopK 块数、MaxChars 总预算。
type RetrieveOpts struct {
	Types    []EntityType
	TopK     int
	MaxChars int
}

// Query is the unified search input. Empty Types = omni-search; cursor/limit
// follow N4.
//
// Query 是统一检索入参。Types 空 = 综搜；cursor/limit 遵循 N4。
type Query struct {
	Q               string
	Types           []EntityType
	Tags            []string
	IncludeArchived bool
	UpdatedAfter    *time.Time
	UpdatedBefore   *time.Time
	Cursor          string
	Limit           int
}

// LexicalQuery is the repository-level form: tokens already routed (§6.1), the
// workspace comes from ctx inside the store.
//
// LexicalQuery 是仓储层入参：token 已路由（§6.1），workspace 由 store 从 ctx 取。
type LexicalQuery struct {
	LongTokens      []string // ≥3 chars → FTS5 MATCH. ≥3 字符 → FTS5 MATCH。
	ShortTokens     []string // <3 chars → LIKE predicates. <3 字符 → LIKE 谓词。
	Types           []EntityType
	Tags            []string
	IncludeArchived bool
	UpdatedAfter    *time.Time
	UpdatedBefore   *time.Time
	Limit           int
}

// Repository is the storage contract for the search index. Implemented over raw
// SQL (FTS5 virtual tables are outside pkg/orm) — the one D2 exemption where the
// workspace predicate is hand-written, pinned by isolation tests.
//
// Repository 是搜索索引的存储契约。基于 raw SQL 实现（FTS5 虚表在 pkg/orm 之外）——
// 这是 D2 唯一手写 workspace 谓词的豁免点，由隔离测试钉死。
type Repository interface {
	// ReplaceDocs swaps an entity's whole projection (empty docs = delete all).
	// ReplaceDocs 整体替换实体投影（docs 空 = 全删）。
	ReplaceDocs(ctx context.Context, t EntityType, entityID string, docs []SourceDoc) error
	// UpsertDocAt writes one chunk row keyed by ChunkNo — the incremental path
	// (a completed conversation message) that avoids re-indexing the entity.
	// UpsertDocAt 按 ChunkNo 写单 chunk 行——增量路径（对话单条 message 完成），免整实体重索。
	UpsertDocAt(ctx context.Context, t EntityType, entityID string, doc SourceDoc) error
	DeleteEntity(ctx context.Context, t EntityType, entityID string) error
	// PurgeWorkspace takes an explicit id: it serves the workspace-deletion
	// cascade, which runs after the workspace ctx is gone.
	// PurgeWorkspace 显式传 id：服务于 workspace 删除级联，彼时 ws ctx 已不在。
	PurgeWorkspace(ctx context.Context, workspaceID string) error
	SearchLexical(ctx context.Context, q LexicalQuery) ([]*DocHit, error)
	// EntityStamps returns entity_id → max(updated_at) for one type in the ctx
	// workspace — the reconcile diff base.
	// EntityStamps 返回 ctx workspace 内某类的 entity_id → max(updated_at)——对账差异基。
	EntityStamps(ctx context.Context, t EntityType) (map[string]time.Time, error)
	GetMeta(ctx context.Context, key string) (string, error)
	SetMeta(ctx context.Context, key, value string) error
	// Semantic layer: vectors keyed by doc id + model (mixed models never fuse).
	// 语义层：向量按 doc id + model 记账（混模型绝不融合）。
	UpsertEmbedding(ctx context.Context, docID, model string, vector []float32) error
	// MissingEmbeddings lists ctx-workspace rows lacking a vector for model —
	// the backfill worker's work queue.
	// MissingEmbeddings 列出 ctx workspace 内缺该 model 向量的行——补算 worker 的工作队列。
	MissingEmbeddings(ctx context.Context, model string, limit int) ([]EmbedDoc, error)
	// WorkspaceVectors loads the ctx workspace's vectors for model (query-side
	// cosine scan source, cached by the app layer).
	// WorkspaceVectors 加载 ctx workspace 的该 model 向量（查询侧余弦扫描源，app 层缓存）。
	WorkspaceVectors(ctx context.Context, model string) (map[string][]float32, error)
	// DocsByIDs hydrates chunk rows for vector-only hits (snippet = body head).
	// DocsByIDs 为纯向量命中补行（snippet = 正文头部）。
	DocsByIDs(ctx context.Context, ids []string) ([]*DocHit, error)
	// BodiesByIDs returns full bodies for RAG retrieval (snippets won't do as
	// injected context).
	// BodiesByIDs 返回完整 body 供 RAG 取数（注入上下文不能用 snippet 凑）。
	BodiesByIDs(ctx context.Context, ids []string) (map[string]string, error)
	// BlockRows lists every block-palette row in the ctx workspace (six kinds,
	// snippet = body head) — the precision chain's direct-feed catalog.
	// BlockRows 列出 ctx workspace 全部积木行（六类，snippet=正文头）——精度链的直喂目录。
	BlockRows(ctx context.Context) ([]*DocHit, error)
	// DropAll clears every index row (all workspaces) — the schema-version
	// rebuild path; sources repopulate via reconcile.
	// DropAll 清空全索引（所有 workspace）——schema 版本重建路径；对账重灌。
	DropAll(ctx context.Context) error
}

// Notifier is the write-side hook entity Services call after a successful
// mutation. Implementations must be non-blocking and nil-safe at the call site
// (a nil notifier drops the event; boot reconcile heals).
//
// Notifier 是实体 Service 写成功后调用的钩子。实现必须非阻塞，调用侧 nil 安全
// （nil 即丢弃，boot 对账兜底）。
type Notifier interface {
	// Changed marks an entity dirty. anchor != "" requests the incremental
	// single-chunk path (conversation message id); "" re-projects the entity.
	// Changed 标记实体脏。anchor 非空走单 chunk 增量（对话 message id）；空则整实体重投影。
	Changed(ctx context.Context, t EntityType, entityID, anchor string)
}

// Notify is the nil-safe Notifier call every entity Service uses — a service
// without search wired stays fully functional.
//
// Notify 是实体 Service 统一使用的 nil 安全通知——未接搜索的 service 完全可用。
func Notify(ctx context.Context, n Notifier, t EntityType, entityID, anchor string) {
	if n != nil && entityID != "" {
		n.Changed(ctx, t, entityID, anchor)
	}
}

// EmbeddingProvider is the semantic-layer port (§8): builtin llama-server
// subprocess or Ollama. Absence degrades search to pure lexical silently.
//
// EmbeddingProvider 是语义层端口（§8）：内置 llama-server 子进程或 Ollama。
// 缺席时检索无声降级纯词法。
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Model identifies stored vectors so switching embedders invalidates them
	// instead of mixing; dims derive from the vectors themselves.
	// Model 标识存量向量，使换 embedder 时失效而非混用；维度由向量自身得出。
	Model() string
}

// EmbedDoc is one projection row awaiting a vector.
//
// EmbedDoc 是一行待嵌入的投影。
type EmbedDoc struct {
	DocID string
	Title string
	Body  string
}

// Embedder configuration values (search_meta "embedder"; "" reads as builtin).
//
// embedder 配置值（search_meta "embedder"；"" 视为 builtin）。
const (
	EmbedderBuiltin = "builtin"
	EmbedderOllama  = "ollama"
	EmbedderOff     = "off"
)

// IsValidEmbedder reports whether v is a storable embedder choice.
//
// IsValidEmbedder 报告 v 是否为可落库的 embedder 选择。
func IsValidEmbedder(v string) bool {
	return v == EmbedderBuiltin || v == EmbedderOllama || v == EmbedderOff
}

// EffectiveEmbedder resolves the stored value: unset means builtin (the
// AnythingLLM-style zero-config default).
//
// EffectiveEmbedder 解析存储值：未设即 builtin（AnythingLLM 式零配置默认）。
func EffectiveEmbedder(stored string) string {
	if stored == EmbedderOllama || stored == EmbedderOff {
		return stored
	}
	return EmbedderBuiltin
}

// Ollama connection defaults — the AUTHORITY for "unset" (search_meta
// "ollama_base_url" / "ollama_model" empty). The engine adapter keeps matching
// fallbacks as defense, but this is where the values are defined.
//
// Ollama 连接默认值——「未设」的**权威**（search_meta "ollama_base_url" /
// "ollama_model" 为空时）。engine 适配器留有一致的兜底作防御，但定义在此。
const (
	DefaultOllamaBaseURL = "http://127.0.0.1:11434"
	DefaultOllamaModel   = "embeddinggemma"
)

// EffectiveOllama resolves stored Ollama params to their effective values.
//
// EffectiveOllama 把存储的 Ollama 参数解析为生效值。
func EffectiveOllama(baseURL, model string) (string, string) {
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}
	if model == "" {
		model = DefaultOllamaModel
	}
	return baseURL, model
}

// Domain sentinels (§S20); wire codes registered in error-codes.md.
//
// domain sentinel（§S20）；wire code 登记于 error-codes.md。
var (
	ErrQueryRequired   = errorspkg.New(errorspkg.KindInvalid, "SEARCH_QUERY_REQUIRED", "search query is required")
	ErrTypeInvalid     = errorspkg.New(errorspkg.KindInvalid, "SEARCH_TYPE_INVALID", "unknown search entity type")
	ErrCursorInvalid   = errorspkg.New(errorspkg.KindInvalid, "SEARCH_CURSOR_INVALID", "search cursor is invalid or stale")
	ErrReindexRunning  = errorspkg.New(errorspkg.KindConflict, "SEARCH_REINDEX_RUNNING", "a reindex is already running")
	ErrEmbedderInvalid = errorspkg.New(errorspkg.KindInvalid, "SEARCH_EMBEDDER_INVALID", "embedder must be one of builtin, ollama, off")
)
