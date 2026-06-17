// selfiter_confirm_f1batch_test.go — BATCH VERIFY for F1's GENERALIZE step. F1 (model guesses a
// lazy tool's id param name because the overview one-liner didn't name it) was found systemic:
// 49/50 id-taking tools shared the gap. The fix is at the foundation (toolset.Overview surfaces
// each tool's required arg names; chat renders "name(args): purpose"), so it must work for OTHER
// entities too — not just function. This probes get_handler BY NAME, and its dumped system prompt
// lets the judge confirm the param-naming now covers handler/workflow/agent/... tools. Re-run
// alongside TestConfirmF1 (function) for the cross-entity before/after. EVALS=1 gated.
//
// selfiter_confirm_f1batch_test.go —— F1 GENERALIZE 步的批量 VERIFY。F1（模型瞎猜 lazy 工具 id 参数名，
// 因概览一句话没点名）已证系统性：49/50 取 id 的工具同病。修在地基（toolset.Overview 浮出每个工具的必填
// 参数名；chat 渲成 name(args): purpose），故对**别的实体**也该生效、非只 function。本 probe 按名查
// get_handler，其 dump 的 system prompt 供判官核对参数点名已覆盖 handler/workflow/agent… 工具。
package golden

import "testing"

// ── 跨实体批量验证：按名字查 handler（get_handler，需 handlerId）─────────────────
func TestConfirmF1Batch_HandlerByName(t *testing.T) {
	outDir := trajOut(t)
	wc := evalWS(t)
	wc.POST("/api/v1/handlers", map[string]any{
		"name":        "inventory_tracker",
		"description": "track item counts in memory",
		"initBody":    "self.count = 0",
		"methods": []map[string]any{
			{"name": "add_one", "inputs": []any{}, "body": "self.count = self.count + 1\nreturn {\"count\": self.count}"},
		},
	}).OK(t, nil)
	conv := newConv(t, wc, "confirm f1 batch: handler by name")
	defer dumpTrajectory(t, wc, conv, outDir, "confirm_f1b_handler")
	say(t, wc, conv, "What methods does my handler named `inventory_tracker` have, and what does each one do?", 180000)
}
