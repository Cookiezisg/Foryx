// subagent_test.go — E2E contract tests for the /api/v1/subagent-* +
// /api/v1/conversations/{id}/subagent-runs routes. Real httptest server
// backed by an in-memory SQLite store; subagent Service is constructed
// without LLM dependencies because these endpoints are pure observability
// (no Spawn). Spawn end-to-end → D4-5 pipeline.
//
// subagent_test.go ——/api/v1/subagent-* + /api/v1/conversations/{id}/
// subagent-runs 路由端到端契约测试。真 httptest server 后端用内存 SQLite；
// subagent Service 不依赖 LLM——这些端点纯观测（无 Spawn）。完整 Spawn 走
// D4-5 pipeline。
package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	subagentstore "github.com/sunweilin/forgify/backend/internal/infra/store/subagent"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// newSubagentTestServer constructs an httptest server backed by an
// in-memory SQLite store. The Service is built with nil bridge / picker /
// keys / factory because the four observability endpoints under test
// don't touch any of them — they only read repository rows + the
// in-process registry.
//
// newSubagentTestServer 构造 httptest server，后端内存 SQLite。Service 用
// nil bridge / picker / keys / factory——被测四个观测端点都不触它们，只
// 读 repository 行 + 内存 registry。
func newSubagentTestServer(t *testing.T) (*httptest.Server, subagentdomain.Repository) {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb,
		&subagentdomain.SubagentRun{},
		&subagentdomain.SubagentMessage{},
	); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}
	repo := subagentstore.New(gdb)
	log := zaptest.NewLogger(t)
	svc := subagentapp.New(repo, subagentapp.NewRegistry(), nil, nil, nil, nil, log)

	h := NewSubagentHandler(svc, log)
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(middlewarehttpapi.InjectUserID(mux)), repo
}

