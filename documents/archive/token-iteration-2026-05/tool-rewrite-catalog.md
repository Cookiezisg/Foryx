# 工具描述/schema 重写清单 — token 优化专项

> 6 个 subagent 按 best practice 逐一重写 64 个工具的 `Description()` + `Parameters()`。实现时照此改。
> **总账:全 64 把 desc+schema ~13.3k → ~6.2k token(省 ~53%);常驻 28 把重写后 ~3k(完整常驻含 summary 壳 + system prompt ≈ ~3.8k,对比现状 28k 降 ~86%)。**
> after description 为英文(直接进代码);token 为各 subagent 估算(char/3.6–4),口径自洽、相对值可靠。

## 重写规范(8 条)
1. Description = 一句话「做什么 + 何时用」;常驻 ≤ ~30 token,长尾(activate 后加载)可加关键约束 ≤ ~80 token。
2. 砍:礼貌语/元叙述、与 schema 重复的(type/必填)、低级技术细节(ID 格式/MIME)、教程与示例、废话。
3. 留:隐性知识(特殊格式、领域术语、工具间关系)、非显然的"何时用/不用"。
4. 命名优先:参数名自解释(`user_id` 而非 `user`),好名字下省 param description。
5. schema:只留必要参数;可选值用 enum;不重复 type/required。
6. 长尾巨头(create/edit_*)留在自身、砍废话即可 —— **不搬去 activate 返回**(长尾本就按需加载)。
7. 大响应工具加 `response_format: concise|detailed`(默认 concise),治多轮累积。
8. 保持功能/语义不变,只砍 token。

## 副产品发现(顺带,需另行处理)
- `revert_function` description 引用了**不存在的 `list_function`**(stale)→ 删。
- `create_handler` 原 op 列表**漏了 `set_python_version`**(`apply.go` 实有)→ 重写已补全。
- `call_mcp_tool` description 引用 `search_mcp`,实际工具名是 `search_mcp_tools` → 重写已改。
- `hcl_`(handler call)/ `mcl_`(mcp call)/ `ske_`(skill exec)ID 前缀**未登记在 CLAUDE.md §S15** → 补登记。

---

## function (前 ~760 → 后 ~330)
- **run_function** [常驻] `Run a function with kwargs; returns {ok, output, errorMsg, elapsedMs}.` — schema 删 ID 格式提示
- **search_function** [常驻] `Find functions in the user's library by query, ranked by relevance; get_function to inspect code before running.`
- **create_function** [长尾] `Build a new function from ops: set_meta, set_code, set_parameters required; set_return_schema/set_dependencies/set_python_version optional. v1 auto-accepts; failed venv installs auto-retry deps (≤3).`
- **edit_function** [长尾] `Apply ops to a function as a pending version for user accept/reject. ops=[] force-rebuilds the active env. Failed venv installs auto-retry deps (≤3).`
- **delete_function** [长尾] `Soft-delete a function; referencing workflows become needs_attention.`
- **revert_function** [长尾] `Restore a function's active version to an earlier accepted version number (see get_function for history).`
- **get_function** [长尾] `Full function details: code, parameters, dependencies, and any pending version. Use before running or editing.`
- **get_function_execution** [长尾] `Full detail of one execution: input + output (4KB cap) plus diagnostic hints (outputEmpty, significantlySlower).`
- **search_function_executions** [长尾] `Search the execution log; filters: functionId, versionId, status, conversationId, flowrunId, since/until (ISO8601). Returns 200-byte previews + status/latency aggregates. get_function_execution for one full row.`

## handler (前 ~1631 → 后 ~705;create_handler 单点省 ~580 tok)
- **call_handler** [常驻] `Invoke a method on a handler. Chat-scope calls are per-call (spawn → run → destroy). Streaming methods (Python yield) emit progress deltas; the final return is the tool_result. Fails if configState != ready — set init_args via update_handler_config first.`
- **search_handler** [常驻] `Search the user's handler library by natural-language query, ranked by relevance. Inspect a hit with get_handler (methods + configState) before call_handler.`
- **create_handler** [长尾] 见 subagent 输出(保留:OPS 全列表 + BODY CONTRACT 裸名规则 + type enum + identifier 规则 + sensitive 掩码 + persistence-scope;砍 counter 示例 ~700 字符)
- **edit_handler** [长尾] `Edit a handler by applying ops (same op shapes as create_handler). Creates/iterates a pending version awaiting user accept; pass ops=[] to force-rebuild the active version's env. Use update_method for in-place body changes (JSON Merge Patch). A failed venv install triggers an internal env-fix loop (≤3 LLM dep-revision retries).` + BODY CONTRACT 一行
- **delete_handler** [长尾] `Soft-delete a handler. Destroys all live instances; workflows referencing it become needs_attention.`
- **revert_handler** [长尾] `Point a handler's active version at a previously-accepted version number.`
- **get_handler** [长尾] `Full handler details: methods, init_args schema, configState (ready|partially_configured|unconfigured), pending version, and masked config (sensitive values shown as ********).`
- **update_handler_config** [长尾] `Set/merge a handler's init_args values (DB strings, API keys); null deletes a key. Encrypted at rest; sensitive values are never echoed in any tool result. Returns the new configState.`
- **get_handler_call** [长尾] `Full detail of one handler call: complete input/output (truncated at 4KB) plus computed hints (outputEmpty, significantlySlower) for diagnosis.`
- **search_handler_calls** [长尾] `Search the handler call log (filters: handlerId, versionId, method, instanceId, ownerKind, status, conversationId, flowrunId, since/until as RFC3339). Returns 200-byte input/output previews + aggregates (per-status counts, avg/p95 elapsedMs). Drill into one call via get_handler_call.`

