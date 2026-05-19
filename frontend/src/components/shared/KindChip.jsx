// KindChip — entity kind label with color coding.
// kinds: function | handler | workflow | skill | mcp
//
// boilerplate's CSS already defines .kind-chip.fn/.hd/.wf/.sk/.mcp.

const META = {
  function: { cls: "fn", label: "Function" },
  handler:  { cls: "hd", label: "Handler" },
  workflow: { cls: "wf", label: "Workflow" },
  skill:    { cls: "sk", label: "Skill" },
  mcp:      { cls: "mcp", label: "MCP" },
};

export function KindChip({ kind }) {
  const m = META[kind] || { cls: "fn", label: kind };
  return <span className={`kind-chip ${m.cls}`}>{m.label}</span>;
}
