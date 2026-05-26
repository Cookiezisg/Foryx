// DocumentsPage — Notion-style tree + WYSIWYG markdown editor.
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
// DocumentsPage —— Notion 风格树 + 所见即所得编辑器。activeDoc/onSetActiveDocument
// 由 AppShell 经 props 传入，pages 层零 app 依赖。

import { useEffect, useMemo, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { ActionMenu } from "../../widgets/action-menu/ActionMenu.jsx";
import { AskAiTrigger } from "../../widgets/ask-ai-trigger/AskAiTrigger.jsx";
import { EntityRelMeta } from "../../widgets/entity-rel-meta/EntityRelMeta.jsx";
import { RelTime } from "../../shared/ui/RelTime.jsx";
import { PaneCollapseToggle } from "../../shared/ui/PaneCollapseToggle.jsx";
import { DocEditor } from "@entities/document";
import {
  useDocumentTree, useDocument,
  useCreateDocument, useUpdateDocument, useDeleteDocument,
} from "../../api/library.js";
import { useToastStore } from "@shared/ui/toastStore";
import { useCollapsible } from "../../hooks/useCollapsible.js";

export function DocumentsPage({ activeDoc, onSetActiveDocument }) {
  const { t } = useTranslation(["library", "common"]);
  const treeQ = useDocumentTree();
  const setActiveDocument = onSetActiveDocument;
  const [openSet, setOpenSet] = useState(new Set());
  const [pendingFocusTitle, setPendingFocusTitle] = useState(null);

  const flat = treeQ.data || [];
  const rooted = useMemo(() => buildTree(flat), [flat]);

  const createDoc = useCreateDocument();
  const pushToast = useToastStore((s) => s.pushToast);

  const onCreateRoot = async () => {
    try {
      const res = await createDoc.mutateAsync({ name: t("documents.untitled"), parentId: null });
      setActiveDocument(res.id);
      setPendingFocusTitle(res.id);
    } catch (e) { pushToast({ kind: "error", title: t("documents.createFail"), desc: e.message }); }
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
      {!sidebarOpen && <PaneCollapseToggle onClick={toggleSidebar} title={t("documents.expandSidebar")} />}
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
  const { t } = useTranslation("library");
  return (
    <div className="empty" style={{ flex: 1 }}>
      <Icon.FileText className="icon" />
      <div className="title">{t("documents.emptyTitle")}</div>
      <div className="sub">{t("documents.emptySub")}</div>
      <Button size="sm" variant="accent" onClick={onCreate} style={{ marginTop: 12 }}>
        <Icon.Plus /> {t("documents.emptyCreate")}
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
  const { t } = useTranslation(["library", "common"]);
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
          <input placeholder={t("documents.searchPlaceholder")} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <button className="icon-btn" title={t("documents.newRootTitle")} onClick={onCreateRoot}>
          <Icon.Plus />
        </button>
        {onCollapse && (
          <button className="icon-btn" title={t("documents.collapseSidebar")} onClick={onCollapse}>
            <Icon.ChevronRight style={{ transform: "rotate(180deg)" }} />
          </button>
        )}
      </div>
      <div className="doc-tree">
        {isLoading && <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>{t("common:loading")}</div>}
        {!isLoading && filtered.length === 0 && (
          <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>
            <Trans i18nKey="documents.noDocs" ns="library"><Icon.Plus style={{ display: "inline", width: 11, height: 11, verticalAlign: "-2px" }} /></Trans>
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
  const { t } = useTranslation(["library", "common"]);
  const hasChildren = node.children?.length > 0;
  const isOpen = openSet.has(node.id);
  const create = useCreateDocument();
  const del = useDeleteDocument();
  const pushToast = useToastStore((s) => s.pushToast);

  const toggle = () => {
    setOpenSet((s) => {
      const next = new Set(s);
      if (next.has(node.id)) next.delete(node.id); else next.add(node.id);
      return next;
    });
  };

  const onNewChild = async () => {
    try {
      const res = await create.mutateAsync({ name: t("documents.untitled"), parentId: node.id });
      setOpenSet((s) => { const n = new Set(s); n.add(node.id); return n; });
      onChildCreated?.(res.id);
    } catch (e) { pushToast({ kind: "error", title: t("documents.createChildFail"), desc: e.message }); }
  };
  const onDelete = () => {
    if (!confirm(t("documents.deleteConfirm", { name: node.name }))) return;
    del.mutate(node.id, {
      onSuccess: () => pushToast({ kind: "success", title: t("documents.deleteSuccess") }),
      onError: (e) => pushToast({ kind: "error", title: t("documents.deleteFail"), desc: e.message }),
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
          title={hasChildren ? (isOpen ? t("documents.collapseTitle") : t("documents.expandTitle")) : undefined}
        >
          <Icon.FileText className="dtr-icon" />
          <Icon.ChevronRight className={"dtr-chev" + (isOpen ? " is-open" : "")} />
        </button>
        <button className="dtr-label" onClick={() => onSelect(node.id)}>
          {node.name || t("documents.untitled")}
        </button>
        <div className="dtr-actions">
          <ActionMenu
            placement="bottom-end"
            renderTrigger={({ ref, ...rest }) => (
              <button ref={ref} className="dtr-act-btn" title={t("documents.actionsTitle")} {...rest}>
                <Icon.MoreHorizontal />
              </button>
            )}
            items={[
              { label: t("common:delete"), icon: Icon.Trash, danger: true, onClick: onDelete },
            ]}
          />
          <button className="dtr-act-btn" title={t("documents.newChildTitle")} onClick={onNewChild}>
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
  const { t } = useTranslation(["library", "common"]);
  const { data: doc, isLoading } = useDocument(docId);
  const update = useUpdateDocument(docId);
  const treeQ = useDocumentTree();
  const pushToast = useToastStore((s) => s.pushToast);

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
        onError: (e) => pushToast({ kind: "error", title: t("documents.saveFail"), desc: e.message }),
      });
    }, 1500);
    return () => clearTimeout(saveTimer.current);
  }, [draftName, draftBody, dirty]);

  if (isLoading || !doc) {
    return <div className="empty" style={{ padding: 48 }}><div className="sub">{t("common:loading")}</div></div>;
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
          placeholder={t("documents.untitled")}
        />
        <span className={"wf-saved is-" + status}>
          {status === "saving" && <><span className="spinner" /> {t("documents.statusSaving")}</>}
          {status === "dirty" && <><span className="dot" /> {t("documents.statusDirty")}</>}
          {status === "clean" && <><span className="dot" /> {t("documents.statusClean")}</>}
        </span>
      </div>

      <div className="doc-page-meta">
        <span><Icon.Clock /> {t("documents.editedLabel")} <RelTime ts={doc.updatedAt} /></span>
        <EntityRelMeta entityId={doc.id} kind="document" />
        <div style={{ flex: 1 }} />
        <AskAiTrigger
          kind="document"
          entityId={doc.id}
          context={t("documents.askAiContext", { name: doc.name || t("documents.untitled") })}
          suggestions={[
            t("documents.aiSuggestExpand"),
            t("documents.aiSuggestTable"),
            t("documents.aiSuggestTranslate"),
            t("documents.aiSuggestSummarize"),
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
