// MemoryPane — 4 categories (user/feedback/project/reference) tab
// filter, pin/edit/delete actions, edit drawer.
//
// MemoryPane —— 4 类型 tab + pin/edit/delete + 编辑 Drawer。

import { useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { useMemories, useUpdateMemory, useDeleteMemory, usePinMemory } from "../../api/library.js";
import { useUIStore } from "../../store/ui.js";

const TABS = [
  ["all", "全部"],
  ["user", "user"],
  ["feedback", "feedback"],
  ["project", "project"],
  ["reference", "reference"],
];

export function MemoryPane() {
  const [tab, setTab] = useState("all");
  const { data: memories = [], isLoading } = useMemories(tab === "all" ? undefined : tab);
  const [editing, setEditing] = useState(null);
  const del = useDeleteMemory();
  const pin = usePinMemory();
  const pushToast = useUIStore((s) => s.pushToast);

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Brain /> Memory</div>
          <div className="page-subtitle">跨对话长期事实库</div>
        </div>
        <div className="page-actions">
          <Button size="sm" variant="accent"
            onClick={() => setEditing({ name: "", description: "", body: "", memType: "user", source: "user" })}
          >
            <Icon.Plus /> 新建
          </Button>
        </div>
      </div>

      <div className="page-tabs">
        {TABS.map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l}
          </button>
        ))}
      </div>

      <div className="page-body" style={{ padding: 24 }}>
        {isLoading ? <div className="empty"><div className="sub">加载中…</div></div>
          : memories.length === 0 ? (
            <div className="empty">
              <Icon.Brain className="icon" />
              <div className="title">{tab === "all" ? "Memory 库还是空的" : `没有 ${tab} 类型的 memory`}</div>
              <div className="sub">在对话里告诉 AI："记住这件事" 即可写入</div>
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
                      onSuccess: () => pushToast({ kind: "success", title: m.pinned ? "已取消 pin" : "已 pin" }),
                    });
                  }}>
                    <Icon.Pin />
                  </button>
                  <button className="icon-btn" onClick={(e) => {
                    e.stopPropagation();
                    if (confirm(`确认删除 ${m.name}?`)) del.mutate(m.name);
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
  const [body, setBody] = useState(memory.body || "");
  const [description, setDescription] = useState(memory.description || "");
  const update = useUpdateMemory();
  const pushToast = useUIStore((s) => s.pushToast);
  const isNew = !memory.name;
  const [name, setName] = useState(memory.name || "");

  const submit = () => {
    update.mutate({
      name: name || "untitled-" + Date.now(),
      body: { description, body, memType: memory.memType, source: memory.source || "user" },
    }, {
      onSuccess: () => { pushToast({ kind: "success", title: "已保存" }); onClose(); },
      onError: (e) => pushToast({ kind: "error", title: "保存失败", desc: e.message }),
    });
  };

  return (
    <div className="drawer-wrap is-open">
      <div className="drawer-scrim" onClick={onClose} />
      <div className="drawer" style={{ width: 560 }}>
        <div className="drawer-head">
          <div className="drawer-title">{isNew ? "新建 Memory" : memory.name}</div>
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
            <Button size="sm" variant="ghost" onClick={onClose}>取消</Button>
            <Button size="sm" variant="accent" onClick={submit}>保存</Button>
          </div>
        </div>
      </div>
    </div>
  );
}
