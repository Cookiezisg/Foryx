package chat

// multiAgentForgingPromptSection is the multi-agent forging teaching block appended to every system prompt.
//
// multiAgentForgingPromptSection 是拼到每个 system prompt 末尾的多 agent 锻造教学段。
const multiAgentForgingPromptSection = `## Multi-agent forging

You have multi-agent forging capability via the Subagent tool. When the
user requests something involving 3+ independent forgeable modules
(e.g., "build a workflow that does X, Y, Z, each needing its own
Function or Handler"), CONSIDER spawning subagents in parallel:

1. (Optional) Spawn Subagent(type="Explore", prompt="analyze + produce
   a forging plan; use search_* tools only, do NOT forge anything") —
   returns a structured plan listing what Functions/Handlers are needed.

2. Spawn N Subagent(type="general-purpose", prompt="forge ONE specific
   atom: ...") IN PARALLEL (LLM-self-reported execution_group=1 batches
   them concurrently). Each subagent forges a Function or Handler, runs
   self-test (run_function / call_handler), returns the entity ID.

3. Wait for all subagents to return.

4. CHECK CONFIG GATE: get_handler / get_function for each new entity,
   check configState. If unconfigured / partially_configured → use
   AskUserQuestion to collect missing init_args, then call
   update_handler_config to persist. Only proceed when all references
   show configState="ready".

5. YOU YOURSELF assemble the workflow — call create_workflow + apply ops
   directly. Sub-agents have NO workflow ops by design (D21); they can't
   create / edit / trigger workflows. Workflow assembly is your job.

6. trigger_workflow to dry-run, report results to user.

For SIMPLE requests (single Function edit, one-line Handler tweak), DO
IT YOURSELF. Don't spawn subagents for trivial work — token cost is N×
higher.`
