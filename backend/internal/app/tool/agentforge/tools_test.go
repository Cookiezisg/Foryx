package agentforge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	agentstore "github.com/sunweilin/forgify/backend/internal/infra/store/agent"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newAgentSvc(t *testing.T) *agentapp.Service {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := dbinfra.Migrate(gdb, agentstore.AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return agentapp.New(agentstore.New(gdb), zap.NewNop())
}

// TestCreateAgent_Execute is the first coverage for the agent forge tools (they shipped with zero
// tests). Proves create_agent actually creates a first-class agent entity end-to-end + that
// search/get find it afterward — the "agent is a first-class citizen you forge directly" contract.
func TestCreateAgent_Execute(t *testing.T) {
	svc := newAgentSvc(t)
	ctx := reqctxpkg.SetUserID(context.Background(), "u_test")

	create := &CreateAgent{svc: svc}
	out, err := create.Execute(ctx, `{"name":"sentiment","description":"classify sentiment","prompt":"Classify the input sentiment.","outputSchema":{"kind":"enum","enums":["positive","negative","neutral"]}}`)
	if err != nil {
		t.Fatalf("create_agent Execute: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("create_agent output not JSON: %v (%s)", err, out)
	}
	id, _ := res["id"].(string)
	if !strings.HasPrefix(id, "ag_") {
		t.Fatalf("expected ag_ id, got %q (out=%s)", id, out)
	}
	if ns, _ := res["next_step"].(string); strings.Contains(ns, "run_agent") {
		t.Errorf("next_step references non-existent tool run_agent: %q", ns)
	}

	// search_agents must now find it.
	search := &SearchAgents{svc: svc}
	sOut, err := search.Execute(ctx, `{"query":"sentiment"}`)
	if err != nil {
		t.Fatalf("search_agents Execute: %v", err)
	}
	if !strings.Contains(sOut, id) {
		t.Errorf("search_agents did not return the created agent %q: %s", id, sOut)
	}

	// get_agent must resolve it.
	get := &GetAgent{svc: svc}
	gOut, err := get.Execute(ctx, `{"id":"`+id+`"}`)
	if err != nil {
		t.Fatalf("get_agent Execute: %v", err)
	}
	if !strings.Contains(gOut, "sentiment") {
		t.Errorf("get_agent did not return the agent details: %s", gOut)
	}
}

// TestCreateAgent_RejectsAgentToolRef guards the "agents cannot call other agents" rule (validateToolRefs).
func TestCreateAgent_RejectsAgentToolRef(t *testing.T) {
	svc := newAgentSvc(t)
	ctx := reqctxpkg.SetUserID(context.Background(), "u_test")
	create := &CreateAgent{svc: svc}
	_, err := create.Execute(ctx, `{"name":"bad","prompt":"x","tools":[{"ref":"ag_other"}]}`)
	if err == nil {
		t.Fatalf("expected create_agent to reject an ag_ tool ref (worker cannot call worker)")
	}
}
