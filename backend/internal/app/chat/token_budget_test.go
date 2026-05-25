package chat

import (
	"context"
	"encoding/json"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	toolsettool "github.com/sunweilin/forgify/backend/internal/app/tool/toolset"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
)

// lazyStub is a minimal Tool stub placed in Lazy groups so activate_tools
// has a realistic group map to snapshot.
//
// lazyStub 是放 Lazy 组的最小 Tool stub，让 activate_tools 能快照到真实组。
type lazyStub struct{ name string }

func (r *lazyStub) Name() string                        { return r.name }
func (r *lazyStub) Description() string                 { return "stub" }
func (r *lazyStub) Parameters() json.RawMessage         { return json.RawMessage(`{"type":"object","properties":{}}`) }
func (r *lazyStub) IsReadOnly() bool                    { return false }
func (r *lazyStub) NeedsReadFirst() bool                { return false }
func (r *lazyStub) RequiresWorkspace() bool             { return false }
func (r *lazyStub) ValidateInput(json.RawMessage) error { return nil }
func (r *lazyStub) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (r *lazyStub) Execute(_ context.Context, _ string) (string, error) { return "", nil }

// buildResidentStubs constructs Tool stubs representing the actual resident tools with
// their production Description() and Parameters() text, giving accurate byte measurements.
// Descriptions are copied verbatim from the tool sources; schemas are structurally accurate.
func buildResidentStubs() []toolapp.Tool {
	type tool struct {
		name   string
		desc   string
		params string
	}
	defs := []tool{
		{
			name: "Read",
			desc: `Read a file. Absolute path; cat -n output (line-num TAB content). Defaults to first 2000 lines; use offset+limit to page. For directory listing use Glob "*".`,
			params: `{"type":"object","required":["file_path"],"properties":{
				"file_path":{"type":"string","description":"The absolute path to the file to read"},
				"offset":{"type":"number","description":"The line number to start reading from (1-based; default 1). Only provide if the file is too large to read at once."},
				"limit":{"type":"number","description":"The number of lines to read (default 2000). Only provide if the file is too large to read at once."}}}`,
		},
		{
			name:   "Write",
			desc:   "Write content to a file (creates dirs; overwrites). Requires Read first if file exists.",
			params: `{"type":"object","required":["file_path","content"],"properties":{"file_path":{"type":"string"},"content":{"type":"string"}}}`,
		},
		{
			name:   "Edit",
			desc:   "Exact-string replacement in a file. old_string must be unique; use replace_all to replace every occurrence. Requires Read first.",
			params: `{"type":"object","required":["file_path","old_string","new_string"],"properties":{"file_path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean","default":false}}}`,
		},
		{
			name:   "Grep",
			desc:   "Search for a regex pattern across files. Returns matching lines with file:line context.",
			params: `{"type":"object","required":["pattern","path"],"properties":{"pattern":{"type":"string"},"path":{"type":"string"},"include":{"type":"string"},"output_mode":{"type":"string"}}}`,
		},
		{
			name:   "Glob",
			desc:   "Glob file paths matching a pattern. Returns sorted matches.",
			params: `{"type":"object","required":["pattern"],"properties":{"pattern":{"type":"string","description":"Glob pattern"},"path":{"type":"string","description":"Directory to search from (default: cwd)"}}}`,
		},
		{
			name: "Bash",
			desc: "Run a shell command (POSIX sh; cmd.exe /c on Windows). cwd persists when the whole command is `cd <path>`. Output is combined stdout+stderr, capped 256KB, with an exit-code footer. Python/Node commands auto-route to a per-conversation sandbox.",
			params: `{"type":"object","required":["command"],"properties":{
				"command":{"type":"string","description":"Shell command to execute (POSIX sh)."},
				"run_in_background":{"type":"boolean","default":false,"description":"If true, spawn without waiting and return a bash_id for BashOutput / KillShell."},
				"timeout":{"type":"number","default":120000,"description":"Timeout in milliseconds (max 600000)."}}}`,
		},
		{
			name:   "BashOutput",
			desc:   "Read buffered output from a background shell command started with run_in_background.",
			params: `{"type":"object","required":["bash_id"],"properties":{"bash_id":{"type":"string","description":"ID returned by Bash run_in_background."}}}`,
		},
		{
			name:   "KillShell",
			desc:   "Kill a background shell command by ID and return its buffered output.",
			params: `{"type":"object","required":["bash_id"],"properties":{"bash_id":{"type":"string","description":"ID of the background shell to kill."}}}`,
		},
		{
			name:   "WebSearch",
			desc:   "Search the web and return a summarised list of results.",
			params: `{"type":"object","required":["query"],"properties":{"query":{"type":"string","description":"Search query"},"num_results":{"type":"integer","default":5}}}`,
		},
		{
			name:   "WebFetch",
			desc:   "Fetch a URL and return its text content (HTML stripped). Optionally summarise with a prompt.",
			params: `{"type":"object","required":["url"],"properties":{"url":{"type":"string","description":"URL to fetch"},"prompt":{"type":"string","description":"Optional: ask the LLM to answer this question about the page content."}}}`,
		},
		{
			name:   "AskUserQuestion",
			desc:   "Pause to ask the user a clarifying question. Use when blocked; prefer short plain text.",
			params: `{"type":"object","required":["question"],"properties":{"question":{"type":"string","description":"The question to ask the user."}}}`,
		},
		{
			name:   "search_function",
			desc:   "Search functions by name, tags, or description. Returns a list of matches with IDs.",
			params: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","default":20}}}`,
		},
		{
			name:   "search_handler",
			desc:   "Search handlers by name, tags, or description. Returns a list of matches with IDs.",
			params: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","default":20}}}`,
		},
		{
			name:   "search_workflow",
			desc:   "Search workflows by name, tags, or description. Returns a list of matches with IDs.",
			params: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","default":20}}}`,
		},
		{
			name:   "search_skills",
			desc:   "Search skills by name or description. Returns a list of matches.",
			params: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","default":20}}}`,
		},
		{
			name:   "search_mcp_tools",
			desc:   "Search installed MCP server tools by name or description.",
			params: `{"type":"object","properties":{"query":{"type":"string"},"serverName":{"type":"string","description":"Optional: filter by server name."}}}`,
		},
		{
			name:   "run_function",
			desc:   "Run the active version of a function synchronously and return its output.",
			params: `{"type":"object","required":["id"],"properties":{"id":{"type":"string","description":"Function ID"},"input":{"type":"object","description":"Input matching the function parameters schema."}}}`,
		},
		{
			name:   "call_handler",
			desc:   "Invoke a method on a handler instance. The handler must be configured (configState=ready).",
			params: `{"type":"object","required":["handlerName","methodName"],"properties":{"handlerName":{"type":"string"},"methodName":{"type":"string"},"args":{"type":"object"}}}`,
		},
		{
			name:   "read_memory",
			desc:   "Read all memory entries for the current user.",
			params: `{"type":"object","properties":{"filter":{"type":"string","description":"Optional substring filter."}}}`,
		},
		{
			name:   "write_memory",
			desc:   "Write or update a memory entry identified by key.",
			params: `{"type":"object","required":["key","value"],"properties":{"key":{"type":"string"},"value":{"type":"string"}}}`,
		},
		{
			name:   "forget_memory",
			desc:   "Delete a memory entry by key.",
			params: `{"type":"object","required":["key"],"properties":{"key":{"type":"string"}}}`,
		},
		{
			name:   "TodoCreate",
			desc:   "Create a new todo item.",
			params: `{"type":"object","required":["content"],"properties":{"content":{"type":"string"},"priority":{"type":"string","enum":["low","medium","high"]}}}`,
		},
		{
			name:   "TodoUpdate",
			desc:   "Update an existing todo item's status or content.",
			params: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"status":{"type":"string"},"content":{"type":"string"}}}`,
		},
		{
			name:   "TodoList",
			desc:   "List all todo items, optionally filtered by status.",
			params: `{"type":"object","properties":{"status":{"type":"string","enum":["pending","in_progress","completed","cancelled"]}}}`,
		},
		{
			name:   "TodoGet",
			desc:   "Get a single todo item by ID.",
			params: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		},
		{
			name:   "activate_skill",
			desc:   "Activate a named skill, loading its allowed-tools list and system prompt extension.",
			params: `{"type":"object","required":["skillName"],"properties":{"skillName":{"type":"string","description":"Name of the skill to activate."}}}`,
		},
		{
			name:   "Subagent",
			desc:   "Spawn a subagent for a focused subtask (Explore or general-purpose). Returns the subagent's final message.",
			params: `{"type":"object","required":["type","prompt"],"properties":{"type":{"type":"string","enum":["Explore","general-purpose"]},"prompt":{"type":"string","description":"Task for the subagent."}}}`,
		},
	}

	tools := make([]toolapp.Tool, len(defs))
	for i, d := range defs {
		tools[i] = &stubTool{name: d.name}
		// stubTool.Parameters() returns the empty schema; override via type assertion
		// by casting to a thin wrapper that carries the real description + params.
		tools[i] = &residentToolStub{name: d.name, desc: d.desc, params: json.RawMessage(d.params)}
	}
	return tools
}

// residentToolStub satisfies toolapp.Tool with a realistic description + parameters payload.
type residentToolStub struct {
	name   string
	desc   string
	params json.RawMessage
}

func (r *residentToolStub) Name() string                        { return r.name }
func (r *residentToolStub) Description() string                 { return r.desc }
func (r *residentToolStub) Parameters() json.RawMessage         { return r.params }
func (r *residentToolStub) IsReadOnly() bool                    { return true }
func (r *residentToolStub) NeedsReadFirst() bool                { return false }
func (r *residentToolStub) RequiresWorkspace() bool             { return false }
func (r *residentToolStub) ValidateInput(json.RawMessage) error { return nil }
func (r *residentToolStub) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (r *residentToolStub) Execute(_ context.Context, _ string) (string, error) { return "", nil }

// TestResidentContext_UnderBudget measures the on-wire byte size of the resident context
// sent to the LLM on a fresh user's first "Hello" turn: system prompt + resident ToolDefs.
//
// The capability-disclosure refactor goal was ≈3.8k resident tokens (down from ≈28k when
// all tools were always sent). Guard threshold is 24 000 bytes (≈6k tokens at ~4 bytes/token)
// — generous headroom so normal description edits don't trip the gate.
//
// TestResidentContext_UnderBudget 测量首回合发给 LLM 的 context 实际字节数：
// system prompt + resident ToolDef JSON。目标 ≈3.8k tokens；守卫上限 24000 bytes。
func TestResidentContext_UnderBudget(t *testing.T) {
	residentTools := buildResidentStubs()

	// Construct a Toolset mirroring production: resident tools + representative lazy groups
	// (needed so activate_tools.Description() carries the category list).
	ts := toolapp.Toolset{
		Resident: residentTools,
		Lazy: map[string][]toolapp.Tool{
			"function": {&lazyStub{"create_function"}},
			"handler":  {&lazyStub{"create_handler"}},
			"workflow": {&lazyStub{"create_workflow"}},
			"mcp":      {&lazyStub{"call_mcp_tool"}},
			"document": {&lazyStub{"create_document"}},
			"skill":    {&lazyStub{"get_skill_execution"}},
		},
	}
	ts.Resident = append(ts.Resident, toolsettool.NewActivateTools(ts))

	// Serialize resident ToolDefs to JSON — this is the on-wire tool list bytes.
	defs := toolapp.ToLLMDefs(ts.Resident)
	toolDefsJSON, err := json.Marshal(defs)
	if err != nil {
		t.Fatalf("marshal tool defs: %v", err)
	}

	// Build system prompt for a bare conversation (no memory, docs, or catalog assets).
	svc := &Service{toolset: ts}
	conv := &convdomain.Conversation{}
	systemPrompt := svc.buildSystemPrompt(context.Background(), conv)

	totalBytes := len(systemPrompt) + len(toolDefsJSON)
	approxTokens := totalBytes / 4

	t.Logf("Resident context breakdown:")
	t.Logf("  system prompt  : %d bytes", len(systemPrompt))
	t.Logf("  tool defs JSON : %d bytes (%d tools)", len(toolDefsJSON), len(ts.Resident))
	t.Logf("  TOTAL          : %d bytes (~%d tokens @ 4 bytes/token)", totalBytes, approxTokens)

	const maxBytes = 24_000
	if totalBytes >= maxBytes {
		t.Errorf("resident context %d bytes >= %d-byte ceiling (~%d tokens); refactor goal violated",
			totalBytes, maxBytes, approxTokens)
	}
}