// seedRun + seedMessage are repo-direct fixtures.
//
// seedRun + seedMessage 是 repo 直建 fixture。
func seedRun(t *testing.T, repo subagentdomain.Repository, id, convID, typ, status string) *subagentdomain.SubagentRun {
	t.Helper()
	now := time.Now().UTC()
	r := &subagentdomain.SubagentRun{
		ID:                   id,
		ParentConversationID: convID,
		Type:                 typ,
		Prompt:               "find foo",
		Status:               status,
		StartedAt:            now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := repo.CreateRun(t.Context(), r); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return r
}

// insertWithStartedAt creates a SubagentRun with an explicit StartedAt so
// list-ordering tests don't depend on monotonic-clock skew between
// successive seedRun calls.
//
// insertWithStartedAt 显式设 StartedAt 创建 SubagentRun，避免 list 排序
// 测试依赖连续 seedRun 之间的单调时钟偏移。
func insertWithStartedAt(t *testing.T, repo subagentdomain.Repository, id, convID, typ, status string, startedAt time.Time) {
	t.Helper()
	r := &subagentdomain.SubagentRun{
		ID:                   id,
		ParentConversationID: convID,
		Type:                 typ,
		Prompt:               "find foo",
		Status:               status,
		StartedAt:            startedAt,
		CreatedAt:            startedAt,
		UpdatedAt:            startedAt,
	}
	if err := repo.CreateRun(t.Context(), r); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}

func seedMessage(t *testing.T, repo subagentdomain.Repository, id, runID, role string) {
	t.Helper()
	m := &subagentdomain.SubagentMessage{
		ID:            id,
		SubagentRunID: runID,
		Role:          role,
		Blocks: []chatdomain.Block{
			{Type: chatdomain.BlockTypeText, Data: `{"text":"hello"}`},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.AppendMessage(t.Context(), m); err != nil {
		t.Fatalf("seed message: %v", err)
	}
}

// subagentEnvelope mirrors response.Success's wire shape — `{"data": ...}`.
//
// subagentEnvelope 镜像 response.Success 的 wire 形状——`{"data": ...}`。
type subagentEnvelope[T any] struct {
	Data T `json:"data"`
}

func decodeSubagentEnvelope[T any](t *testing.T, body io.ReadCloser) T {
	t.Helper()
	defer body.Close()
	var env subagentEnvelope[T]
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env.Data
}

// ── ListRunsByConversation ───────────────────────────────────────────

func TestSubagent_ListRunsByConversation_FiltersAndOrders(t *testing.T) {
	srv, repo := newSubagentTestServer(t)
	defer srv.Close()

	now := time.Now().UTC()
	// Insert in reverse chronological order so it's not the insertion order
	// but the StartedAt sort that decides — proves the handler honors
	// Service.ListRunsByConversation's "newest-first" contract.
	//
	// 反时序插入——证明排序由 StartedAt 决定，而非插入顺序。
	insertWithStartedAt(t, repo, "sar_old", "cv_x", "Explore", subagentdomain.StatusCompleted, now.Add(-time.Hour))
	insertWithStartedAt(t, repo, "sar_new", "cv_x", "Plan", subagentdomain.StatusRunning, now)
	insertWithStartedAt(t, repo, "sar_other", "cv_y", "Explore", subagentdomain.StatusCompleted, now) // diff conv

	resp, err := http.Get(srv.URL + "/api/v1/conversations/cv_x/subagent-runs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	rows := decodeSubagentEnvelope[[]*subagentdomain.SubagentRun](t, resp.Body)
	if len(rows) != 2 {
		t.Fatalf("got %d rows for cv_x, want 2", len(rows))
	}
	if rows[0].ID != "sar_new" || rows[1].ID != "sar_old" {
		t.Errorf("ordering wrong: %s, %s", rows[0].ID, rows[1].ID)
	}
}

func TestSubagent_ListRunsByConversation_Empty(t *testing.T) {
	srv, _ := newSubagentTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/conversations/cv_nothing/subagent-runs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	rows := decodeSubagentEnvelope[[]*subagentdomain.SubagentRun](t, resp.Body)
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

// ── GetRun ───────────────────────────────────────────────────────────

func TestSubagent_GetRun_Found(t *testing.T) {
	srv, repo := newSubagentTestServer(t)
	defer srv.Close()
	seedRun(t, repo, "sar_1", "cv_1", "Explore", subagentdomain.StatusCompleted)

	resp, err := http.Get(srv.URL + "/api/v1/subagent-runs/sar_1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	row := decodeSubagentEnvelope[*subagentdomain.SubagentRun](t, resp.Body)
	if row.ID != "sar_1" || row.Type != "Explore" {
		t.Errorf("row = %+v", row)
	}
}

func TestSubagent_GetRun_NotFound(t *testing.T) {
	srv, _ := newSubagentTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/subagent-runs/sar_missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ── ListMessages ─────────────────────────────────────────────────────

func TestSubagent_ListMessages_OrderedBySeq(t *testing.T) {
	srv, repo := newSubagentTestServer(t)
	defer srv.Close()
	seedRun(t, repo, "sar_msg", "cv_1", "Explore", subagentdomain.StatusRunning)
	seedMessage(t, repo, "smm_a", "sar_msg", "user")
	seedMessage(t, repo, "smm_b", "sar_msg", "assistant")
	seedMessage(t, repo, "smm_c", "sar_msg", "tool")

	resp, err := http.Get(srv.URL + "/api/v1/subagent-runs/sar_msg/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	rows := decodeSubagentEnvelope[[]*subagentdomain.SubagentMessage](t, resp.Body)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for i, r := range rows {
		if r.Seq != i {
			t.Errorf("rows[%d].Seq = %d, want %d", i, r.Seq, i)
		}
	}
}

func TestSubagent_ListMessages_EmptyRunReturnsEmptyList(t *testing.T) {
	srv, repo := newSubagentTestServer(t)
	defer srv.Close()
	seedRun(t, repo, "sar_empty", "cv_1", "Explore", subagentdomain.StatusCancelled)

	resp, err := http.Get(srv.URL + "/api/v1/subagent-runs/sar_empty/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	rows := decodeSubagentEnvelope[[]*subagentdomain.SubagentMessage](t, resp.Body)
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

// ── ListTypes ────────────────────────────────────────────────────────

func TestSubagent_ListTypes_BuiltInThree(t *testing.T) {
	srv, _ := newSubagentTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/subagent-types")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	types := decodeSubagentEnvelope[[]subagentdomain.SubagentType](t, resp.Body)
	if len(types) != 3 {
		t.Fatalf("got %d types, want 3", len(types))
	}
	want := []string{"Explore", "Plan", "general-purpose"}
	for i, n := range want {
		if types[i].Name != n {
			t.Errorf("types[%d].Name = %q, want %q", i, types[i].Name, n)
		}
	}
}
