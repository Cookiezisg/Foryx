/* eslint-disable react/prop-types */
// App shell — multi-pane workspace + collapse + resize + ⌘shortcuts + agent sticky + settings popover

const { useState: useAppState, useEffect: useAppEffect, useRef: useAppRef } = React;

// ── Pane chrome (per-pane bar with close) ───────────────────────────────
const PANE_META = {
  chat:      { icon: "MessageSquare", label: "对话" },
  forge:     { icon: "Hammer",        label: "锻造" },
  execute:   { icon: "Play",          label: "执行" },
  documents: { icon: "FileText",      label: "文档" },
  skills:    { icon: "Sparkles",      label: "Skills" },
  mcp:       { icon: "Server",        label: "MCP" },
  memory:    { icon: "Brain",         label: "Memory" },
  observe:   { icon: "Activity",      label: "洞察" },
  config:    { icon: "Settings",      label: "设置" },
};

function Pane({ kind, onClose, crumbs, children }) {
  const meta = PANE_META[kind];
  const I = Icon[meta.icon];
  if (kind === "chat") {
    // chat has its own header — skip the pane-bar to save vertical space
    return (
      <div className="pane" data-kind={kind}>
        <div className="pane-body">{children}</div>
      </div>
    );
  }
  return (
    <div className="pane" data-kind={kind}>
      <div className="pane-bar">
        <div className="pane-crumbs">
          <I className="icon" />
          <span className={crumbs.length === 1 ? "cur" : ""}>{crumbs[0]}</span>
          {crumbs.slice(1).map((c, i) => (
            <React.Fragment key={i}>
              <Icon.ChevronRight className="sep" />
              <span className={i === crumbs.length - 2 ? "cur" : ""}>{c}</span>
            </React.Fragment>
          ))}
        </div>
        <div className="pane-actions">
          <button className="icon-btn" title="更多"><Icon.MoreHorizontal /></button>
          <button className="icon-btn" title="关闭" onClick={onClose}><Icon.X /></button>
        </div>
      </div>
      <div className="pane-body">{children}</div>
    </div>
  );
}

// ── ToastTray ───────────────────────────────────────────────────────────
function ToastTray({ toasts, dismiss }) {
  if (!toasts || toasts.length === 0) return null;
  return (
    <div className="toast-tray">
      {toasts.map(t => (
        <div key={t.id} className={"toast" + (t.kind ? " is-" + t.kind : "")}>
          <div className="toast-icon">
            {t.kind === "error" ? <Icon.AlertCircle />
             : t.kind === "warn" ? <Icon.AlertCircle />
             : <Icon.CheckCircle />}
          </div>
          <div className="toast-body">
            {t.title && <div className="toast-title">{t.title}</div>}
            <div className="toast-desc">{t.desc}</div>
          </div>
          {t.undo && (
            <button className="btn btn-xs btn-ghost" onClick={() => { t.undo(); dismiss(t.id); }}>
              <Icon.Refresh /> 撤销
            </button>
          )}
          <button className="icon-btn" onClick={() => dismiss(t.id)}><Icon.X /></button>
        </div>
      ))}
    </div>
  );
}
window.ToastTray = ToastTray;

