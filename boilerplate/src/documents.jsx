/* eslint-disable react/prop-types */
// Documents view — Notion-like file tree + page body

const { useState: useDocState } = React;

function flattenDocs(nodes, depth = 0, out = []) {
  for (const n of nodes) {
    out.push({ ...n, depth });
    if (n.children) flattenDocs(n.children, depth + 1, out);
  }
  return out;
}

function DocTreeItem({ node, openSet, toggleOpen, selectedId, onSelect }) {
  const isFolder = node.kind === "folder";
  const isOpen = openSet.has(node.id);
  return (
    <div className={"doc-tree-row" + (selectedId === node.id ? " is-selected" : "")}>
      <button
        className={"doc-tree-item" + (selectedId === node.id ? " is-selected" : "")}
        style={{ paddingLeft: 8 + node.depth * 14 }}
        onClick={() => {
          if (isFolder) toggleOpen(node.id);
          else onSelect(node.id);
        }}
      >
        {isFolder ? (
          <Icon.ChevronRight className="chev" style={{ transform: isOpen ? "rotate(90deg)" : "none" }} />
        ) : <span className="chev-placeholder" />}
        <span className="doc-icon">
          {isFolder ? <Icon.Folder /> : <Icon.FileText />}
        </span>
        <span className="doc-label">{node.title}</span>
      </button>
      {!isFolder && (
        <ActionMenu items={[
          { label: "在新 pane 打开", icon: Icon.ArrowRight },
          { label: "重命名", icon: Icon.Edit, shortcut: "F2" },
          { label: "移动到…", icon: Icon.Folder },
          { label: "复制", icon: Icon.Copy, shortcut: "⌘D" },
          "divider",
          { label: "导出 Markdown", icon: Icon.Inbox },
          { label: "归档", icon: Icon.Folder },
          { label: "删除", icon: Icon.Trash, danger: true, shortcut: "⌫" },
        ]} />
      )}
    </div>
  );
}

function DocSidebar({ selectedId, onSelect }) {
  const [openSet, setOpenSet] = useDocState(new Set(["d_root", "d_arch"]));
  const toggleOpen = (id) => setOpenSet(s => {
    const next = new Set(s);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });

  // Custom flatten that respects openSet
  const flat = [];
  const walk = (nodes, depth) => {
    for (const n of nodes) {
      flat.push({ ...n, depth });
      if (n.kind === "folder" && openSet.has(n.id) && n.children) {
        walk(n.children, depth + 1);
      }
    }
  };
  walk(Forgify.documents, 0);

  return (
    <aside className="doc-sidebar">
      <div className="doc-sidebar-head">
        <div className="search-input doc-search">
          <Icon.Search className="icon" />
          <input placeholder="搜索文档…" />
        </div>
      </div>
      <div className="doc-tree">
        {flat.map(n => (
          <DocTreeItem
            key={n.id}
            node={n}
            openSet={openSet}
            toggleOpen={toggleOpen}
            selectedId={selectedId}
            onSelect={onSelect}
          />
        ))}
        <button className="doc-tree-item is-add" style={{ paddingLeft: 8 }}>
          <Icon.Plus className="chev" />
          <span className="doc-label" style={{ color: "var(--fg-muted)" }}>新建页面</span>
        </button>
      </div>
    </aside>
  );
}

