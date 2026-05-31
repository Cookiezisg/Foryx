# 基线工具描述(as-tested,91 工具)——「现状」原始参考

> **这是被测的基线工具描述(91 个 `Description()` 原文 + 参数),即 before→after 里的「现状」。** 由 `research/llm-experiments/render_spec.py` 从 `spec_catalog.py` 渲染。
> **优化后(ship-this)的逐项 before→after 见 [`../15-tool-catalog.md`](../15-tool-catalog.md)**;实验依据见 [`../14-llm-validation-research-record.md`](../14-llm-validation-research-record.md)。
> 合计 **91** 工具。被测 deepseek-v4-flash。

## Function 锻造(11)
_验证模式:create/edit=CODE(真执行 90%);余 USAGE_

### `search_functions`
Find functions by name/tag/description (optional kind filter). Returns ids. Use before get/edit when you don't have the id; returns empty if none exist.
必填:`query`  可选:`kind`

### `get_function`
Get a function's active version: code + signature + kind. Read before editing.
必填:`id`

### `get_function_versions`
List a function's version history (who/when/change reason). Use to compare or pick a revert target.
必填:`id`

### `create_function`
Create a stateless Python function. kind=normal (on-demand callable) | polling (system runs on interval; must accept last_cursor, return {events,next_cursor}).
必填:`name, kind, code`  可选:`polling_interval`

### `edit_function`
Edit a function via ops (update_code replaces the body; update_kind/update_polling_interval/update_description). Preserve existing behavior unless asked.
必填:`id, ops`

### `accept_pending_function`
Promote a function's pending version to active. Call after create/edit once satisfied.
必填:`id`

### `revert_function`
Revert a function to a previous version (creates a new pending from it).
必填:`id, targetVersion`

### `delete_function`
Soft-delete a function. Fails if still referenced by a workflow/agent.
必填:`id`

### `run_function`
Test-run a function with args (polling: platform supplies a mock last_cursor). Use to verify before wiring.
必填:`id, args`

### `search_function_executions`
List a function's past executions (filter by time).
必填:`id`  可选:`since`

### `get_function_execution`
Get one function execution's detail (args, result, timing, error).
必填:`executionId`

## Handler 锻造(12)
_验证模式:create/edit=CODE(真执行 100%);余 USAGE_

### `search_handlers`
Find stateful handlers by name/tag/description. Returns ids.
必填:`query`

### `get_handler`
Get a handler's class definition + init schema + methods schema. Read before editing.
必填:`id`

### `get_handler_versions`
List a handler's version history.
必填:`id`

### `create_handler`
Create a stateful Python class handler (holds connections/cache/tokens). Body uses BARE-NAMED params on __init__ and each method (not a dict).
必填:`name, code, init_schema, methods_schema`

### `edit_handler`
Edit a handler via ops (update_code / update schemas). Mind state persistence + thread safety.
必填:`id, ops`

### `accept_pending_handler`
Promote a handler's pending version to active.
必填:`id`

### `revert_handler`
Revert a handler to a previous version.
必填:`id, targetVersion`

### `delete_handler`
Soft-delete a handler (fails if referenced).
必填:`id`

### `call_handler`
Test-call one handler method (instantiates with init args, then calls). Use to verify behavior.
必填:`id, method, args`

### `update_handler_config`
Set a handler's init args / secrets (stored encrypted). Separate from code edits.
必填:`id, config`

### `search_handler_calls`
List a handler's past calls (filter by time).
必填:`id`  可选:`since`

### `get_handler_call`
Get one handler call's detail.
必填:`callId`

## Agent 锻造(11)
_验证模式:create/edit=ARTIFACT(90%);余 USAGE_

### `search_agents`
Find agents (configured LLM workers) by name/tag/description. Returns ids.
必填:`query`

### `get_agent`
Get an agent's active config: prompt / skill / knowledge / tools / outputSchema / model. Read before editing.
必填:`id`

### `get_agent_versions`
List an agent's version history.
必填:`id`