// ── Pane resize divider ─────────────────────────────────────────────────
function PaneResize({ onDrag }) {
  const [dragging, setDragging] = useAppState(false);
  useAppEffect(() => {
    if (!dragging) return;
    const onMove = (e) => onDrag(e.clientX);
    const onUp = () => setDragging(false);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [dragging]);
  return <div className={"pane-resize" + (dragging ? " is-dragging" : "")} onMouseDown={() => setDragging(true)} />;
}

// ── Settings popover (retired Tweaks) ────────────────────────────────────
function SettingsPopover({ tweaks, setTweak, onClose, openConfigPane }) {
  const [accountUiMode, setAccountUiMode] = useAppState("view");
  const [newName, setNewName] = useAppState("");
  const active = (tweaks.accounts || []).find(a => a.id === tweaks.activeAccount) || (tweaks.accounts || [])[0];
  const addAccount = () => {
    if (!newName.trim()) return;
    const id = newName.toLowerCase().replace(/\s+/g, "_").slice(0, 16) + "_" + Math.random().toString(36).slice(2, 5);
    const palette = ["#d97757", "#2383e2", "#0f7b6c", "#6940a5", "#37352f"];
    const color = palette[(tweaks.accounts?.length || 0) % palette.length];
    setTweak("accounts", [...(tweaks.accounts || []), { id, name: newName.trim(), color }]);
    setTweak("activeAccount", id);
    setNewName("");
    setAccountUiMode("view");
  };
  return (
    <div className="settings-pop" onClick={e => e.stopPropagation()}>
      <div className="settings-pop-account">
        <div className="settings-pop-account-head">
          <div className="settings-pop-account-avatar" style={{ background: active?.color }}>
            {active?.name?.slice(0, 1).toUpperCase() || "?"}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-strong)" }}>{active?.name || "无账号"}</div>
            <div style={{ fontSize: 10, color: "var(--fg-faint)" }}>本地账号 · {(tweaks.accounts || []).length} 个</div>
          </div>
          <button className="btn btn-xs btn-ghost" onClick={() => setAccountUiMode(m => m === "switch" ? "view" : "switch")}>切换</button>
        </div>
        {accountUiMode === "switch" && (
          <div className="settings-pop-account-list">
            {(tweaks.accounts || []).map(a => (
              <button key={a.id} className={"settings-pop-account-row" + (a.id === tweaks.activeAccount ? " is-active" : "")}
                      onClick={() => { setTweak("activeAccount", a.id); setAccountUiMode("view"); }}>
                <span className="settings-pop-account-avatar small" style={{ background: a.color }}>{a.name.slice(0, 1).toUpperCase()}</span>
                <span style={{ flex: 1 }}>{a.name}</span>
                {a.id === tweaks.activeAccount && <Icon.Check />}
              </button>
            ))}
            <div className="settings-pop-account-add">
              <input className="cfg-input" placeholder="新账号名…" value={newName}
                     onChange={e => setNewName(e.target.value)}
                     onKeyDown={e => { if (e.key === "Enter") addAccount(); }} />
              <button className="btn btn-xs btn-accent" onClick={addAccount}><Icon.Plus /> 添加</button>
            </div>
          </div>
        )}
      </div>

      <div className="settings-pop-row">
        <span>主题</span>
        <div style={{ display: "flex", gap: 4 }}>
          {[["system", "系统"], ["light", "明"], ["dark", "暗"]].map(([v, l]) => (
            <button key={v} className={"btn btn-xs" + (tweaks.theme === v ? " btn-primary" : " btn-ghost")} onClick={() => setTweak("theme", v)}>{l}</button>
          ))}
        </div>
      </div>
      <div className="settings-pop-row">
        <span>Accent</span>
        <div className="settings-pop-swatches">
          {[["claude", "#d97757"], ["blue", "#2383e2"], ["ink", "#37352f"], ["green", "#0f7b6c"], ["purple", "#6940a5"]].map(([k, c]) => (
            <button key={k} className={"settings-pop-swatch" + (tweaks.accent === c || tweaks.accent === k ? " is-active" : "")}
                    style={{ background: c }} onClick={() => setTweak("accent", c)} />
          ))}
        </div>
      </div>
      <div className="settings-pop-row">
        <span>密度</span>
        <div style={{ display: "flex", gap: 4 }}>
          {[["compact", "紧凑"], ["cozy", "适中"], ["comfortable", "舒展"]].map(([v, l]) => (
            <button key={v} className={"btn btn-xs" + (tweaks.density === v ? " btn-primary" : " btn-ghost")} onClick={() => setTweak("density", v)}>{l}</button>
          ))}
        </div>
      </div>
      <div className="settings-pop-row">
        <span>语言</span>
        <div style={{ display: "flex", gap: 4 }}>
          {[["zh", "中文"], ["en", "English"]].map(([v, l]) => (
            <button key={v} className={"btn btn-xs" + (tweaks.lang === v ? " btn-primary" : " btn-ghost")} onClick={() => setTweak("lang", v)}>{l}</button>
          ))}
        </div>
      </div>
      <div style={{ borderTop: "1px solid var(--border-soft)", paddingTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
        <button className="settings-pop-link" onClick={() => { onClose(); openConfigPane?.(); }}>
          <Icon.KeyRound /> API Keys / Model / Sandbox…
        </button>
      </div>
    </div>
  );
}

// ── Default tweak values ────────────────────────────────────────────────
const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "system",
  "accent": "claude",
  "density": "cozy",
  "reasoningDefault": "collapsed",
  "showApprovalBanner": true,
  "showOnboarding": false,
  "hasApiKey": true,
  "lang": "zh",
  "activeAccount": "sun",
  "accounts": [
    { "id": "sun",   "name": "Sun",   "color": "#d97757" },
    { "id": "alice", "name": "Alice", "color": "#2383e2" }
  ]
}/*EDITMODE-END*/;

const HEX_TO_ACCENT = {
  "#d97757": "claude", "#2383e2": "blue", "#37352f": "ink", "#0f7b6c": "green", "#6940a5": "purple",
};
const ACCENT_KEYS = new Set(["claude", "blue", "ink", "green", "purple"]);

function applyTheme(t) {
  const root = document.documentElement;
  const resolved = t.theme === "system"
    ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
    : t.theme;
  root.dataset.theme = resolved;
  root.dataset.accent = HEX_TO_ACCENT[t.accent] || (ACCENT_KEYS.has(t.accent) ? t.accent : "claude");
  root.dataset.density = t.density;
}

// ── App root ────────────────────────────────────────────────────────────
function App() {
  const [tweaks, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [openPanes, setOpenPanes] = useAppState(["chat"]);
  const [activeConv, setActiveConv] = useAppState("cv_a1");
  const [cmdk, setCmdk] = useAppState(false);
  const [notifs, setNotifs] = useAppState(false);
  const [ask, setAsk] = useAppState(false);
  const [streaming, setStreaming] = useAppState(true);
  const [showOnb, setShowOnb] = useAppState(!!tweaks.showOnboarding);
  const [toasts, setToasts] = useAppState([]);
  const [collapsed, setCollapsed] = useAppState(false);
  const [leftPct, setLeftPct] = useAppState(50);
  const [settings, setSettings] = useAppState(false);
  const [narrow, setNarrow] = useAppState(false);
  const [activePane, setActivePane] = useAppState(null);
  const mainRef = useAppRef(null);

  // Detect narrow viewport — drop two-pane mode below ~1000px main width
  useAppEffect(() => {
    if (!mainRef.current) return;
    const check = () => {
      const w = mainRef.current.clientWidth;
      setNarrow(w < 1000);
    };
    check();
    const ro = new ResizeObserver(check);
    ro.observe(mainRef.current);
    return () => ro.disconnect();
  }, []);


  // Shell API
  const [focusEntity, setFocusEntity] = useAppState({}); // { pane: entityId } map
  const togglePane = (k) => {
    setOpenPanes(curr => {
      if (curr.includes(k)) return curr.filter(x => x !== k);
      if (curr.length >= 2) return [curr[1], k];
      return [...curr, k];
    });
  };
  const closePane = (k) => setOpenPanes(curr => curr.filter(x => x !== k));
  const openPaneIfClosed = (k) => setOpenPanes(curr => curr.includes(k) ? curr : (curr.length >= 2 ? [curr[1], k] : [...curr, k]));
  const openEntity = (pane, id) => {
    openPaneIfClosed(pane);
    setFocusEntity(fe => ({ ...fe, [pane]: id }));
  };

  const pushToast = (t) => {
    const id = Math.random().toString(36).slice(2, 8);
    setToasts(ts => [...ts, { ...t, id }]);
    setTimeout(() => setToasts(ts => ts.filter(x => x.id !== id)), t.duration || 5000);
  };

  React.useEffect(() => {
    window.Shell = {
      openPane: openPaneIfClosed,
      closePane,
      togglePane,
      setActiveConv,
      openConv: (id) => { if (id) setActiveConv(id); openPaneIfClosed("chat"); },
      openEntity,
      focusEntity,
      toast: pushToast,
    };
  });

  useAppEffect(() => { applyTheme(tweaks); }, [tweaks]);
  useAppEffect(() => {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const fn = () => applyTheme(tweaks);
    mql.addEventListener?.("change", fn);
    return () => mql.removeEventListener?.("change", fn);
  }, [tweaks]);
  useAppEffect(() => { setShowOnb(!!tweaks.showOnboarding); }, [tweaks.showOnboarding]);

  // Global keyboard shortcuts
  useAppEffect(() => {
    const onKey = (e) => {
      const tag = e.target?.tagName;
      const inField = tag === "INPUT" || tag === "TEXTAREA" || e.target?.isContentEditable;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault(); setCmdk(c => !c); return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "b") {
        e.preventDefault(); setCollapsed(c => !c); return;
      }
      if ((e.metaKey || e.ctrlKey) && /^[1-9]$/.test(e.key)) {
        const idx = parseInt(e.key) - 1;
        const c = Forgify.conversations.filter(x => !x.archived)[idx];
        if (c) { e.preventDefault(); setActiveConv(c.id); openPaneIfClosed("chat"); }
        return;
      }
      if (e.key === "Escape") {
        if (cmdk) setCmdk(false);
        else if (settings) setSettings(false);
        else if (ask) setAsk(false);
        else if (notifs) setNotifs(false);
        else if (streaming) setStreaming(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [streaming, cmdk, ask, notifs, settings]);

  // Pane resize
  const onPaneDrag = (clientX) => {
    if (!mainRef.current) return;
    const r = mainRef.current.getBoundingClientRect();
    const pct = ((clientX - r.left) / r.width) * 100;
    setLeftPct(Math.max(20, Math.min(80, pct)));
  };

  const conv = Forgify.conversations.find(c => c.id === activeConv) || Forgify.conversations[0];
  const crumbsFor = (k) => {
    if (k === "chat") return ["对话", conv.title];
    return [PANE_META[k].label];
  };
  const hasUnread = Forgify.notifications.some(n => n.unread);
  const sseHealth = { eventlog: "ok", notifs: "ok", forge: "ok" };

  const renderPane = (k, idx) => {
    let content = null;
    if (k === "chat") {
      content = !tweaks.hasApiKey ? (
        <NoApiKeyGate openConfig={() => openPaneIfClosed("config")} />
      ) : (
        <ChatView
          conv={conv}
          tweaks={tweaks}
          isStreaming={streaming}
          onSend={() => setStreaming(true)}
          onCancel={() => setStreaming(false)}
          onClose={() => closePane("chat")}
        />
      );
    }
    else if (k === "forge")     content = <ForgeView />;
    else if (k === "execute")   content = <ExecuteView />;
    else if (k === "documents") content = <DocumentsView />;
    else if (k === "skills")    content = <SkillsView />;
    else if (k === "mcp")       content = <McpView />;
    else if (k === "memory")    content = <MemoryView />;
    else if (k === "observe")   content = <ObserveView />;
    else if (k === "config")    content = <ConfigView />;

    const isTwoPane = openPanes.length === 2 && !narrow;
    const visibleIdx = narrow && openPanes.length === 2
      ? (activePane === k ? idx : -1)
      : idx;
    // In narrow mode, hide the non-active pane
    const isHidden = narrow && openPanes.length === 2 && activePane && activePane !== k;
    const style = isTwoPane
      ? (idx === 0 ? { flex: `0 0 calc(${leftPct}% - 2px)` } : { flex: `1 1 auto` })
      : {};
    return (
      <React.Fragment key={k}>
        {!isHidden && (
          <div style={style} className={"pane-wrap" + (idx === 1 ? " is-secondary" : "")}>
            <Pane kind={k} onClose={() => closePane(k)} crumbs={crumbsFor(k)}>
              {content}
            </Pane>
          </div>
        )}
        {isTwoPane && idx === 0 && <PaneResize onDrag={onPaneDrag} />}
      </React.Fragment>
    );
  };

  // Default narrow active to last opened
  useAppEffect(() => {
    if (narrow && openPanes.length === 2 && !activePane) {
      setActivePane(openPanes[openPanes.length - 1]);
    }
    if (openPanes.length < 2) setActivePane(null);
  }, [narrow, openPanes, activePane]);

  // Agent activity (when any chat is streaming and chat pane is not currently visible)
  const agentBusy = streaming && conv ? { conv: conv.title, what: "create_workflow" } : null;

  return (
    <div className={"app" + (collapsed ? " is-collapsed" : "") + (narrow ? " is-narrow" : "")}>
      <Sidebar
        openPanes={openPanes}
        togglePane={togglePane}
        activeConv={activeConv}
        onPickConv={setActiveConv}
        onOpenCmdk={() => setCmdk(true)}
        onOpenNotifs={() => setNotifs(n => !n)}
        onOpenAsk={() => setAsk(true)}
        onOpenSettings={() => setSettings(s => !s)}
        hasUnread={hasUnread}
        askActive={true}
        sseHealth={sseHealth}
        collapsed={collapsed}
        onToggleCollapse={() => setCollapsed(c => !c)}
        agentBusy={agentBusy && !openPanes.includes("chat") ? agentBusy : null}
      />

      <main className="main" ref={mainRef}>
        {openPanes.length === 0 ? (
          <Dashboard
            openPane={togglePane}
            openConv={(id) => { if (id) setActiveConv(id); togglePane("chat"); }}
          />
        ) : (
          openPanes.map((k, i) => renderPane(k, i))
        )}
      </main>

      {tweaks.showApprovalBanner && !openPanes.includes("execute") && (
        <ApprovalBanner onOpen={() => { if (!openPanes.includes("execute")) togglePane("execute"); }} />
      )}

      {narrow && openPanes.length === 2 && (
        <div className="narrow-switch">
          {openPanes.map(k => (
            <button
              key={k}
              className={"narrow-switch-btn" + (activePane === k ? " is-active" : "")}
              onClick={() => setActivePane(k)}
            >
              {PANE_META[k].label}
            </button>
          ))}
        </div>
      )}

      <CommandPalette open={cmdk} onClose={() => setCmdk(false)} onNavigate={(k) => { if (!openPanes.includes(k)) togglePane(k); }} />
      <AskUserModal open={ask} onClose={() => setAsk(false)} onAnswer={() => {}} />
      <NotificationsDrawer open={notifs} onClose={() => setNotifs(false)} />
      <ToastTray toasts={toasts} dismiss={(id) => setToasts(ts => ts.filter(t => t.id !== id))} />
      {showOnb && <Onboarding onDismiss={() => { setShowOnb(false); setTweak("showOnboarding", false); }} />}
      {settings && <div className="overlay" style={{ background: "transparent" }} onClick={() => setSettings(false)}>
        <SettingsPopover tweaks={tweaks} setTweak={setTweak} onClose={() => setSettings(false)} openConfigPane={() => openPaneIfClosed("config")} />
      </div>}
    </div>
  );
}

// ── No API key gate (first-run) ─────────────────────────────────────────
function NoApiKeyGate({ openConfig }) {
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo" style={{ background: "var(--status-warn)" }}><Icon.KeyRound /></div>
        <div>
          <div className="empty-shell-title">先来配一个 API Key</div>
          <div className="empty-shell-sub">
            key 加密存在 <code style={{ fontFamily: "var(--font-mono)" }}>~/.forgify/</code>，不上传。
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="btn btn-sm" onClick={openConfig}>查看 Provider 列表</button>
          <button className="btn btn-sm btn-accent" onClick={openConfig}><Icon.Plus /> 现在去添加</button>
        </div>
      </div>
    </div>
  );
}

const root = ReactDOM.createRoot(document.getElementById("root"));
root.render(<App />);
