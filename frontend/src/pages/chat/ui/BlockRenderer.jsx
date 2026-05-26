// BlockRenderer — recursive renderer for the 7 eventlog block types.
//   text · reasoning · tool_call · tool_result · progress · message ·
//   compaction
//
// The renderer pulls blocks out of the chat store by id, so per-block
// re-renders stay localised — a TextBlock for one message doesn't
// re-render when an unrelated tool_call gets a delta.
//
// 7 个 block type 的递归渲染；从 chat store 按 id 拿 block，单 block 改动
// 不连累兄弟组件。

import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useChatStore, selectBlock, selectChildIds } from "../../../store/chat.js";
import { Icon } from "../../../components/primitives/Icon.jsx";
import { EntityLink } from "../../../widgets/entity-link/EntityLink.jsx";
import { MarkdownView } from "../../../shared/ui/MarkdownView.jsx";

function fmtDuration(ms) {
  if (ms == null) return "";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const m = Math.floor(ms / 60_000);
  const s = Math.round((ms % 60_000) / 1000);
  return `${m}m ${s}s`;
}

const TOOL_ICON = {
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
function toolIcon(name) { return TOOL_ICON[name] || Icon.Wrench; }

// ── inline renderer for text content ─────────────────────────────────
const ENTITY_RE = /\b(?:fn|hd|wf|sk|mcp|mem|cv|fr|doc)_[a-z0-9]{2,32}\b/g;
const INLINE_RE = /(\b(?:fn|hd|wf|sk|mcp|mem|cv|fr|doc)_[a-z0-9]{2,32}\b|\*\*[^*]+\*\*|`[^`]+`)/g;

function renderInline(s) {
  const out = [];
  let last = 0;
  let m;
  while ((m = INLINE_RE.exec(s)) !== null) {
    if (m.index > last) out.push(s.slice(last, m.index));
    const tok = m[0];
    if (tok.startsWith("**")) {
      out.push(<strong key={out.length}>{tok.slice(2, -2)}</strong>);
    } else if (tok.startsWith("`")) {
      const inner = tok.slice(1, -1);
      if (ENTITY_RE.test(inner)) {
        ENTITY_RE.lastIndex = 0; // reset due to global flag
        out.push(<EntityLink key={out.length} id={inner} />);
      } else {
        out.push(<code key={out.length}>{inner}</code>);
      }
    } else {
      out.push(<EntityLink key={out.length} id={tok} />);
    }
    last = m.index + tok.length;
  }
  if (last < s.length) out.push(s.slice(last));
  return out;
}

function renderTextLines(text) {
  const lines = text.split("\n");
  const out = [];
  let bullets = null;
  const flush = () => {
    if (bullets) { out.push(<ul key={`u${out.length}`}>{bullets}</ul>); bullets = null; }
  };
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
  return out;
}

// ── TextBlock ────────────────────────────────────────────────────────
// Renders Markdown so AI replies actually look like Markdown (headings,
// bullets, code blocks, bold/italic, links). Stream-safe — partial
// markdown re-parses on each delta; `streaming` propagates so
// HighlightedCode skips expensive autodetect mid-stream.
const TextBlock = memo(function TextBlock({ convId, blockId }) {
  const block = useChatStore((s) => selectBlock(convId, blockId, s));
  if (!block) return null;
  const isStreaming = block.status === "streaming";
  return (
    <div className="blk-text">
      <MarkdownView source={block.content} streaming={isStreaming} />
      {isStreaming && <span className="streaming-caret" />}
    </div>
  );
});

// ── ReasoningBlock (collapsible) ────────────────────────────────────
const ReasoningBlock = memo(function ReasoningBlock({ convId, blockId, defaultOpen }) {
  const { t } = useTranslation("conv");
  const block = useChatStore((s) => selectBlock(convId, blockId, s));
  const [open, setOpen] = useState(!!defaultOpen);
  if (!block) return null;
  return (
    <div className={"blk-reasoning" + (open ? " is-open" : "")}>
      <button className="blk-reasoning-head" onClick={() => setOpen((o) => !o)}>
        <Icon.ChevronRight />
        <Icon.Brain style={{ width: 12, height: 12 }} />
        <span>{t("block.reasoningLabel", { duration: fmtDuration(block.durationMs) })}</span>
        {block.status === "streaming" && (
          <span className="dot-pulse"><span /><span /><span /></span>
        )}
        <span className="meta">{block.content.length} chars</span>
      </button>
      {open && <div className="blk-reasoning-body">{block.content}</div>}
    </div>
  );
});