### `create_agent`
Create an agent (configured LLM worker). Mounts: prompt, skill(0-1), knowledge(docs), tools(fn/hd/mcp only — never platform tools, never another agent), outputSchema(enum|json_schema|free_text), model.
必填:`name, ops`

### `edit_agent`
Edit an agent via ops. set_tools REPLACES the list — include existing tools to keep them.
必填:`id, ops`

### `accept_pending_agent`
Promote an agent's pending version to active.
必填:`id`

### `revert_agent`
Revert an agent to a previous version.
必填:`id, targetVersion`

### `delete_agent`
Soft-delete an agent (fails if referenced).
必填:`id`

### `run_agent`
Test-run an agent with a payload; returns output + tokens + latency. Verify before wiring.
必填:`id, payload`

### `search_agent_executions`
List an agent's past runs.
必填:`id`  可选:`since`

### `get_agent_execution`
Get one agent run's detail (prompt, tool-call chain, output).
必填:`executionId`

## Workflow 编排(9)
_验证模式:create/edit=ARTIFACT(55%→check/fix);余 USAGE_

### `search_workflows`
Find workflows by name/tag/description (optional active filter). Returns ids + active state.
必填:`query`  可选:`active`

### `get_workflow`
Get a workflow's full graph (nodes + edges + each node config). Read before editing.
必填:`id`

### `get_workflow_versions`
List a workflow's version history.
必填:`id`

### `create_workflow`
Create a workflow graph (5 node types: trigger/agent/tool/case/approval) via ops. Initial version auto-accepts.
必填:`name, ops`

### `edit_workflow`
Edit a workflow graph via ops (add_node/remove_node/connect/disconnect/update_config). Reference existing node ids.
必填:`id, ops`

### `accept_pending_workflow`
Promote a workflow's pending version to active.
必填:`id`

### `revert_workflow`
Revert a workflow to a previous version.
必填:`id, targetVersion`

### `delete_workflow`
Soft-delete a workflow.
必填:`id`

### `capability_check_workflow`
Pre-check a workflow before activating: every callable ref exists + kinds match + payload schemas flow. Returns first blocking problem.
必填:`id`

## Workflow 生命周期(3)
_验证模式:USAGE / trigger=ARTIFACT(payload)_

### `activate_workflow`
Activate a workflow: register its triggers/listeners, set active=true. Fails capability_check if it references missing callables.
必填:`id`

### `deactivate_workflow`
Deactivate a workflow: remove listeners, destroy owner=workflow instances, set active=false.
必填:`id`

### `trigger_workflow`
Fire a workflow from a specific trigger node with a payload. Use for manual nodes (product) or to test-fire a cron/webhook node (debug).
必填:`id, triggerNodeId, payload`

## 运行时观察(5)
_验证模式:USAGE(诊断链 6/7)_

### `search_flowruns`
List a workflow's runs (filter status/time). Start here to find failed runs.
必填:`workflowId`  可选:`status, since`

### `get_flowrun`
Get one run's summary (status / which node failed / timing).
必填:`id`

### `get_flowrun_trace`
Get a run's message causal chain (node-by-node, with errors). Use to see WHERE/WHY it failed.
必填:`id`

### `get_flowrun_nodes`
Get per-node state of a run (running/completed/failed/approval-pending).
必填:`id`

### `cancel_flowrun`
Cancel a stuck/running flowrun.
必填:`id`

## 错误诊断 + 修复(5)
_验证模式:USAGE(诊断链 6/7)_

### `query_events`
Query a workflow's event log (handler_crash / message_failed / trigger_exhausted / dead_letter_created). Spot recurring failure types.
必填:`workflowId`  可选:`type, since`

### `list_dead_letters`
List dead-lettered messages (retries exhausted) for a workflow.
必填:`workflowId`

### `get_dead_letter`
Get a dead letter's detail (payload + ctx + failure reason + stack). Root-cause here.
必填:`messageId`

### `replay_message`
Replay a dead-lettered/failed message (from its node or whole run). ONLY after the root cause is fixed.
必填:`messageId`  可选:`fromNode`