## workflow (前 ~2300 → 后 ~1083;create_workflow 单点省最大)
- **search_workflow** [常驻] `Search the user's workflows by substring over name/description/tags (empty=list all). Returns id, name, description, tags, enabled, activeVersionId, needsAttention.`
- **create_workflow** [长尾] 见 subagent 输出(保留全部 DAG/node/port/loop 格式契约 + 节点 type enum + fromPort 规则 + loop body 绑定;砍两个 worked example)
- **edit_workflow** [长尾] 见 subagent 输出(update/delete op 形状 + iterate-same-pending 语义;add_* 形状交叉引用 create_workflow)
- **delete_workflow** [长尾] `Soft-delete a workflow: hidden from listings/search and its triggers stop firing; historical versions stay in DB.`
- **revert_workflow** [长尾] `Flip a workflow's active version to a previously accepted version number; historical versions are kept.`
- **get_workflow** [长尾] `Get a workflow with the parsed graph (nodes/edges/variables) of its active and pending versions. Use before edit_workflow to inspect current shape.`
- **get_workflow_execution** [长尾] `Get one workflow node execution by id, with full input/output JSON, error, timing, and attempts.`
- **search_workflow_executions** [长尾] `Search workflow node-execution history. Filters: flowrunId, nodeType, status (ok|failed|cancelled|timeout|skipped), conversationId. Returns 200-byte input/output previews; use get_workflow_execution for full detail.`
- **trigger_workflow** [长尾] ⚠️ 待 Phase 4 实现(见 memory `workflow-execution-engine-missing`);本专项假定其存在

## document (前 ~1392 → 后 ~727,全长尾)
- **create_document** `Create a document in the user's library. parentId nests it under another doc (Notion-style); null/omit = root. content is the full markdown body (split into child docs if >1MB). Name must be unique among siblings (auto-suffixed on collision).`
- **edit_document** `Update a document's fields; only supplied fields change. content and tags are full replacements (no diff/patch). Renaming cascades the path to all descendants. To change parent, use move_document.`
- **delete_document** `Soft-delete a document and all descendants recursively; set destructive=true. Returns the deleted count.`
- **move_document** `Reparent a document; parentId=null moves to root. position is the sibling index (0=first), omit to append. Path cascades to descendants. Cycles and self-parenting are rejected.`
- **read_document** `Load a document's full markdown body plus path, description, and tags. Use after picking a doc via search_documents/list_documents.`
- **list_documents** `List direct children one level under parentId (null/omit=root): name, description, path each. Walk the tree progressively; use search_documents for keyword search.`
- **search_documents** `Search documents by keyword over name/description/tags. Returns path + description per match so you can pick which to read. Prefer list_documents when you know the folder.`

## mcp (前 ~1014 → 后 ~545)
- **search_mcp_tools** [常驻] `Find tools on connected MCP servers by natural-language query; returns candidates with their inputSchema for a follow-up call_mcp_tool. Prefer native tools (Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch); use MCP for external integrations (browser, GitHub, SQL).`
- **call_mcp_tool** [长尾] `Invoke a tool on a connected MCP server. Discover server/tool/inputSchema via search_mcp_tools first; args must match that inputSchema. Failures return the reason as text so you can adjust args or pick another tool.`
- **install_mcp_server** [长尾] `Install an MCP server from the curated marketplace (name = a list_mcp_marketplace slug). Two-step: call without 'confirmed' to get a needs_confirmation envelope (suggested_question, required_env, required_args) — relay it via the ask tool to collect values; then call again with confirmed:true plus env/arguments to install.`
- **uninstall_mcp_server** [长尾] `Remove an installed MCP server from config and disconnect it. Use the short name it was installed under (from list_mcp_marketplace / install_mcp_server).`
- **list_mcp_marketplace** [长尾] `List the curated MCP marketplace when a capability is needed but no installed server matches (search_mcp_tools came up empty). Returns entries (name, description, runtime, category, tier 0–3, requiredEnv/requiredArgs, notes) sorted by tier then name. Then call install_mcp_server with the chosen name.`
- **get_mcp_call** [长尾] `Fetch one MCP call by id (from search_mcp_calls) with full untruncated input/output, error, server/tool, timing.`
- **search_mcp_calls** [长尾] `Search MCP call history. Returns previews (200-byte input/output snippets) plus aggregates (status counts, avg/p95 elapsed). Filterable by server, tool, status, conversation, or flowrun.`

