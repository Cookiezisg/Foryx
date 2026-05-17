/* eslint-disable react/prop-types */
// Recursive block renderer. Handles all 6 block types from event-log protocol:
//   text · reasoning · tool_call · tool_result · progress · message (subagent nested)

const { useState } = React;

// ── Utilities ─────────────────────────────────────────────────────────────
function fmtDuration(ms) {
  if (!ms && ms !== 0) return "";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const m = Math.floor(ms / 60_000);
  const s = Math.round((ms % 60_000) / 1000);
  return `${m}m ${s}s`;
}

function toolIcon(name) {
  const map = {
    search_forges: Icon.Search,
    get_forge: Icon.Folder,
    create_forge: Icon.Hammer,
    create_workflow: Icon.Workflow,
    edit_forge: Icon.Wrench,
    run_forge: Icon.Play,
    Read: Icon.FileText,
    Write: Icon.FileText,
    Edit: Icon.FileText,
    Bash: Icon.Terminal,
    WebFetch: Icon.Globe,
    WebSearch: Icon.Search,
    Grep: Icon.Search,
    Glob: Icon.Folder,
    Subagent: Icon.Bot,
    AskUserQuestion: Icon.HelpCircle,
  };
  return map[name] || Icon.Wrench;
}

// ── Reasoning block ───────────────────────────────────────────────────────
function ReasoningBlock({ block, defaultOpen }) {
  const [open, setOpen] = useState(!!defaultOpen);
  return (
    <div className={"blk-reasoning" + (open ? " is-open" : "")}>
      <button className="blk-reasoning-head" onClick={() => setOpen(o => !o)}>
        <Icon.ChevronRight />
        <Icon.Brain style={{ width: 12, height: 12 }} />
        <span>已思考 {fmtDuration(block.durationMs)}</span>
        {block.status === "streaming" && <span className="dot-pulse"><span /><span /><span /></span>}
        <span className="meta">{block.content.length} chars</span>
      </button>
      {open && <div className="blk-reasoning-body">{block.content}</div>}
    </div>
  );
}

// ── Text block (markdown-ish minimal) ─────────────────────────────────────
function TextBlock({ block }) {
  // Minimal inline formatter: **bold**, `code`, line breaks, leading-dash bullets.
  const lines = block.content.split("\n");
  const out = [];
  let bullets = null;
  const flush = () => { if (bullets) { out.push(<ul key={"u" + out.length}>{bullets}</ul>); bullets = null; } };
  lines.forEach((line, i) => {
    const trimmed = line.replace(/^\s+/, "");
    if (trimmed.startsWith("- ")) {
      bullets = bullets || [];
      bullets.push(<li key={i}>{renderInline(trimmed.slice(2))}</li>);
    } else if (line.trim() === "") {
      flush();
    } else {
      flush();
      out.push(<p key={i}>{renderInline(line)}</p>);
    }
  });
  flush();
  return (
    <div className="blk-text">
      {out}
      {block.status === "streaming" && <span className="streaming-caret" />}
    </div>
  );
}

function renderInline(s) {
  const parts = [];
  // Match: entity IDs (fn_/hd_/wf_/sk_/mcp_/mem_/cv_/fr_ + hex) | **bold** | `code`
  const re = /(\b(?:fn|hd|wf|sk|mcp|mem|cv|fr)_[a-z0-9]{2,16}\b|\*\*[^*]+\*\*|`[^`]+`)/g;
  let last = 0;
  s.replace(re, (m, _g, idx) => {
    if (idx > last) parts.push(s.slice(last, idx));
    if (m.startsWith("**")) {
      parts.push(<strong key={idx}>{m.slice(2, -2)}</strong>);
    } else if (m.startsWith("`")) {
      // Backtick code — still check if it's an entity inside
      const inner = m.slice(1, -1);
      if (/^(fn|hd|wf|sk|mcp|mem|cv|fr)_[a-z0-9]{2,16}$/.test(inner)) {
        parts.push(<EntityLink key={idx} id={inner} />);
      } else {
        parts.push(<code key={idx}>{inner}</code>);
      }
    } else {
      parts.push(<EntityLink key={idx} id={m} />);
    }
    last = idx + m.length;
    return m;
  });
  if (last < s.length) parts.push(s.slice(last));
  return parts;
}

function EntityLink({ id }) {
  const prefix = id.split("_")[0];
  const meta = {
    fn:  { pane: "forge",     icon: "Code",        label: "Function" },
    hd:  { pane: "forge",     icon: "Server",      label: "Handler" },
    wf:  { pane: "forge",     icon: "Workflow",    label: "Workflow" },
    sk:  { pane: "skills",    icon: "Sparkles",    label: "Skill" },
    mcp: { pane: "mcp",       icon: "Server",      label: "MCP" },
    mem: { pane: "memory",    icon: "Brain",       label: "Memory" },
    cv:  { pane: "chat",      icon: "MessageSquare", label: "对话" },
    fr:  { pane: "execute",   icon: "Play",        label: "FlowRun" },
  }[prefix] || { pane: "forge", icon: "Code", label: prefix };
  const Ic = Icon[meta.icon];
  return (
    <button
      className="entity-link"
      title={meta.label + " · 点击在右侧打开"}
      onClick={(e) => {
        e.stopPropagation();
        if (!window.Shell) return;
        if (prefix === "cv") window.Shell.openConv(id);
        else window.Shell.openEntity?.(meta.pane, id);
      }}
    >
      <Ic className="icon" />{id}
    </button>
  );
}

