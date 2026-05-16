package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	forgeinfra "github.com/sunweilin/forgify/backend/internal/infra/forge"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

func newForgeServer(t *testing.T) (*httptest.Server, *forgeinfra.Bridge) {
	t.Helper()
	bridge := forgeinfra.NewBridge(nil)
	mux := http.NewServeMux()
	NewForgeHandler(bridge, nil).Register(mux)
	srv := httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
	t.Cleanup(srv.Close)
	return srv, bridge
}

func forgePublishCtx() context.Context {
	return reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
}

func TestForge_StreamDeliversAllEventTypes(t *testing.T) {
	srv, bridge := newForgeServer(t)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/forge", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	time.Sleep(50 * time.Millisecond)
	ctx := forgePublishCtx()
	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindFunction, ID: "fn_x"}

	bridge.Publish(ctx, forgedomain.ForgeStarted{
		Scope: scope, Operation: forgedomain.OperationCreate,
		ConversationID: "cv_a", ToolCallID: "tc_1",
	})
	bridge.Publish(ctx, forgedomain.ForgeEnvAttempt{
		Scope: scope, Attempt: 1, Status: forgedomain.EnvAttemptFailed,
		Error: "No matching distribution",
	})
	bridge.Publish(ctx, forgedomain.ForgeEnvAttempt{
		Scope: scope, Attempt: 2, Status: forgedomain.EnvAttemptOK,
	})
	bridge.Publish(ctx, forgedomain.ForgeCompleted{
		Scope: scope, Status: forgedomain.CompletedOK,
		VersionID: "fnv_y", EnvStatus: "ready", AttemptsUsed: 2,
	})

	got := readSSE(t, resp.Body, 4, 2*time.Second)
	if len(got) != 4 {
		t.Fatalf("want 4 events, got %d", len(got))
	}
	want := []string{"forge_started", "forge_env_attempt", "forge_env_attempt", "forge_completed"}
	for i, w := range want {
		if got[i].event != w {
			t.Errorf("event %d: got %s want %s", i, got[i].event, w)
		}
	}
	if !strings.Contains(got[0].data, `"scope":{`) {
		t.Errorf("forge_started missing nested scope: %q", got[0].data)
	}
	if !strings.Contains(got[0].data, `"kind":"function"`) {
		t.Errorf("forge_started missing scope.kind: %q", got[0].data)
	}
	if !strings.Contains(got[0].data, `"conversationId":"cv_a"`) {
		t.Errorf("forge_started missing conversationId: %q", got[0].data)
	}
}

func TestForge_LastEventIDReplays(t *testing.T) {
	srv, bridge := newForgeServer(t)

	ctx := forgePublishCtx()
	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindHandler, ID: "hd_x"}
	for i := 0; i < 3; i++ {
		bridge.Publish(ctx, forgedomain.ForgeStarted{
			Scope: scope, Operation: forgedomain.OperationCreate,
		})
	}

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/forge", nil)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	got := readSSE(t, resp.Body, 2, 2*time.Second)
	if len(got) != 2 || got[0].id != "2" || got[1].id != "3" {
		t.Errorf("replay: got %d events, ids=%v want 2 events ids=[2,3]",
			len(got), []string{got[0].id, got[1].id})
	}
}
