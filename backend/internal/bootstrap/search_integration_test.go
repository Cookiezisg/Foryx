package bootstrap

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	workspaceapp "github.com/sunweilin/forgify/backend/internal/app/workspace"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// TestBuild_SearchEndToEnd proves the whole search chain through the real
// composition root: entity write → publish hook → Notifier → index worker →
// FTS projection → omni-search hit; then delete → zero residue. This is the
// wiring test — package-level tests cover the engine itself.
//
// TestBuild_SearchEndToEnd 经真实装配根证明搜索全链：实体写 → publish 钩子 →
// Notifier → 索引 worker → FTS 投影 → 综搜命中；再删 → 零残留。这是接线测试——
// 引擎本体由包内测试覆盖。
func TestBuild_SearchEndToEnd(t *testing.T) {
	app, err := Build(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer app.svc.search.Close()

	ws, err := app.svc.workspace.Create(context.Background(), workspaceapp.CreateInput{Name: "搜索测试"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	ctx := reqctxpkg.Detached(ws.ID)
	app.svc.search.Start([]string{ws.ID})

	doc, err := app.svc.document.Create(ctx, documentapp.CreateInput{
		Name:    "持久化设计",
		Content: "# 引擎\n\n工作流引擎采用节点结果记忆化实现崩溃恢复。",
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	search := func(q string) []*searchdomain.Hit {
		page, err := app.svc.search.Search(ctx, &searchdomain.Query{Q: q, IncludeArchived: true})
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return page.Hits
	}
	wait := func(desc string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timeout: %s", desc)
	}

	// Content hit (trigram CJK) with the document's heading anchor.
	// 正文命中（trigram 中文），附文档标题锚。
	wait("content indexed", func() bool {
		hits := search("记忆化")
		return len(hits) == 1 && hits[0].EntityID == doc.ID && hits[0].EntityType == searchdomain.TypeDocument
	})
	// Cross-workspace isolation through the real stack.
	// 真实栈下的跨 workspace 隔离。
	ws2, err := app.svc.workspace.Create(context.Background(), workspaceapp.CreateInput{Name: "另一个"})
	if err != nil {
		t.Fatalf("create ws2: %v", err)
	}
	if page, err := app.svc.search.Search(reqctxpkg.Detached(ws2.ID), &searchdomain.Query{Q: "记忆化", IncludeArchived: true}); err != nil || len(page.Hits) != 0 {
		t.Fatalf("isolation broken: %v %+v", err, page)
	}

	// Delete → the publish hook drives the index to zero residue.
	// 删除 → publish 钩子驱动索引零残留。
	if _, err := app.svc.document.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	wait("index cleaned after delete", func() bool { return len(search("记忆化")) == 0 })
}

// TestBuild_SearchHTTPSurface proves the HTTP wire: omni-search returns the N1
// envelope with hits, and reindex answers 202 (N2/N5).
//
// TestBuild_SearchHTTPSurface 证明 HTTP 线缆：综搜返回 N1 envelope + 命中，
// reindex 回 202（N2/N5）。
func TestBuild_SearchHTTPSurface(t *testing.T) {
	app, err := Build(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer app.svc.search.Close()
	srv := httptest.NewServer(app.Handler)
	defer srv.Close()

	ws, err := app.svc.workspace.Create(context.Background(), workspaceapp.CreateInput{Name: "http 测试"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	app.svc.search.Start([]string{ws.ID})
	ctx := reqctxpkg.Detached(ws.ID)
	if _, err := app.svc.document.Create(ctx, documentapp.CreateInput{Name: "天气接入指南", Content: "对接天气服务的步骤"}); err != nil {
		t.Fatalf("create document: %v", err)
	}

	get := func(path string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set(middlewarehttpapi.HeaderWorkspaceID, ws.ID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		code, body := get("/api/v1/search?q=" + url.QueryEscape("天气"))
		if code == http.StatusOK && strings.Contains(body, `"entityType":"document"`) && strings.Contains(body, `"total":1`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("omni-search never hit: %d %s", code, body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Empty q → 400 SEARCH_QUERY_REQUIRED through the envelope.
	// 空 q → 经 envelope 的 400 SEARCH_QUERY_REQUIRED。
	if code, body := get("/api/v1/search?q="); code != http.StatusBadRequest || !strings.Contains(body, "SEARCH_QUERY_REQUIRED") {
		t.Fatalf("empty q: %d %s", code, body)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/search:reindex", nil)
	req.Header.Set(middlewarehttpapi.HeaderWorkspaceID, ws.ID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST reindex: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("reindex status = %d, want 202", resp.StatusCode)
	}
}

// TestBuild_SearchSettingsSurface proves the settings wire: default builtin,
// PATCH switches, invalid values reject with the domain code.
//
// TestBuild_SearchSettingsSurface 证明 settings 线缆：默认 builtin、PATCH 可切、
// 非法值按域码拒绝。
func TestBuild_SearchSettingsSurface(t *testing.T) {
	app, err := Build(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer app.svc.search.Close()
	srv := httptest.NewServer(app.Handler)
	defer srv.Close()

	ws, err := app.svc.workspace.Create(context.Background(), workspaceapp.CreateInput{Name: "settings 测试"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	do := func(method, path, body string) (int, string) {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		req.Header.Set(middlewarehttpapi.HeaderWorkspaceID, ws.ID)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code, body := do(http.MethodGet, "/api/v1/search/settings", ""); code != 200 || !strings.Contains(body, `"embedder":"builtin"`) {
		t.Fatalf("default settings: %d %s", code, body)
	}
	if code, body := do(http.MethodPatch, "/api/v1/search/settings", `{"embedder":"off"}`); code != 200 || !strings.Contains(body, `"status":"off"`) {
		t.Fatalf("switch off: %d %s", code, body)
	}
	if code, body := do(http.MethodPatch, "/api/v1/search/settings", `{"embedder":"cloud"}`); code != 400 || !strings.Contains(body, "SEARCH_EMBEDDER_INVALID") {
		t.Fatalf("invalid embedder: %d %s", code, body)
	}
	// Ollama connection params patch + echo; "" resets to the domain default.
	// Ollama 连接参数修补 + 回显；"" 重置回域默认。
	if code, body := do(http.MethodPatch, "/api/v1/search/settings", `{"ollamaBaseUrl":"http://10.0.0.9:11434","ollamaModel":"nomic-embed-text"}`); code != 200 ||
		!strings.Contains(body, `"ollamaBaseUrl":"http://10.0.0.9:11434"`) || !strings.Contains(body, `"ollamaModel":"nomic-embed-text"`) {
		t.Fatalf("ollama params patch: %d %s", code, body)
	}
	if code, body := do(http.MethodPatch, "/api/v1/search/settings", `{"ollamaModel":""}`); code != 200 ||
		!strings.Contains(body, `"ollamaModel":"embeddinggemma"`) || !strings.Contains(body, `"ollamaBaseUrl":"http://10.0.0.9:11434"`) {
		t.Fatalf("ollama model reset: %d %s", code, body)
	}
}
