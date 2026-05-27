import { useState } from "react";
import { Link, useLocation } from "react-router-dom";

const SECTIONS: Array<{ label: string; routes: Array<[string, string]> }> = [
  { label: "current", routes: [
    ["/current/wire", "Wire Trace"], ["/current/eventlog", "Eventlog"], ["/current/notifications", "Notifications"],
    ["/current/subagents", "SubAgents"], ["/current/tools", "Tool Calls"], ["/current/todos", "Todos"],
    ["/current/asks", "Asks"], ["/current/attachments", "Attachments"], ["/current/compaction", "Compaction"],
  ]},
  { label: "forge", routes: [
    ["/forge/functions", "Functions"], ["/forge/handlers", "Handlers"], ["/forge/workflows", "Workflows"],
    ["/forge/tools", "Tools Registry"],
  ]},
  { label: "execute", routes: [
    ["/execute/triggers", "Triggers"], ["/execute/flowruns", "FlowRuns"],
    ["/execute/approvals", "Approvals"], ["/execute/executions", "Executions"],
  ]},
  { label: "observe", routes: [
    ["/observe/live", "Live SSE"], ["/observe/notifications", "Notif History"],
    ["/observe/catalog", "Catalog"], ["/observe/usage", "Usage"], ["/observe/mock-llm", "Mock LLM"],
  ]},
  { label: "config", routes: [
    ["/config/apikeys", "API Keys"], ["/config/models", "Models"], ["/config/skills", "Skills"],
    ["/config/mcp", "MCP Servers"], ["/config/sandbox", "Sandbox"], ["/config/memory", "Memory"],
    ["/config/documents", "Documents"], ["/config/permissions", "Permissions"],
    ["/config/llm-health", "LLM Health"], ["/config/profile", "Profile"],
  ]},
  { label: "dev", routes: [
    ["/dev/sql", "SQL"], ["/dev/info", "Info"], ["/dev/routes", "Routes"],
    ["/dev/logs", "Backend Logs"], ["/dev/processes", "Processes"], ["/dev/metrics", "Metrics"],
    ["/dev/errors", "Errors"], ["/dev/prompts", "Prompts"],
  ]},
];

export function TabNav() {
  const loc = useLocation();
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  return (
    <div style={{ height: "100%", overflowY: "auto", background: "var(--bg-sidebar)", padding: "6px 0" }}>
      {SECTIONS.map((s) => (
        <div key={s.label}>
          <div onClick={() => setCollapsed((c) => ({ ...c, [s.label]: !c[s.label] }))} style={{
            padding: "4px 12px", cursor: "pointer", fontSize: 11,
            color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: 0.5,
          }}>
            {collapsed[s.label] ? "▸" : "▾"} {s.label}
          </div>
          {!collapsed[s.label] && s.routes.map(([p, l]) => {
            const active = loc.pathname === p;
            return (
              <Link key={p} to={p} style={{
                display: "block", padding: "3px 24px", fontSize: 12, textDecoration: "none",
                color: active ? "var(--accent)" : "var(--fg-body)",
                background: active ? "var(--bg-elev)" : "transparent",
                borderLeft: "2px solid transparent",
                borderLeftColor: active ? "var(--accent)" : "transparent",
              }}>{l}</Link>
            );
          })}
        </div>
      ))}
    </div>
  );
}
