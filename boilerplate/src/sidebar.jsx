/* eslint-disable react/prop-types */
// Sidebar — toggle-based navigation + recent conversations sub-list

const { useState: useSideState } = React;

function SideNavItem({ icon: I, label, active, badge, onClick, indent }) {
  return (
    <button
      className={"nav-item" + (active ? " is-active" : "") + (indent ? " is-sub" : "")}
      onClick={onClick}
    >
      {I && <I className="icon" />}
      <span className="label">{label}</span>
      {badge != null && <span className="badge">{badge}</span>}
    </button>
  );
}

function ConvMore({ conv }) {
  const [open, setOpen] = useSideState(false);
  const [pos, setPos] = useSideState(null);
  const btnRef = React.useRef(null);
  const onClick = (e) => {
    e.stopPropagation();
    if (open) { setOpen(false); return; }
    const r = btnRef.current.getBoundingClientRect();
    setPos({ top: r.bottom + 4, left: Math.min(r.left, window.innerWidth - 200) });
    setOpen(true);
  };
  React.useEffect(() => {
    if (!open) return;
    const close = (e) => { if (e.target.closest(".conv-more-menu")) return; setOpen(false); };
    setTimeout(() => window.addEventListener("click", close), 0);
    return () => window.removeEventListener("click", close);
  }, [open]);
  return (
    <div className="conv-more" onClick={(e) => e.stopPropagation()}>
      <button ref={btnRef} className="rel-more-btn" onClick={onClick} title="对话操作"><Icon.MoreHorizontal /></button>
      {open && pos && ReactDOM.createPortal(
        <div className="conv-more-menu" style={{ position: "fixed", top: pos.top, left: pos.left }}>
          <button><Icon.Pin /> {conv.pinned ? "取消置顶" : "置顶"}</button>
          <button><Icon.Edit /> 重命名</button>
          <button><Icon.Folder /> {conv.archived ? "取消归档" : "归档"}</button>
          <button className="is-danger"><Icon.Trash /> 删除</button>
        </div>,
        document.body
      )}
    </div>
  );
}

function ChatListItem({ conv, active, onClick }) {
  const isStreaming = conv.status === "streaming";
  const isApproval = conv.status === "approval";
  return (
    <div className={"nav-item-wrap" + (active ? " is-active" : "")}>
      <button
        className={"nav-item" + (active ? " is-active" : "") + (isStreaming ? " is-streaming" : "")}
        onClick={onClick}
        title={conv.title}
      >
        <span
          className={"dot" + (isStreaming ? " is-streaming" : "")}
          style={isApproval ? { background: "var(--status-warn)" } : null}
        />
        <span className="label">{conv.title}</span>
        {isApproval && (
          <span className="badge" style={{ background: "color-mix(in srgb, var(--status-warn) 16%, transparent)", color: "var(--status-warn)" }}>!</span>
        )}
      </button>
      <ConvMore conv={conv} />
    </div>
  );
}