// ── ToolCallBlock (collapsible) ─────────────────────────────────────
// CRITICAL perf rule: this component must NOT subscribe to the whole
// `blocks` Map. Earlier version did, which fan-out re-rendered every
// tool_call in the conversation on every SSE delta (the Map ref is
// new each delta even though only one block actually changed). We
// render children via per-id <ToolChildBlock> dispatchers — each one
// only re-renders when its own block changes.
//
// CRITICAL：禁止订阅整张 blocksMap；只能各 child 自订 selectBlock。
const ToolCallBlock = memo(function ToolCallBlock({ convId, blockId, defaultOpen }) {
  const { t } = useTranslation(["conv", "common"]);
  const block = useChatStore((s) => selectBlock(convId, blockId, s));
  const childIds = useChatStore((s) => selectChildIds(convId, blockId, s));
  const [open, setOpen] = useState(!!defaultOpen);
  if (!block) return null;

  const isStreaming = block.status === "streaming";
  const isError = block.status === "error";
  const isSuccess = block.status === "completed";
  const ToolIcon = toolIcon(block.attrs?.tool);

  const cls = [
    "blk-tool",
    isStreaming && "is-streaming",
    isError && "is-error",
    isSuccess && "is-success",
    open && "is-open",
  ].filter(Boolean).join(" ");

  const copyArgs = (e) => {
    e.stopPropagation();
    navigator.clipboard?.writeText(block.content || "").catch(() => {});
  };

  return (
    <div className={cls}>
      <div className="blk-tool-head" onClick={() => setOpen((o) => !o)}>
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
          {isStreaming && <span className="badge streaming"><span className="dot" />{t("block.toolRunning")}</span>}
          {isError && <span className="badge error"><span className="dot" />{t("block.toolFailed")}</span>}
          {isSuccess && block.durationMs != null && (
            <span className="blk-tool-timing">{fmtDuration(block.durationMs)}</span>
          )}
          <Icon.ChevronRight className="blk-tool-chevron" />
        </div>
      </div>

      {open && (
        <div className="blk-tool-body">
          <div className="blk-tool-section">
            <div className="blk-tool-section-label">
              <span>{t("block.toolParamsLabel")}</span>
              <div className="actions">
                <button onClick={copyArgs}>{t("common:copy")}</button>
              </div>
            </div>
            <pre className="code-block">
              {block.content || "(empty)"}
              {isStreaming && <span className="streaming-caret" />}
            </pre>
          </div>

          {childIds.map((id) => (
            <ToolChildBlock key={id} convId={convId} blockId={id} />
          ))}
        </div>
      )}
    </div>
  );
});

// ── ToolChildBlock (per-id dispatcher) ──────────────────────────────
// Subscribes ONLY to its own block's type — type never changes once
// set, so this hook returns a stable string and re-renders only when
// the block first appears or is removed. Children components do their
// own narrow subscriptions for the content/status they care about.
const ToolChildBlock = memo(function ToolChildBlock({ convId, blockId }) {
  const type = useChatStore((s) => s.convs[convId]?.blocks.get(blockId)?.type);
  switch (type) {
    case "progress":    return <ProgressBlock convId={convId} blockId={blockId} />;
    case "message":     return <SubagentBlock convId={convId} blockId={blockId} />;
    case "tool_result": return <ToolResultBlock convId={convId} blockId={blockId} />;
    default:            return null;
  }
});

// ── ProgressBlock ───────────────────────────────────────────────────
const ProgressBlock = memo(function ProgressBlock({ convId, blockId }) {
  const { t } = useTranslation("conv");
  const p = useChatStore((s) => selectBlock(convId, blockId, s));
  if (!p) return null;
  const pStreaming = p.status === "streaming";
  const pError = p.status === "error";
  return (
    <div className={"blk-progress" + (pStreaming ? " is-streaming" : pError ? " is-error" : " is-done")}>
      <div className="blk-progress-head">
        {pStreaming
          ? <span className="spinner" />
          : pError
            ? <Icon.AlertCircle style={{ width: 12, height: 12, color: "var(--status-error)" }} />
            : <Icon.Check style={{ width: 12, height: 12, color: "var(--status-success)" }} />}
        <span>{t("block.progressLabel")}</span>
        {p.attrs?.stage && <span className="stage">· {p.attrs.stage}</span>}
      </div>
      <div className="blk-progress-line">{p.content}</div>
    </div>
  );
});

// ── ToolResultBlock ────────────────────────────────────────────────
const ToolResultBlock = memo(function ToolResultBlock({ convId, blockId }) {
  const { t } = useTranslation("conv");
  const block = useChatStore((s) => selectBlock(convId, blockId, s));
  if (!block) return null;
  const isErr = block.status === "error";
  return (
    <div className="blk-tool-section">
      <div className={"tool-result" + (isErr ? " is-error" : "")}>
        <div className="tool-result-head">
          <span className="status-dot" />
          <span>{isErr ? t("block.resultError") : t("block.resultLabel")}</span>
        </div>
        <div className="tool-result-content">{block.content}</div>
      </div>
    </div>
  );
});

