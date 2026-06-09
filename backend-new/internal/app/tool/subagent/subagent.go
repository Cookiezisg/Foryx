// Package subagent is the Subagent (Task) tool: the LLM-facing tool that spawns an isolated
// sub-agent over a focused task and returns its final answer. It is a thin shell over a Runner
// port (the subagentapp.Service) — recursion is refused here (a subagent's tool set never
// includes this tool, and a subagent ctx is rejected as a second guard), the spawning tool_call
// anchor flows through ctx (loop seeded it), and a spawn failure degrades to a tool-result string
// (no HTTP error — tool failures are reported to the LLM, not bubbled).
//
// Package subagent 是 Subagent（Task）工具：面向 LLM、在一段聚焦任务上派隔离子 agent 并返回其最终
// 答案的工具。它是 Runner 端口（subagentapp.Service）的薄壳——递归在此拒（subagent 的工具集本就不含
// 本工具，且 subagent ctx 作第二道守卫被拒），派它的 tool_call 锚点经 ctx 流（loop 已埋），派发失败
// 降级为 tool-result 串（无 HTTP 错——工具失败报给 LLM、不冒泡）。
package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

var _ toolapp.Tool = (*Tool)(nil)

// Runner is the spawn port (subagentapp.Service satisfies it; mirrors skilldomain.SubagentRunner).
//
// Runner 是派发端口（subagentapp.Service 满足；与 skilldomain.SubagentRunner 同形）。
type Runner interface {
	Spawn(ctx context.Context, agentType, prompt string) (result string, err error)
}

// Tool is the Subagent (Task) tool. agentTypes is the valid type enum (from the runner's
// registry) used for the schema + validation.
//
// Tool 是 Subagent（Task）工具。agentTypes 是合法类型 enum（取自 runner 注册表）供 schema + 校验。
type Tool struct {
	runner     Runner
	agentTypes []string
}

// New constructs the tool with the runner + the valid agent-type names.
//
// New 用 runner + 合法 agent-type 名构造工具。
func New(runner Runner, agentTypes []string) *Tool {
	return &Tool{runner: runner, agentTypes: agentTypes}
}

func (t *Tool) Name() string { return "Subagent" }

func (t *Tool) Description() string {
	return "Spawn an isolated subagent to carry out a focused sub-task and return its result. " +
		"Pick a type: Explore (read-only code reconnaissance — locate files/definitions/usages), " +
		"Plan (investigate and produce an implementation plan), or general-purpose (a focused worker " +
		"with your tools). Give a self-contained prompt — the subagent has no access to this " +
		"conversation's history. The subagent cannot spawn further subagents."
}

func (t *Tool) Parameters() json.RawMessage {
	enum, _ := json.Marshal(t.agentTypes)
	return json.RawMessage(`{
		"type": "object",
		"required": ["subagent_type", "prompt"],
		"properties": {
			"subagent_type": {"type": "string", "enum": ` + string(enum) + `, "description": "Which built-in subagent to run."},
			"prompt": {"type": "string", "description": "A self-contained task description for the subagent (it sees none of this conversation)."}
		}
	}`)
}

type args struct {
	SubagentType string `json:"subagent_type"`
	Prompt       string `json:"prompt"`
}

func (t *Tool) parse(argsJSON []byte) (args, error) {
	var a args
	if err := json.Unmarshal(argsJSON, &a); err != nil {
		return a, fmt.Errorf("invalid arguments: %w", err)
	}
	a.SubagentType = strings.TrimSpace(a.SubagentType)
	a.Prompt = strings.TrimSpace(a.Prompt)
	return a, nil
}

func (t *Tool) ValidateInput(argsJSON json.RawMessage) error {
	a, err := t.parse(argsJSON)
	if err != nil {
		return err
	}
	if a.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if !slices.Contains(t.agentTypes, a.SubagentType) {
		return fmt.Errorf("subagent_type must be one of %v", t.agentTypes)
	}
	return nil
}

// Execute refuses recursion (a subagent run is already marked in ctx), then spawns. The spawning
// tool_call id (the E3 anchor) is read from ctx by the runner. A spawn error is returned as the
// error half so loop renders it as the tool_result (the LLM can retry / adjust).
//
// Execute 拒递归（subagent run 已在 ctx 标记），再派。派它的 tool_call id（E3 锚）由 runner 从 ctx
// 读。派发错作 error 半边返回，使 loop 渲成 tool_result（LLM 可重试/调整）。
func (t *Tool) Execute(ctx context.Context, argsJSON string) (string, error) {
	if _, inSub := reqctxpkg.GetSubagentID(ctx); inSub {
		return "", fmt.Errorf("a subagent cannot spawn another subagent")
	}
	a, err := t.parse([]byte(argsJSON))
	if err != nil {
		return "", err
	}
	result, err := t.runner.Spawn(ctx, a.SubagentType, a.Prompt)
	if err != nil {
		return "", fmt.Errorf("subagent run failed: %w", err)
	}
	return result, nil
}
