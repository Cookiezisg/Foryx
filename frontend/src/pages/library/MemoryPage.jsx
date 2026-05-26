// MemoryPage — 4 categories (user/feedback/project/reference) tab
// filter, pin/edit/delete actions, edit drawer.
//
// MemoryPage —— 4 类型 tab + pin/edit/delete + 编辑 Drawer。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { useMemories, useUpdateMemory, useDeleteMemory, usePinMemory } from "../../api/library.js";
import { useToastStore } from "@shared/ui/toastStore";

const TABS = [
  ["all", null],
  ["user", "user"],
  ["feedback", "feedback"],
  ["project", "project"],
  ["reference", "reference"],
];

export function MemoryPage() {
  const { t } = useTranslation(["library", "common"]);
  const [tab, setTab] = useState("all");
  const { data: memories = [], isLoading } = useMemories(tab === "all" ? undefined : tab);
  const [editing, setEditing] = useState(null);
  const del = useDeleteMemory();
  const pin = usePinMemory();
  const pushToast = useToastStore((s) => s.pushToast);

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Brain /> Memory</div>
          <div className="page-subtitle">{t("memory.subtitle")}</div>
        </div>
        <div className="page-actions">
          <Button size="sm" variant="accent"
            onClick={() => setEditing({ name: "", description: "", body: "", memType: "user", source: "user" })}
          >
            <Icon.Plus /> {t("memory.newBtn")}
          </Button>
        </div>
      </div>

      <div className="page-tabs">
        {TABS.map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l ?? t("memory.tabAll")}
          </button>
        ))}
      </div>

      <div className="page-body" style={{ padding: 24 }}>
        {isLoading ? <div className="empty"><div className="sub">{t("common:loading")}</div></div>
          : memories.length === 0 ? (
            <div className="empty">
              <Icon.Brain className="icon" />
              <div className="title">{tab === "all" ? t("memory.emptyAll") : t("memory.emptyTyped", { type: tab })}</div>
              <div className="sub">{t("memory.emptySub")}</div>
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {memories.map((m) => (
                <div key={m.name} className="card" style={{ flexDirection: "row", alignItems: "flex-start", gap: 12 }} onClick={() => setEditing(m)}>
                  {m.pinned && <Icon.Pin style={{ width: 14, height: 14, color: "var(--accent)", marginTop: 4 }} />}
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="card-title" style={{ fontFamily: "var(--font-mono)", fontSize: 13 }}>{m.name}</div>
                    <div className="card-desc" style={{ marginTop: 4 }}>{m.description}</div>
                  </div>
                  <Badge kind="muted">{m.memType}</Badge>
                  <button className="icon-btn" onClick={(e) => {
                    e.stopPropagation();
                    pin.mutate({ name: m.name, pinned: !m.pinned }, {
                      onSuccess: () => pushToast({ kind: "success", title: m.pinned ? t("memory.unpinSuccess") : t("memory.pinSuccess") }),
                    });
                  }}>
                    <Icon.Pin />
                  </button>
                  <button className="icon-btn" onClick={(e) => {
                    e.stopPropagation();
                    if (confirm(t("memory.deleteConfirm", { name: m.name }))) del.mutate(m.name);
                  }}>
                    <Icon.Trash />
                  </button>
                </div>
              ))}
            </div>
          )}
      </div>

      {editing && <MemoryDrawer memory={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

function MemoryDrawer({ memory, onClose }) {
  const { t } = useTranslation(["library", "common"]);
  const [body, setBody] = useState(memory.body || "");
  const [description, setDescription] = useState(memory.description || "");
  const update = useUpdateMemory();
  const pushToast = useToastStore((s) => s.pushToast);
  const isNew = !memory.name;
  const [name, setName] = useState(memory.name || "");

  const submit = () => {
    update.mutate({
      name: name || "untitled-" + Date.now(),
      body: { description, body, memType: memory.memType, source: memory.source || "user" },
    }, {
      onSuccess: () => { pushToast({ kind: "success", title: t("memory.saveSuccess") }); onClose(); },
      onError: (e) => pushToast({ kind: "error", title: t("memory.saveFail"), desc: e.message }),
    });
  };

  return (
    <div className="drawer-wrap is-open">
      <div className="drawer-scrim" onClick={onClose} />
      <div className="drawer" style={{ width: 560 }}>
        <div className="drawer-head">
          <div className="drawer-title">{isNew ? t("memory.drawerNew") : memory.name}</div>
          <button className="icon-btn" onClick={onClose}><Icon.X /></button>
        </div>
        <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
          {isNew && (
            <div>
              <div className="cfg-label">name</div>
              <input className="cfg-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="kebab-case-slug" />
            </div>
          )}
          <div>
            <div className="cfg-label">description</div>
            <input className="cfg-input" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div>
            <div className="cfg-label">body</div>
            <textarea
              className="cfg-input"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              style={{ minHeight: 240, fontFamily: "var(--font-sans)", padding: 8 }}
            />
          </div>
          <div style={{ display: "flex", gap: 8, justifyContent: "flex-end", paddingTop: 8 }}>
            <Button size="sm" variant="ghost" onClick={onClose}>{t("common:cancel")}</Button>
            <Button size="sm" variant="accent" onClick={submit}>{t("common:save")}</Button>
          </div>
        </div>
      </div>
    </div>
  );
}