// ── Tool call block (with result + progress + nested) ─────────────────────
function ToolCallBlock({ block, defaultOpen }) {
  const [open, setOpen] = useState(!!defaultOpen);
  const isStreaming = block.status === "streaming";
  const isError = block.status === "error";
  const isSuccess = block.status === "completed";
  const ToolIcon = toolIcon(block.attrs?.tool);

  const result = (block.children || []).find(c => c.type === "tool_result");
  const progresses = (block.children || []).filter(c => c.type === "progress");

  return (
    <div className={[
      "blk-tool",
      isStreaming && "is-streaming",
      isError && "is-error",
      isSuccess && "is-success",
      open && "is-open",
    ].filter(Boolean).join(" ")}>
      <div className="blk-tool-head" onClick={() => setOpen(o => !o)}>
        <div className="blk-tool-icon">
          {isStreaming ? <span className="spinner" /> : <ToolIcon />}
        </div>
        <div className="blk-tool-meta">
          <div className="blk-tool-name">
            <code>{block.attrs?.tool || "tool"}</code>
            {block.attrs?.executionGroup && (
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--fg-faint)" }}>
                group {block.attrs.executionGroup}
              </span>
            )}
          </div>
          {block.attrs?.summary && (
            <div className="blk-tool-summary">{block.attrs.summary}</div>
          )}
        </div>
        <div className="blk-tool-aside">
          {isStreaming && <span className="badge streaming"><span className="dot" />运行中</span>}
          {isError && <span className="badge error"><span className="dot" />失败</span>}
          {isSuccess && block.durationMs && (
            <span className="blk-tool-timing">{fmtDuration(block.durationMs)}</span>
          )}
          <Icon.ChevronRight className="blk-tool-chevron" />
        </div>
      </div>

      {open && (
        <div className="blk-tool-body">
          <div className="blk-tool-section">
            <div className="blk-tool-section-label">
              <span>Arguments</span>
              <div className="actions">
                <button>copy</button>
                <button>raw</button>
              </div>
            </div>
            <pre className="code-block">{block.content || "(empty)"}{isStreaming && <span className="streaming-caret" />}</pre>
          </div>

          {progresses.map(p => (
            <div key={p.id} className="blk-progress">
              <div className="blk-progress-head">
                <span className="spinner" />
                <span>Progress</span>
                {p.attrs?.stage && <span className="stage">· {p.attrs.stage}</span>}
              </div>
              <div className="blk-progress-line">{p.content}</div>
            </div>
          ))}

          {result && (
            <div className="blk-tool-section">
              <div className={"tool-result" + (result.status === "error" ? " is-error" : "")}>
                <div className="tool-result-head">
                  <span className="status-dot" />
                  <span>{result.status === "error" ? "Result · error" : "Result"}</span>
                </div>
                <div className="tool-result-content">{result.content}</div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── Subagent (nested message) block ───────────────────────────────────────
function SubagentBlock({ block, depth = 0 }) {
  const [open, setOpen] = useState(false);
  const inner = block.children?.[0]; // single nested "message"
  const a = block.attrs || {};
  return (
    <div className={"blk-subagent" + (open ? " is-open" : "")}>
      <div className="blk-subagent-head" onClick={() => setOpen(o => !o)}>
        <div className="blk-subagent-icon"><Icon.Bot /></div>
        <div className="blk-subagent-meta">
          <div className="blk-subagent-title">
            Subagent · <code style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--accent)" }}>{a.agentType}</code>
            <span style={{ marginLeft: 8, color: "var(--fg-muted)", fontWeight: 400 }}>{a.title}</span>
          </div>
          <div className="blk-subagent-sub">
            {inner?.blocks?.length || 0} 步 · {fmtDuration(block.durationMs)} ·
            {" "}
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 10 }}>
              {block.tokens?.in}↓ {block.tokens?.out}↑
            </span>
          </div>
        </div>
        <Icon.ChevronRight className="blk-tool-chevron" />
      </div>
      {open && inner && (
        <div className="blk-subagent-body">
          <BlockList blocks={inner.blocks} depth={depth + 1} defaultOpenTools />
        </div>
      )}
    </div>
  );
}

// ── Block list (used inside message body and inside subagent body) ────────
function BlockList({ blocks, depth = 0, defaultOpenTools = false }) {
  // Group consecutive tool_calls in same execution_group into a batch.
  const groups = [];
  let buf = null;
  for (const b of blocks) {
    const eg = b.type === "tool_call" ? b.attrs?.executionGroup : null;
    if (eg && buf && buf.eg === eg) {
      buf.items.push(b);
    } else {
      buf = { eg: eg || null, items: [b] };
      groups.push(buf);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {groups.map((g, gi) => {
        // Parallel batch
        if (g.eg && g.items.length > 1) {
          return (
            <div className="tool-batch" key={gi} style={{ marginLeft: 22 }}>
              <div className="tool-batch-tag" />
              {g.items.map(b => (
                <ToolCallBlock key={b.id} block={b} defaultOpen={defaultOpenTools} />
              ))}
            </div>
          );
        }
        return g.items.map(b => {
          switch (b.type) {
            case "text":      return <TextBlock key={b.id} block={b} />;
            case "reasoning": return <ReasoningBlock key={b.id} block={b} />;
            case "tool_call": return <ToolCallBlock key={b.id} block={b} defaultOpen={defaultOpenTools} />;
            case "message":   return <SubagentBlock key={b.id} block={b} depth={depth} />;
            default:          return null;
          }
        });
      })}
    </div>
  );
}

window.BlockList = BlockList;
window.fmtDuration = fmtDuration;
