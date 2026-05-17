/* eslint-disable react/prop-types */
// Memory view — cross-conversation long-term facts

const { useState: useMemState } = React;

const KIND_META = {
  user:      { label: "用户偏好",  color: "var(--status-info)" },
  project:   { label: "项目事实",  color: "var(--status-success)" },
  feedback:  { label: "反馈",      color: "var(--status-warn)" },
  reference: { label: "参考",      color: "var(--accent)" },
};

function MemoryRow({ m }) {
  const meta = KIND_META[m.kind] || KIND_META.reference;
  return (
    <div className={"mem-row" + (m.pinned ? " is-pinned" : "")}>
      <button className={"mem-pin" + (m.pinned ? " is-on" : "")} title={m.pinned ? "已 pin" : "pin 到 system prompt"}>
        <Icon.Pin />
      </button>
      <span className="mem-kind" style={{ "--c": meta.color }}>{meta.label}</span>
      <div className="mem-text">{m.text}</div>
      <span className="mem-source cell-mono">ai</span>
      <ActionMenu items={[
        { label: m.pinned ? "取消置顶" : "Pin 到 system prompt", icon: Icon.Pin },
        { label: "编辑", icon: Icon.Edit },
        "divider",
        { label: "删除", icon: Icon.Trash, danger: true },
      ]} />
    </div>
  );
}

function MemoryView() {
  const [tab, setTab] = useMemState("all");
  const list = tab === "all" ? Forgify.memories : Forgify.memories.filter(m => m.kind === tab);
  const pinnedCount = Forgify.memories.filter(m => m.pinned).length;

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Brain /> Memory</div>
          <div className="page-subtitle">跨对话事实 · pinned 进 system prompt</div>
        </div>
        <div className="page-actions">
          <span className="badge muted"><Icon.Pin style={{ width: 11, height: 11 }} /> {pinnedCount} pinned</span>
          <button className="btn btn-sm btn-accent"><Icon.Plus /> 添加</button>
        </div>
      </div>

      <div className="page-tabs">
        {[
          ["all", "全部", Forgify.memories.length],
          ["user", "用户偏好", Forgify.memories.filter(m => m.kind === "user").length],
          ["project", "项目事实", Forgify.memories.filter(m => m.kind === "project").length],
          ["feedback", "反馈", Forgify.memories.filter(m => m.kind === "feedback").length],
          ["reference", "参考", Forgify.memories.filter(m => m.kind === "reference").length],
        ].map(([k, l, c]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l}<span className="count">{c}</span>
          </button>
        ))}
      </div>

      <div className="page-body" style={{ padding: "0" }}>
        <div className="mem-list">
          {list.map(m => <MemoryRow key={m.id} m={m} />)}
        </div>
        <div style={{ padding: "14px 32px", fontSize: 12, color: "var(--fg-faint)", borderTop: "1px solid var(--border-soft)" }}>
          LLM 自管：3 个 system tools <code style={{ fontFamily: "var(--font-mono)" }}>read_memory</code> · <code style={{ fontFamily: "var(--font-mono)" }}>write_memory</code> · <code style={{ fontFamily: "var(--font-mono)" }}>forget_memory</code>。
          Pinning 是用户控件——LLM 看不到 <code style={{ fontFamily: "var(--font-mono)" }}>pinned</code> 字段。
        </div>
      </div>
    </div>
  );
}

window.MemoryView = MemoryView;