// Very small markdown renderer (h1/h2/h3, blockquote, list, pre, table, paragraph)
function MD({ source }) {
  const blocks = [];
  const lines = source.split("\n");
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (/^#{1,3}\s/.test(line)) {
      const m = line.match(/^(#{1,3})\s(.*)/);
      const lvl = m[1].length;
      const text = m[2];
      blocks.push(React.createElement(`h${lvl}`, { key: i }, text));
      i++;
    } else if (/^>\s?/.test(line)) {
      blocks.push(<blockquote key={i}>{line.replace(/^>\s?/, "")}</blockquote>);
      i++;
    } else if (line.startsWith("```")) {
      const code = [];
      i++;
      while (i < lines.length && !lines[i].startsWith("```")) { code.push(lines[i]); i++; }
      i++;
      blocks.push(<pre key={i} className="code-block">{code.join("\n")}</pre>);
    } else if (line.startsWith("- ") || line.startsWith("* ")) {
      const items = [];
      while (i < lines.length && (lines[i].startsWith("- ") || lines[i].startsWith("* "))) {
        items.push(lines[i].slice(2));
        i++;
      }
      blocks.push(<ul key={"ul" + i}>{items.map((t, j) => <li key={j}>{renderInlineMd(t)}</li>)}</ul>);
    } else if (line.startsWith("|")) {
      // table
      const rows = [];
      while (i < lines.length && lines[i].startsWith("|")) {
        rows.push(lines[i]);
        i++;
      }
      const headers = rows[0].split("|").slice(1, -1).map(s => s.trim());
      const dataRows = rows.slice(2).map(r => r.split("|").slice(1, -1).map(s => s.trim()));
      blocks.push(
        <table key={"t" + i} className="md-table">
          <thead><tr>{headers.map((h, j) => <th key={j}>{h}</th>)}</tr></thead>
          <tbody>{dataRows.map((row, k) => <tr key={k}>{row.map((c, j) => <td key={j}>{renderInlineMd(c)}</td>)}</tr>)}</tbody>
        </table>
      );
    } else if (line.trim() === "") {
      i++;
    } else {
      blocks.push(<p key={i}>{renderInlineMd(line)}</p>);
      i++;
    }
  }
  return <div className="md-body">{blocks}</div>;
}

function renderInlineMd(s) {
  const parts = [];
  const re = /(\*\*[^*]+\*\*|`[^`]+`)/g;
  let last = 0;
  s.replace(re, (m, _g, idx) => {
    if (idx > last) parts.push(s.slice(last, idx));
    if (m.startsWith("**")) parts.push(<strong key={idx}>{m.slice(2, -2)}</strong>);
    else parts.push(<code key={idx}>{m.slice(1, -1)}</code>);
    last = idx + m.length;
    return m;
  });
  if (last < s.length) parts.push(s.slice(last));
  return parts;
}

function findDoc(id, nodes = Forgify.documents) {
  for (const n of nodes) {
    if (n.id === id) return n;
    if (n.children) { const r = findDoc(id, n.children); if (r) return r; }
  }
  return null;
}

function DocPage({ doc, onShowBacklinks }) {
  if (!doc) {
    return (
      <div className="empty" style={{ flex: 1 }}>
        <Icon.FileText className="icon" />
        <div className="title">选择一个文档，或新建</div>
        <div className="sub">左侧选一篇</div>
      </div>
    );
  }
  return (
    <div className="doc-page">
      <div className="doc-page-head">
        <div className="doc-page-icon"><Icon.FileText /></div>
        <div className="doc-page-title" contentEditable suppressContentEditableWarning>
          {doc.title}
        </div>
      </div>
      <div className="doc-page-meta">
        <span><Icon.Clock /> 编辑 <RelTime ts={new Date(Date.now() - 12 * 60 * 1000)} /></span>
        <span>·</span>
        <span><Icon.User /> Sun</span>
        <EntityRelMeta entityId={doc.id} />
        <div style={{ flex: 1 }} />
        <div className="page-actions" style={{ display: "flex", gap: 6 }}>
          <button className="btn btn-xs btn-ghost" onClick={onShowBacklinks}><Icon.Layers /> 引用 (3)</button>
          <AskAiTrigger
            size="xs"
            context={"文档 · " + doc.title}
            suggestions={[
              "帮我把这一节扩写到 500 字",
              "把表格转成 bullet list",
              "翻译成英文",
              "提炼一段 200 字摘要",
            ]}
          />
        </div>
      </div>
      <div className="doc-page-body">
        {doc.body ? <MD source={doc.body} /> : (
          <p style={{ color: "var(--fg-faint)" }}>空文档。点击开始书写…</p>
        )}
      </div>
    </div>
  );
}


function DocumentsView() {
  const [selectedId, setSelectedId] = useDocState(window.Shell?.focusEntity?.documents || "d_strava");
  const [showBacklinks, setShowBacklinks] = useDocState(false);
  const [treeOpen, setTreeOpen] = useDocState(false);
  const doc = findDoc(selectedId);

  return (
    <div className={"doc-shell" + (treeOpen ? " is-tree-open" : "")}>
      <DocSidebar selectedId={selectedId} onSelect={(id) => { setSelectedId(id); setTreeOpen(false); }} />
      <button className="pane-side-toggle" title="切换文档树" onClick={() => setTreeOpen(o => !o)}>
        <Icon.Menu />
      </button>
      <div className="doc-main">
        <DocPage doc={doc} onShowBacklinks={() => setShowBacklinks(true)} />
      </div>
      {showBacklinks && doc && (
        <DocBacklinks doc={doc} onClose={() => setShowBacklinks(false)} />
      )}
    </div>
  );
}

function DocBacklinks({ doc, onClose }) {
  // Mock: which forges/workflows/conversations reference this doc
  const refs = [
    { type: "workflow", id: "wf_weekly_training", name: "weekly-training-summary", reason: "在 capability check 阶段读取 schema" },
    { type: "function", id: "fn_aggregate_week", name: "aggregate_week", reason: "代码注释引用本文档作为字段约定" },
    { type: "chat",     id: "cv_a1",             name: "CSV → Notion 同步脚本", reason: "对话中 attach 了这篇文档" },
  ];
  return (
    <aside className="doc-backlinks">
      <div className="doc-backlinks-head">
        <div style={{ fontSize: 12, fontWeight: 600, color: "var(--fg-strong)" }}>
          <Icon.Layers style={{ width: 13, height: 13, marginRight: 4 }} /> 引用本文档 ({refs.length})
        </div>
        <button className="icon-btn" onClick={onClose}><Icon.X /></button>
      </div>
      <div className="doc-backlinks-body">
        {refs.map((r, i) => (
          <button key={i} className="doc-backlink-row" onClick={() => {
            if (r.type === "chat") window.Shell?.openConv(r.id);
            else window.Shell?.openPane(r.type === "workflow" ? "forge" : r.type === "function" ? "forge" : r.type);
          }}>
            <div className="doc-backlink-icon">
              {r.type === "workflow" && <Icon.Workflow />}
              {r.type === "function" && <Icon.Code />}
              {r.type === "chat" && <Icon.MessageSquare />}
            </div>
            <div className="doc-backlink-meta">
              <div className="doc-backlink-name"><code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>{r.id}</code> {r.name}</div>
              <div className="doc-backlink-reason">{r.reason}</div>
            </div>
            <Icon.ChevronRight style={{ width: 12, height: 12, color: "var(--fg-faint)" }} />
          </button>
        ))}
      </div>
    </aside>
  );
}

window.DocumentsView = DocumentsView;
window.MD = MD;