## skill (前 ~741 → 后 ~430)
- **activate_skill** [常驻] `Load a skill's full instructions (from search_skills). Returns the substituted body, or — for context:fork skills — the output of an isolated subagent that ran it. Also pre-approves the skill's allowed-tools for this conversation.`
- **search_skills** [常驻] `Find installed skills (procedural workflows + allowed-tools bundles) relevant to a task; returns candidates with name, description, and an isFork flag (fork = runs in an isolated subagent). Then call activate_skill.`
- **get_skill_execution** [长尾] `Fetch one skill activation by id (from search_skill_executions) with full output, substitutions, fork depth, timing.`
- **search_skill_executions** [长尾] `Search skill activation history. Returns previews plus aggregates. Filterable by skill, status, conversation, flowrun, or fork depth.`

## filesystem / search / shell / web (前 ~3763 → 后 ~1463,全常驻)
- **Read** `Read a file. Absolute path; cat -n output (line-num TAB content). Defaults to first 2000 lines; use offset+limit to page. For directory listing use Glob "*".`
- **Write** `Write a file, overwriting if it exists (atomic). Absolute path; parent dir must exist. Overwrite needs a prior Read this conversation. Prefer Edit for changes.`
- **Edit** `Exact literal string replace in a file (not regex; whitespace/case matter). Read the file first. old_string must be unique unless replace_all; strip Read's line-num prefix.`
- **Glob** `Find files by glob pattern (supports ** recursion), sorted by mtime desc; returns JSON {root,matches:[{path,type,size,mtime}],total,truncated}. Use Grep for content.`
- **Grep** `Regex content search across files (ripgrep). Never call grep/rg via Bash. Filter by glob or type; output_mode files_with_matches (default) | content | count.`
- **Bash** `Run a shell command (POSIX sh; cmd.exe /c on Windows). cwd persists when the whole command is \`cd <path>\`. Output is combined stdout+stderr, capped 256KB, with an exit-code footer. Python/Node commands auto-route to a per-conversation sandbox.`
- **BashOutput** `Read new stdout/stderr from a background Bash shell (bash_id). Returns only output since the last poll, plus a status footer.`
- **KillShell** `Terminate a background Bash shell (bash_id). Idempotent.`
- **WebSearch** `Web search via the first available backend (BYOK Brave/Serper/Tavily/Bocha, else duckduckgo MCP). Returns JSON {query,source,results:[{title,url,snippet}],truncated}.`
- **WebFetch** `Fetch a URL and return an LLM summary answering prompt. Absolute http/https only; private/loopback addresses blocked.`

## todo / ask / memory / subagent (前 ~1722 → 后 ~938,全常驻)
- **TodoCreate** `Add a todo (status "pending") to the current conversation's list. Returns the todo as JSON with its id for later TodoUpdate.`
- **TodoUpdate** `Update fields of a todo; omitted fields stay. Status flow pending→in_progress→completed; "deleted" removes it. blocked_by replaces the whole list ([] clears). Returns the todo as JSON, or {deleted, id}.`
- **TodoList** `List active todos of the current conversation, in creation order. Returns {total, todos:[...]}.`
- **TodoGet** `Fetch one todo by id from the current conversation. Returns it as JSON, or a not-found message.`
- **AskUserQuestion** `Pause and ask the user a question; returns their free-form answer (or "(user skipped)" — proceed with sensible defaults). Give options as quick-picks for a choice, omit for open-ended; the user can always type instead. Blocks until they reply.`
- **read_memory** `Read a memory entry (persistent fact about the user/preferences/projects/references) by name. Names come from the memory index in your system prompt; call only when an indexed entry is relevant.`
- **write_memory** `Save a durable fact across conversations (user trait, preference/correction, current project, or external reference). Reusing an existing name updates it; recorded as AI-authored, user-editable.`
- **forget_memory** `Delete a memory by name — when it's outdated, wrong, or the user asks to forget it.`
- **Subagent** `Run a focused subtask in an isolated subagent (own context + curated tools; parent context untouched). Returns its final message. Types in schema.` — schema: `subagent_type` 改 enum["Explore","Plan","general-purpose"]

---

## 关键 schema 结构改动(非纯文本)
- `Subagent.subagent_type`:自由 string → `enum["Explore","Plan","general-purpose"]`
- `search_*_executions` / `search_*_calls` 的 `status`:成段文字 → `enum`
- 各 `get_*` / `delete_*`:删 ID 格式提示(`fn_xxx`/`hd_xxx`/`doc_xxx` 等),id 从 search 结果原样复制
- 大响应工具(search_*、list_*、read_document、call_*)考虑加 `response_format: concise|detailed`
