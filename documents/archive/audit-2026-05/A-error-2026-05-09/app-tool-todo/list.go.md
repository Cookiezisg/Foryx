# Audit trace — internal/app/tool/todo/list.go

**LOC**: 72
**Purpose**: TodoList system tool (LLM-facing) — returns all active todos in current conversation. §S18 9-method contract; delegates to `*todoapp.Service.List`. Read-only.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | list.go:46 | `func (t *TodoList) ValidateInput(_ json.RawMessage) error { return nil }` | A.1 | OK | Schema is `{type:object, properties:{}}` — no inputs to validate. Empty validator is correct (matches mcp list_marketplace.go #1 pattern, accepted in mcp audit summary spot-check). | N-A | — | — | — |
| 2 | list.go:48-50 | `func (t *TodoList) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | Read-only conversation-scoped fetch; PermissionAllow correct per §S18. | N-A | — | — | — |
| 3 | list.go:55-59 | `func (t *TodoList) Execute(ctx context.Context, _ string) (string, error) { todos, err := t.svc.List(ctx); if err != nil { return classifyTodoErr(err, "list"), nil }; ...` | A.1/A.2 | OK | §S18 friendly tool_result via shared `classifyTodoErr` (see create.go #5). §S9 N/A — read-only List does not perform terminal write; ctx pass-through is correct. | N-A | — | — | — |
| 4 | list.go:67-70 | `body, err := json.MarshalIndent(out, "", "  "); if err != nil { return "", fmt.Errorf("TodoList.Execute: marshal: %w", err) }; return string(body), nil` | A.4 | OK | §S16 prefix `TodoList.Execute: marshal:` + `%w` — full literal pkg.method-style prefix. Note: stricter than the shared `marshalIndent` helper in create.go (which uses `marshal:` only) — this is INCONSISTENT but each is individually compliant or WAIVED. `json.MarshalIndent` over plain types is essentially unfailable, but the err is properly wrapped if it ever fires. | N-A | — | — | — |

## Sub-check

- **A.1 §S3 错误吞没**: not present — Service err surfaced via friendly classify; Marshal err wrapped.
- **A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: none — TodoList is a pure read.
  - 各自 ctx 来源: passed-through ctx
  - violations: N/A — read-only tool.
- **A.3 §S15 ID 生成**:
  - ID generation calls: none in this file
  - violations: N/A — TodoList does not generate IDs.
- **A.4 §S16 错误 wrap 格式**:
  - violations: not present — site #4 uses full `TodoList.Execute: marshal:` prefix + `%w`. (Mild stylistic inconsistency vs shared `marshalIndent` helper's `marshal:` only — but that's a §S16 inconsistency between files, not a violation in either.)
- **A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none in this file
  - 已登记 errmap: N/A
  - missing: N/A — file declares no sentinels (consumed sentinels via shared classify covered in create.go audit).

**Findings**: 0 violations. Cleanest of the 4 tool files — schema-empty validator, read-only permissions, full-prefix Marshal wrap.