### `clear_dead_letters`
Bulk-clear a workflow's dead letters (after handling them).
必填:`workflowId`

## 资产 — MCP(5)
_验证模式:USAGE / call=ARTIFACT(args);lazy 激活验证_

### `search_mcp_tools`
Search installed MCP servers' tools by capability. Returns server/tool names.
必填:`query`

### `call_mcp_tool`
Call an MCP tool (server + tool + args). The mcp group must be activated first.
必填:`server, tool, args`

### `list_mcp_servers`
List installed MCP servers + their tools + health.

### `install_mcp_from_registry`
Install an MCP server from the registry by name. Use when a needed integration isn't installed yet.
必填:`name`

### `health_check_mcp`
Check one MCP server's health/connectivity.
必填:`server`

## 资产 — Skill(3)
_验证模式:USAGE_

### `search_skills`
Find skills (reusable methodologies) by name/description.
必填:`query`

### `get_skill`
Read a skill's full content/instructions.
必填:`name`

### `activate_skill`
Activate a skill into the current conversation (loads its methodology). For chat use; agents pin a skill on the entity instead.
必填:`name`

## 资产 — Document(7)
_验证模式:create/edit=CONTENT(100%);余 USAGE_

### `search_documents`
Find documents by content/title/tag. Returns ids.
必填:`query`

### `list_documents`
List documents in a folder/path.
可选:`path`

### `read_document`
Read a document's full content.
必填:`id`

### `create_document`
Create a knowledge document (markdown). Use for notes/knowledge an agent can mount, NOT for code (forge a function/handler).
必填:`title, content`  可选:`path`

### `edit_document`
Edit a document's content.
必填:`id, content`

### `move_document`
Move/rename a document.
必填:`id, path`

### `delete_document`
Soft-delete a document.
必填:`id`

## 资产 — Memory(3)
_验证模式:write=CONTENT(100%);余 USAGE_

### `read_memory`
Read long-term memory entries matching a query (cross-conversation facts the chat boss remembers).
必填:`query`

### `write_memory`
Save a durable fact/preference to long-term memory. Use for things to remember across conversations, NOT transient task state.
必填:`name, content`

### `forget_memory`
Delete a long-term memory entry.
必填:`name`

## 主对话基础(17)
_验证模式:USAGE(91 全集选择 ~91% reasonable)_

### `Read`
Read a local file from disk. For the user's own files — NOT for Forgify entities (use get_function/get_workflow/read_document).
必填:`file_path`

### `Write`
Write/overwrite a local file on disk.
必填:`file_path, content`

### `Edit`
Edit a local file by string replacement.
必填:`file_path, old_string, new_string`

### `Glob`
Find files by glob pattern.
必填:`pattern`

### `Grep`
Search file contents by regex.
必填:`pattern`  可选:`path`

### `Bash`
Run a shell command on the user's machine. For local system tasks — to give a workflow shell capability, forge a function instead.
必填:`command`

### `BashOutput`
Get output from a running background shell.
必填:`bash_id`

### `KillShell`
Kill a background shell.
必填:`shell_id`

### `WebFetch`
Fetch + extract content from a URL. For the chat boss's research — to give a workflow web access, forge a function.
必填:`url`

### `WebSearch`
Search the web. For the chat boss's research.
必填:`query`

### `TodoCreate`
Create a todo to track multi-step work in THIS conversation.
必填:`items`

### `TodoList`
List this conversation's todos.

### `TodoGet`
Get one todo's detail.
必填:`id`

### `TodoUpdate`
Update a todo's status.
必填:`id, status`

### `AskUserQuestion`
Ask the user a clarifying question when genuinely blocked on a decision only they can make. Don't ask what you can decide or look up.
必填:`question`

### `Subagent`
Spawn a subagent for a parallel/independent exploration. Chat-boss only.
必填:`prompt`

### `activate_tools`
Load a lazy tool group (function/handler/workflow/mcp/document/skill) into your available tools. Activate ONLY the group the immediate task needs; don't speculatively activate several.
必填:`category`
