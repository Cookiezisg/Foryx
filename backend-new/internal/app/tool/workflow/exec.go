package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
)

// exec.go is the workflow EXECUTION-LIFECYCLE tool group (D1) — the five verbs that drive a
// workflow's runtime, distinct from the forge/query tools that edit its graph:
//   trigger_workflow    run it once now (synthetic payload)
//   stage_workflow      arm it for one run on the next real trigger, then auto-disarm
//   activate_workflow   bring it online (start listening to its trigger continuously)
//   deactivate_workflow take it offline gracefully (stop listening; in-flight runs finish)
//   kill_workflow       hard-stop it (stop listening + cancel every in-flight run)
//
// exec.go 是 workflow 执行生命周期工具组（D1）——驱动 workflow 运行时的五个动词，区别于编辑其图的
// forge/query 工具。

// --- trigger_workflow ------------------------------------------------------

type TriggerWorkflow struct{ svc *workflowapp.Service }

func (t *TriggerWorkflow) Name() string { return "trigger_workflow" }

func (t *TriggerWorkflow) Description() string {
	return "Run a workflow once, right now, with a payload you supply as if a trigger had fired it. Use this to execute a workflow on demand (a manual \"run now\"). The payload should match the shape the workflow's entry trigger emits; pass {} if it reads nothing. Returns the new flowrun id. Does not change whether the workflow is listening for real triggers — use activate_workflow for that."
}

func (t *TriggerWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {
			"workflowId": {"type": "string"},
			"payload": {"type": "object", "description": "Data fed to the entry trigger node as its result (the workflow reads <triggerNode>.field). Optional; defaults to {}."}
		}
	}`)
}

func (t *TriggerWorkflow) ValidateInput(args json.RawMessage) error {
	var a struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("trigger_workflow: bad args: %w", err)
	}
	if a.WorkflowID == "" {
		return fmt.Errorf("trigger_workflow: workflowId is required")
	}
	return nil
}

func (t *TriggerWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		WorkflowID string         `json:"workflowId"`
		Payload    map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("trigger_workflow: bad args: %w", err)
	}
	runID, err := t.svc.Trigger(ctx, args.WorkflowID, args.Payload)
	if err != nil {
		return "", fmt.Errorf("trigger_workflow: %w", err)
	}
	return toJSON(map[string]any{"flowrunId": runID, "workflowId": args.WorkflowID}), nil
}

// --- stage_workflow --------------------------------------------------------

type StageWorkflow struct{ svc *workflowapp.Service }

func (t *StageWorkflow) Name() string { return "stage_workflow" }

func (t *StageWorkflow) Description() string {
	return "Arm a workflow to run exactly once on its NEXT real trigger fire, then automatically disarm — a trial run with a genuine event. Use this to test a workflow against a real cron tick / webhook / file change without committing it to listen forever. Only works on workflows whose entry is a real trigger source; fails if the workflow is already active (deactivate it first)."
}

func (t *StageWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {"workflowId": {"type": "string"}}
	}`)
}

func (t *StageWorkflow) ValidateInput(args json.RawMessage) error {
	return validateWorkflowID(args, "stage_workflow")
}

func (t *StageWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	id, err := workflowIDArg(argsJSON, "stage_workflow")
	if err != nil {
		return "", err
	}
	if err := t.svc.Stage(ctx, id); err != nil {
		return "", fmt.Errorf("stage_workflow: %w", err)
	}
	return toJSON(map[string]any{"staged": true, "workflowId": id}), nil
}

// --- activate_workflow -----------------------------------------------------

type ActivateWorkflow struct{ svc *workflowapp.Service }

func (t *ActivateWorkflow) Name() string { return "activate_workflow" }

func (t *ActivateWorkflow) Description() string {
	return "Bring a workflow online: it starts listening to its trigger continuously and reacts to every real fire (going live). Use this to deploy a workflow into production. Fails if its entry is not a real trigger source. To run it just once instead, use trigger_workflow (now) or stage_workflow (next real fire)."
}

func (t *ActivateWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {"workflowId": {"type": "string"}}
	}`)
}

func (t *ActivateWorkflow) ValidateInput(args json.RawMessage) error {
	return validateWorkflowID(args, "activate_workflow")
}

func (t *ActivateWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	id, err := workflowIDArg(argsJSON, "activate_workflow")
	if err != nil {
		return "", err
	}
	wf, err := t.svc.Activate(ctx, id)
	if err != nil {
		return "", fmt.Errorf("activate_workflow: %w", err)
	}
	return toJSON(map[string]any{"workflowId": id, "lifecycleState": wf.LifecycleState, "active": wf.Active}), nil
}

// --- deactivate_workflow ---------------------------------------------------

type DeactivateWorkflow struct{ svc *workflowapp.Service }

func (t *DeactivateWorkflow) Name() string { return "deactivate_workflow" }

func (t *DeactivateWorkflow) Description() string {
	return "Take a workflow offline gracefully: it stops listening for new triggers, but any run already in flight is left to finish. Use this to retire a live workflow without disrupting work in progress. To also abort the running executions, use kill_workflow instead."
}

func (t *DeactivateWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {"workflowId": {"type": "string"}}
	}`)
}

func (t *DeactivateWorkflow) ValidateInput(args json.RawMessage) error {
	return validateWorkflowID(args, "deactivate_workflow")
}

func (t *DeactivateWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	id, err := workflowIDArg(argsJSON, "deactivate_workflow")
	if err != nil {
		return "", err
	}
	wf, err := t.svc.Deactivate(ctx, id)
	if err != nil {
		return "", fmt.Errorf("deactivate_workflow: %w", err)
	}
	return toJSON(map[string]any{"workflowId": id, "lifecycleState": wf.LifecycleState, "active": wf.Active}), nil
}

// --- kill_workflow ---------------------------------------------------------

type KillWorkflow struct{ svc *workflowapp.Service }

func (t *KillWorkflow) Name() string { return "kill_workflow" }

func (t *KillWorkflow) Description() string {
	return "Hard-stop a workflow: stop listening for triggers AND immediately cancel every run currently in flight (interrupting even a long-running step). Use this as an emergency stop when a workflow is misbehaving or runaway. For a graceful stop that lets in-flight runs finish, use deactivate_workflow. Returns how many runs were killed."
}

func (t *KillWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {"workflowId": {"type": "string"}}
	}`)
}

func (t *KillWorkflow) ValidateInput(args json.RawMessage) error {
	return validateWorkflowID(args, "kill_workflow")
}

func (t *KillWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	id, err := workflowIDArg(argsJSON, "kill_workflow")
	if err != nil {
		return "", err
	}
	killed, err := t.svc.Kill(ctx, id)
	if err != nil {
		return "", fmt.Errorf("kill_workflow: %w", err)
	}
	return toJSON(map[string]any{"workflowId": id, "killed": killed}), nil
}

// --- shared {workflowId}-only arg helpers ----------------------------------

func validateWorkflowID(args json.RawMessage, tool string) error {
	var a struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("%s: bad args: %w", tool, err)
	}
	if a.WorkflowID == "" {
		return fmt.Errorf("%s: workflowId is required", tool)
	}
	return nil
}

func workflowIDArg(argsJSON, tool string) (string, error) {
	var a struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("%s: bad args: %w", tool, err)
	}
	return a.WorkflowID, nil
}
