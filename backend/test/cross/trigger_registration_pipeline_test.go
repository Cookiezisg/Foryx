//go:build pipeline

package cross

import (
	"testing"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// mustCreateCronWorkflow builds a workflow whose trigger node is a cron listener (valid expression).
func mustCreateCronWorkflow(t *testing.T, h *th.Harness, name string) string {
	t.Helper()
	ctx := th.CtxAs("test-user")
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"` + name + `","description":"cron e2e"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"kind":"cron","spec":{"expression":"0 0 * * *"}}}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Create cron workflow: %v", err)
	}
	return wf.ID
}

func cronTriggerActive(states []triggerdomain.State) bool {
	for _, s := range states {
		if s.Kind == triggerdomain.KindCron {
			return true
		}
	}
	return false
}

// covers: cross:workflow_trigger:activate_registers_listener
// This is the regression test for the trigger-disconnect bug: before the workflow→trigger wire,
// Create/`:activate` only flipped `enabled` and the cron listener never registered.
func TestTriggerRegistration_CreateEnabled_RegistersCronListener(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateCronWorkflow(t, h, "cron_reg")

	// Create auto-enables + syncs → the cron listener must now be registered.
	states := h.Trigger.State(wfID)
	if len(states) == 0 {
		t.Fatalf("no trigger states after create — listener never registered (the disconnect bug)")
	}
	if !cronTriggerActive(states) {
		t.Errorf("cron listener not registered; states=%+v", states)
	}
}

// covers: cross:workflow_trigger:deactivate_unregisters
func TestTriggerRegistration_DeactivateThenReactivate(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateCronWorkflow(t, h, "cron_toggle")

	if !cronTriggerActive(h.Trigger.State(wfID)) {
		t.Fatalf("precondition: cron should be registered after create")
	}

	// Deactivate → listener must be torn down.
	disabled := false
	if _, err := h.Workflow.UpdateMeta(th.CtxAs("test-user"), workflowapp.UpdateMetaInput{ID: wfID, Enabled: &disabled}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if got := h.Trigger.State(wfID); len(got) != 0 {
		t.Errorf("after deactivate, listeners still registered: %+v", got)
	}

	// Reactivate → listener must come back.
	enabled := true
	if _, err := h.Workflow.UpdateMeta(th.CtxAs("test-user"), workflowapp.UpdateMetaInput{ID: wfID, Enabled: &enabled}); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if !cronTriggerActive(h.Trigger.State(wfID)) {
		t.Errorf("after reactivate, cron listener not re-registered: %+v", h.Trigger.State(wfID))
	}
}
