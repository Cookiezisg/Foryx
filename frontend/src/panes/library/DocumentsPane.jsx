// DocumentsPane — Notion-style tree + WYSIWYG markdown editor.
//
//   - Left: useDocumentTree() flat metadata → recursive tree. Click +
//     creates an "未命名" doc inline (no modal / prompt) and opens it
//     with the title focused.
//   - Right: title <input> (inline rename, blur saves) + Tiptap editor
//     (DocEditor) that round-trips markdown. Type `/` for command panel,
//     `@` for doc reference picker. Markdown shortcuts (#, ##, -, > …)
//     auto-format as you type. No dual-column preview — what you see IS
//     the rendered doc.
//
// DocumentsPane —— Notion 风格树 + 所见即所得编辑器。新建/重命名都内联，
// 不弹 prompt。

import { useEffect, useMemo, useRef, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { ActionMenu } from "../../components/shared/ActionMenu.jsx";
import { AskAiTrigger } from "../../components/shared/AskAiTrigger.jsx";
import { EntityRelMeta } from "../../components/shared/EntityRelMeta.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { PaneCollapseToggle } from "../../components/shared/PaneCollapseToggle.jsx";
import { DocEditor } from "./DocEditor.jsx";
import {
  useDocumentTree, useDocument,
  useCreateDocument, useUpdateDocument, useDeleteDocument,
} from "../../api/library.js";
import { useUIStore } from "../../store/ui.js";
import { useCollapsible } from "../../hooks/useCollapsible.js";

const UNTITLED = "未命名";

export function DocumentsPane() {
  const treeQ = useDocumentTree();
  const setActiveDocument = useUIStore((s) => s.setActiveDocument);
  const activeDoc = useUIStore((s) => s.activeDocument);
  const [openSet, setOpenSet] = useState(new Set());
  const [pendingFocusTitle, setPendingFocusTitle] = useState(null);

  const flat = treeQ.data || [];
  const rooted = useMemo(() => buildTree(flat), [flat]);

  const createDoc = useCreateDocument();
  const pushToast = useUIStore((s) => s.pushToast);

  const onCreateRoot = async () => {
    try {
      const res = await createDoc.mutateAsync({ name: UNTITLED, parentId: null });
      setActiveDocument(res.id);
      setPendingFocusTitle(res.id);
    } catch (e) { pushToast({ kind: "error", title: "创建失败", desc: e.message }); }
  };

  const [sidebarOpen, toggleSidebar] = useCollapsible("documents-sidebar", true);

  const shellClass = "doc-shell pane-collapse-host"
    + (sidebarOpen ? "" : " is-sidebar-collapsed");

  return (
    <div className={shellClass}>
      {sidebarOpen && (
        <DocSidebar
          tree={rooted}
          openSet={openSet}
          setOpenSet={setOpenSet}
          selectedId={activeDoc}
          onSelect={setActiveDocument}
          onCreateRoot={onCreateRoot}
          onChildCreated={(id) => { setActiveDocument(id); setPendingFocusTitle(id); }}
          isLoading={treeQ.isLoading}
          onCollapse={toggleSidebar}
        />
      )}
      {!sidebarOpen && <PaneCollapseToggle onClick={toggleSidebar} title="展开文档树" />}
      <div className="doc-main">
        {activeDoc
          ? <DocPage docId={activeDoc} focusTitle={pendingFocusTitle === activeDoc}
                     onTitleFocused={() => setPendingFocusTitle(null)} />
          : <DocEmpty onCreate={onCreateRoot} />}
      </div>
    </div>
  );
}

function DocEmpty({ onCreate }) {
  return (
    <div className="empty" style={{ flex: 1 }}>
      <Icon.FileText className="icon" />
      <div className="title">还没有打开的文档</div>
      <div className="sub">左侧选一篇 · 或</div>
      <Button size="sm" variant="accent" onClick={onCreate} style={{ marginTop: 12 }}>
        <Icon.Plus /> 新建第一篇
      </Button>
    </div>
  );
}

// ── Tree helpers ─────────────────────────────────────────────────────
function buildTree(flat) {
  const byId = new Map(flat.map((d) => [d.id, { ...d, children: [] }]));
  const roots = [];
  for (const d of byId.values()) {
    if (d.parentId && byId.has(d.parentId)) byId.get(d.parentId).children.push(d);
    else roots.push(d);
  }
  const sortRec = (n) => {
    n.children.sort((a, b) => (a.position - b.position) || a.name.localeCompare(b.name));
    n.children.forEach(sortRec);
  };
  roots.sort((a, b) => (a.position - b.position) || a.name.localeCompare(b.name));
  roots.forEach(sortRec);
  return roots;
}

function DocSidebar({ tree, openSet, setOpenSet, selectedId, onSelect, onCreateRoot, onChildCreated, isLoading, onCollapse }) {
  const [q, setQ] = useState("");
  const filtered = useMemo(() => {
    if (!q.trim()) return tree;
    const ql = q.toLowerCase();
    const walk = (nodes) => nodes
      .map((n) => {
        const kids = n.children?.length ? walk(n.children) : [];
        if (n.name.toLowerCase().includes(ql) || kids.length) return { ...n, children: kids };
        return null;
      })
      .filter(Boolean);
    return walk(tree);
  }, [tree, q]);

  return (
    <aside className="doc-sidebar">
      <div className="doc-sidebar-head">
        <div className="search-input doc-search">
          <Icon.Search className="icon" />
          <input placeholder="搜索文档…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <button className="icon-btn" title="新建顶级页面" onClick={onCreateRoot}>
          <Icon.Plus />
        </button>
        {onCollapse && (
          <button className="icon-btn" title="收起侧栏" onClick={onCollapse}>
            <Icon.ChevronRight style={{ transform: "rotate(180deg)" }} />
          </button>
        )}
      </div>
      <div className="doc-tree">
        {isLoading && <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>加载中…</div>}
        {!isLoading && filtered.length === 0 && (
          <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>
            还没有文档 · 点 <Icon.Plus style={{ display: "inline", width: 11, height: 11, verticalAlign: "-2px" }} /> 新建
          </div>
        )}
        {filtered.map((n) => (
          <DocTreeNode
            key={n.id} node={n} depth={0}
            openSet={openSet} setOpenSet={setOpenSet}
            selectedId={selectedId} onSelect={onSelect}
            onChildCreated={onChildCreated}
          />
        ))}
      </div>
    </aside>
  );
}

function DocTreeNode({ node, depth, openSet, setOpenSet, selectedId, onSelect, onChildCreated }) {
  const hasChildren = node.children?.length > 0;
  const isOpen = openSet.has(node.id);
  const create = useCreateDocument();
  const del = useDeleteDocument();
  const pushToast = useUIStore((s) => s.pushToast);

  const toggle = () => {
    setOpenSet((s) => {
      const next = new Set(s);
      if (next.has(node.id)) next.delete(node.id); else next.add(node.id);
      return next;
    });
  };

  const onNewChild = async () => {
    try {
      const res = await create.mutateAsync({ name: UNTITLED, parentId: node.id });
      setOpenSet((s) => { const n = new Set(s); n.add(node.id); return n; });
      onChildCreated?.(res.id);
    } catch (e) { pushToast({ kind: "error", title: "新建失败", desc: e.message }); }
  };
  const onDelete = () => {
    if (!confirm(`删除 "${node.name}"? 包含子页面也会一起删`)) return;
    del.mutate(node.id, {
      onSuccess: () => pushToast({ kind: "success", title: "已删除" }),
      onError: (e) => pushToast({ kind: "error", title: "删除失败", desc: e.message }),
    });
  };

  const selected = selectedId === node.id;

  return (
    <>
      <div
        className={"doc-tree-row" + (selected ? " is-selected" : "")}
        style={{ paddingLeft: 4 + depth * 14 }}
      >
        <button
          className="dtr-toggle"
          data-has-children={hasChildren || undefined}
          data-open={isOpen || undefined}
          onClick={hasChildren ? toggle : () => onSelect(node.id)}
          title={hasChildren ? (isOpen ? "折叠" : "展开") : undefined}
        >
          <Icon.FileText className="dtr-icon" />
          <Icon.ChevronRight className={"dtr-chev" + (isOpen ? " is-open" : "")} />
        </button>
        <button className="dtr-label" onClick={() => onSelect(node.id)}>
          {node.name || UNTITLED}
        </button>
        <div className="dtr-actions">
          <ActionMenu
            placement="bottom-end"
            renderTrigger={({ ref, ...rest }) => (
              <button ref={ref} className="dtr-act-btn" title="操作" {...rest}>
                <Icon.MoreHorizontal />
              </button>
            )}
            items={[
              { label: "删除", icon: Icon.Trash, danger: true, onClick: onDelete },
            ]}
          />
          <button className="dtr-act-btn" title="新建子页面" onClick={onNewChild}>
            <Icon.Plus />
          </button>
        </div>
      </div>
      {isOpen && node.children.map((c) => (
        <DocTreeNode
          key={c.id} node={c} depth={depth + 1}
          openSet={openSet} setOpenSet={setOpenSet}
          selectedId={selectedId} onSelect={onSelect}
          onChildCreated={onChildCreated}
        />
      ))}
    </>
  );
}

// ── DocPage — title input + Tiptap body ──────────────────────────────
function DocPage({ docId, focusTitle, onTitleFocused }) {
  const { data: doc, isLoading } = useDocument(docId);
  const update = useUpdateDocument(docId);
  const treeQ = useDocumentTree();
  const pushToast = useUIStore((s) => s.pushToast);

  const [draftName, setDraftName] = useState("");
  const [draftBody, setDraftBody] = useState("");
  const [dirty, setDirty] = useState(false);
  const saveTimer = useRef(null);
  const titleRef = useRef(null);
  const editorRef = useRef(null);

  // Sync local draft when doc loads / switches.
  useEffect(() => {
    if (!doc) return;
    setDraftName(doc.name || "");
    setDraftBody(doc.content || "");
    setDirty(false);
  }, [doc?.id]);

  // Auto-focus title when a freshly-created doc opens (so user types
  // straight into the title — Notion behaviour).
  useEffect(() => {
    if (focusTitle && titleRef.current) {
      titleRef.current.focus();
      titleRef.current.select();
      onTitleFocused?.();
    }
  }, [focusTitle, doc?.id, onTitleFocused]);

  // Debounced save on body / title change.
  useEffect(() => {
    if (!doc || !dirty) return;
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      const patch = {};
      const trimmedName = draftName.trim();
      if (trimmedName && trimmedName !== doc.name) patch.name = trimmedName;
      if (draftBody !== doc.content) patch.content = draftBody;
      if (!Object.keys(patch).length) { setDirty(false); return; }
      update.mutate(patch, {
        onSuccess: () => setDirty(false),
        onError: (e) => pushToast({ kind: "error", title: "保存失败", desc: e.message }),
      });
    }, 1500);
    return () => clearTimeout(saveTimer.current);
  }, [draftName, draftBody, dirty]);

  if (isLoading || !doc) {
    return <div className="empty" style={{ padding: 48 }}><div className="sub">加载中…</div></div>;
  }

  const status = update.isPending ? "saving" : dirty ? "dirty" : "clean";

  return (
    <div className="doc-page">
      <div className="doc-page-head">
        <div className="doc-page-icon"><Icon.FileText /></div>
        <input
          ref={titleRef}
          className="doc-page-title-input"
          value={draftName}
          onChange={(e) => { setDraftName(e.target.value); setDirty(true); }}
          onKeyDown={(e) => {
            if (e.key === "Enter") { e.preventDefault(); editorRef.current?.focus(); }
          }}
          placeholder={UNTITLED}
        />
        <span className={"wf-saved is-" + status}>
          {status === "saving" && <><span className="spinner" /> 保存中…</>}
          {status === "dirty" && <><span className="dot" /> 未保存</>}
          {status === "clean" && <><span className="dot" /> 已保存</>}
        </span>
      </div>

      <div className="doc-page-meta">
        <span><Icon.Clock /> 编辑 <RelTime ts={doc.updatedAt} /></span>
        <EntityRelMeta entityId={doc.id} kind="document" />
        <div style={{ flex: 1 }} />
        <AskAiTrigger
          kind="document"
          entityId={doc.id}
          context={`文档 · ${doc.name || UNTITLED}`}
          suggestions={[
            "把这一节扩写到 500 字",
            "把表格转成 bullet list",
            "翻译成英文",
            "提炼 200 字摘要",
          ]}
        />
      </div>

      <div className="doc-page-body">
        <DocEditor
          ref={editorRef}
          initialMarkdown={doc.content || ""}
          onChange={(md) => { setDraftBody(md); setDirty(true); }}
          documentsLookup={() => treeQ.data || []}
        />
      </div>
    </div>
  );
}