function Sidebar({ openPanes, togglePane, activeConv, onPickConv, onOpenCmdk, onOpenNotifs, onOpenAsk, onOpenSettings, hasUnread, askActive, sseHealth, collapsed, onToggleCollapse, agentBusy }) {
  const pinned = Forgify.conversations.filter(c => c.pinned && !c.archived);
  const recent = Forgify.conversations.filter(c => !c.pinned && !c.archived);
  const archived = Forgify.conversations.filter(c => c.archived);
  const [showArchived, setShowArchived] = useSideState(false);
  const [showLib, setShowLib] = useSideState(true);
  const [showOther, setShowOther] = useSideState(false);
  const chatQ = "";
  const isOpen = (k) => openPanes.includes(k);

  const matches = (c) => !chatQ || c.title.toLowerCase().includes(chatQ.toLowerCase()) || c.model?.toLowerCase().includes(chatQ.toLowerCase());
  const pinnedF = pinned.filter(matches);
  const recentF = recent.filter(matches);
  const archivedF = archived.filter(matches);

  const running = Forgify.flowruns.filter(f => f.status === "running");
  const waiting = Forgify.flowruns.filter(f => f.status === "waiting_approval");

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="workspace-pill">
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="workspace-name">Forgify</div>
            <div style={{ fontSize: 10, color: "var(--fg-faint)" }}>local · sun@laptop</div>
          </div>
          <Icon.ChevronDown style={{ width: 12, height: 12, color: "var(--fg-faint)", flexShrink: 0 }} />
        </div>

        <button className="cmdk-trigger" onClick={onOpenCmdk}>
          <Icon.Search className="icon" />
          <span className="label">搜索 · 跳转 · 命令</span>
          <kbd>⌘</kbd><kbd>K</kbd>
        </button>
      </div>

      <div className="nav-section">
        <div style={{ height: 4 }} />
        <SideNavItem icon={Icon.MessageSquare} label="对话"   active={isOpen("chat")}     onClick={() => togglePane("chat")} />
        <SideNavItem icon={Icon.Hammer}        label="锻造"   active={isOpen("forge")}    onClick={() => togglePane("forge")} />
        <SideNavItem icon={Icon.Play}          label="执行"   active={isOpen("execute")}  onClick={() => togglePane("execute")} />
        <SideNavItem icon={Icon.FileText}      label="文档"   active={isOpen("documents")} onClick={() => togglePane("documents")} />
        <SideNavItem icon={Icon.Activity}      label="洞察"   active={isOpen("observe")}  onClick={() => togglePane("observe")} />
      </div>

      <div className="nav-section">
        <div className="nav-section-title"><span>资源库</span></div>
        <SideNavItem icon={Icon.Sparkles}      label="Skills" active={isOpen("skills")} onClick={() => togglePane("skills")} />
        <SideNavItem icon={Icon.Server}        label="MCP"    active={isOpen("mcp")}    onClick={() => togglePane("mcp")} />
        <SideNavItem icon={Icon.Brain}         label="Memory" active={isOpen("memory")} onClick={() => togglePane("memory")} />
      </div>

      <div className="nav-section nav-conv-section" style={{ overflowY: "auto", flex: 1, paddingBottom: 12 }}>
        {pinnedF.length > 0 && (
          <div className="nav-section-title"><span>置顶</span></div>
        )}
        {pinnedF.map(c => (
          <ChatListItem
            key={c.id}
            conv={c}
            active={isOpen("chat") && activeConv === c.id}
            onClick={() => { if (!isOpen("chat")) togglePane("chat"); onPickConv(c.id); }}
          />
        ))}

        {recentF.length > 0 && (
          <div className="nav-section-title" style={{ marginTop: pinnedF.length ? 6 : 0 }}>
            <span>最近对话</span>
            <button className="add-btn" title="新对话"><Icon.Plus /></button>
          </div>
        )}
        {recentF.map(c => (
          <ChatListItem
            key={c.id}
            conv={c}
            active={isOpen("chat") && activeConv === c.id}
            onClick={() => { if (!isOpen("chat")) togglePane("chat"); onPickConv(c.id); }}
          />
        ))}

        {archivedF.length > 0 && (
          <>
            <button
              className="nav-section-title nav-section-toggle"
              onClick={() => setShowArchived(s => !s)}
              style={{ marginTop: 10 }}
            >
              <span>归档 · {archivedF.length}</span>
              <Icon.ChevronRight className="chev" style={{ width: 11, height: 11, transform: showArchived ? "rotate(90deg)" : "none", transition: "transform 120ms" }} />
            </button>
            {showArchived && archivedF.map(c => (
              <ChatListItem
                key={c.id}
                conv={c}
                active={isOpen("chat") && activeConv === c.id}
                onClick={() => { if (!isOpen("chat")) togglePane("chat"); onPickConv(c.id); }}
              />
            ))}
          </>
        )}

        {chatQ && pinnedF.length === 0 && recentF.length === 0 && archivedF.length === 0 && (
          <div style={{ padding: "16px 10px", fontSize: 11, color: "var(--fg-faint)", textAlign: "center" }}>
            没有匹配的对话
          </div>
        )}
      </div>

      <div className="sidebar-footer">
        {(running.length > 0 || waiting.length > 0) && (
          <button
            className="sb-running"
            onClick={() => { if (!openPanes.includes("execute")) togglePane("execute"); }}
            title="背景在跑的 workflow"
          >
            {running.length > 0 && <span className="spinner" />}
            <span>
              {running.length > 0 && <>▶ {running.length}</>}
              {running.length > 0 && waiting.length > 0 && " · "}
              {waiting.length > 0 && <>⏸ {waiting.length}</>}
            </span>
          </button>
        )}
        <div className="user-pill">
          <div className="user-avatar">S</div>
          <div className="user-name">Sun</div>
          <span className="user-status" title="后端在线" />
          <div style={{ flex: 1 }} />
          <button className="icon-btn" onClick={onOpenAsk} title="待回答的 agent 问题">
            <Icon.HelpCircle />
            {askActive && <span className="dot" />}
          </button>
          <button className="icon-btn" onClick={onOpenNotifs} title="通知">
            <Icon.Bell />
            {hasUnread && <span className="dot" />}
          </button>
          <button className="icon-btn" onClick={onOpenSettings} title="主题 / 密度 / Accent">
            <Icon.Settings />
          </button>
        </div>
      </div>
    </aside>
  );
}

window.Sidebar = Sidebar;