// ── SubagentBlock (nested message via message-type block) ───────────
const SubagentBlock = memo(function SubagentBlock({ convId, blockId, depth = 0 }) {
  const { t } = useTranslation("conv");
  const block = useChatStore((s) => selectBlock(convId, blockId, s));
  const messages = useChatStore((s) => s.convs[convId]?.messages);
  const [open, setOpen] = useState(false);
  if (!block) return null;

  const innerMsgId = block.attrs?.messageId;
  const inner = innerMsgId && messages ? messages.get(innerMsgId) : null;
  const a = block.attrs || {};

  return (
    <div className={"blk-subagent" + (open ? " is-open" : "")}>
      <div className="blk-subagent-head" onClick={() => setOpen((o) => !o)}>
        <div className="blk-subagent-icon"><Icon.Bot /></div>
        <div className="blk-subagent-meta">
          <div className="blk-subagent-title">
            {t("block.subagentTitle")}
            {a.agentType && (
              <>
                {" · "}
                <code style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--accent)" }}>{a.agentType}</code>
              </>
            )}
            {a.title && <span style={{ marginLeft: 8, color: "var(--fg-muted)", fontWeight: 400 }}>{a.title}</span>}
          </div>
          <div className="blk-subagent-sub">
            {t("block.subagentSteps", { count: inner?.blocks?.length || 0 })}
            {block.durationMs != null && <> · {fmtDuration(block.durationMs)}</>}
          </div>
        </div>
        <Icon.ChevronRight className="blk-tool-chevron" />
      </div>
      {open && inner && (
        <div className="blk-subagent-body">
          <BlockList convId={convId} blockIds={inner.blocks} depth={depth + 1} defaultOpenTools />
        </div>
      )}
    </div>
  );
});

// ── CompactionBlock ────────────────────────────────────────────────
const CompactionBlock = memo(function CompactionBlock({ convId, blockId }) {
  const { t } = useTranslation("conv");
  const block = useChatStore((s) => selectBlock(convId, blockId, s));
  const [open, setOpen] = useState(false);
  if (!block) return null;
  const a = block.attrs || {};
  return (
    <div className="blk-compaction">
      <div className="blk-compaction-head" onClick={() => setOpen((o) => !o)}>
        <Icon.Archive style={{ width: 12, height: 12 }} />
        <span>{t("block.compactionLabel")}</span>
        <span className="blk-compaction-sub">
          {a.blocksArchived != null && t("block.compactionBlocks", { count: a.blocksArchived })}
          {a.blocksArchived != null && a.generatedBy && " · "}
          {a.generatedBy && t("block.compactionBy", { name: a.generatedBy })}
        </span>
        <Icon.ChevronRight className="blk-tool-chevron" style={{ marginLeft: "auto", transform: open ? "rotate(90deg)" : "" }} />
      </div>
      {open && <div className="blk-compaction-body">{block.content}</div>}
    </div>
  );
});

// ── BlockList (groups by execution_group for parallel batches) ─────
export function BlockList({ convId, blockIds, depth = 0, defaultOpenTools = false }) {
  const blocksMap = useChatStore((s) => s.convs[convId]?.blocks);
  if (!blocksMap || !blockIds) return null;

  // Group consecutive tool_calls in the same execution_group into a batch
  const groups = [];
  let buf = null;
  for (const id of blockIds) {
    const b = blocksMap.get(id);
    if (!b) continue;
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
        if (g.eg && g.items.length > 1) {
          return (
            <div className="tool-batch" key={gi} style={{ marginLeft: 22 }}>
              <div className="tool-batch-tag" />
              {g.items.map((b) => (
                <ToolCallBlock key={b.id} convId={convId} blockId={b.id} defaultOpen={defaultOpenTools} />
              ))}
            </div>
          );
        }
        return g.items.map((b) => {
          switch (b.type) {
            case "text":       return <TextBlock key={b.id} convId={convId} blockId={b.id} />;
            case "reasoning":  return <ReasoningBlock key={b.id} convId={convId} blockId={b.id} />;
            case "tool_call":  return <ToolCallBlock key={b.id} convId={convId} blockId={b.id} defaultOpen={defaultOpenTools} />;
            case "message":    return <SubagentBlock key={b.id} convId={convId} blockId={b.id} depth={depth} />;
            case "compaction": return <CompactionBlock key={b.id} convId={convId} blockId={b.id} />;
            default:           return null;
          }
        });
      })}
    </div>
  );
}

export { fmtDuration };
