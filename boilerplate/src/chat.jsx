/* eslint-disable react/prop-types */
// Chat view — header + scrolling thread + composer with slash menu + @mention + drag-drop

const { useState: useChatState, useRef, useEffect } = React;

// ── RelTime util (used everywhere) ──────────────────────────────────────
function RelTime({ ts, prefix = "" }) {
  const d = typeof ts === "string" ? new Date(ts) : ts;
  const diff = (Date.now() - d.getTime()) / 1000;
  let txt;
  if (diff < 5) txt = "刚刚";
  else if (diff < 60) txt = Math.floor(diff) + " 秒前";
  else if (diff < 3600) txt = Math.floor(diff / 60) + " 分钟前";
  else if (diff < 86400) txt = Math.floor(diff / 3600) + " 小时前";
  else if (diff < 86400 * 30) txt = Math.floor(diff / 86400) + " 天前";
  else txt = d.toLocaleDateString("zh-CN", { month: "short", day: "numeric" });
  const iso = d.toLocaleString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" });
  return <time title={iso}>{prefix}{txt}</time>;
}
window.RelTime = RelTime;

function MessageView({ msg, tweaks, isLast }) {
  const isUser = msg.role === "user";
  const time = new Date(msg.createdAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
  const provider = msg.model?.split("-")[0]?.toUpperCase().slice(0, 2) || "AI";

  return (
    <div className={"msg role-" + msg.role} data-comment-anchor={msg.id}>
      <div className="msg-meta">
        <div className="msg-author">
          <div className={"msg-author-avatar " + (isUser ? "user" : "ai")}>
            {isUser ? "Y" : provider}
          </div>
          <span>{isUser ? "你" : msg.model || "Forgify"}</span>
        </div>
        <span>·</span>
        <RelTime ts={msg.createdAt} />
        {!isUser && msg.inputTokens != null && (
          <>
            <span>·</span>
            <span className="msg-tokens">
              <span style={{ color: "var(--fg-muted)" }}>{msg.inputTokens.toLocaleString()}</span>
              <span className="sep"> ↓ ↑ </span>
              <span style={{ color: "var(--fg-muted)" }}>{msg.outputTokens.toLocaleString()}</span>
            </span>
          </>
        )}
        {msg.status === "streaming" && (
          <span className="badge streaming"><span className="dot" />streaming</span>
        )}
        <div style={{ flex: 1 }} />
        <div className="msg-actions">
          <button className="msg-action" title="复制"><Icon.Copy /></button>
          {!isUser && <button className="msg-action" title="重新生成"><Icon.Refresh /></button>}
          {isUser && <button className="msg-action" title="编辑并重发"><Icon.Wrench /></button>}
          <button className="msg-action" title="从这里分叉新对话"><Icon.GitBranch /></button>
          <button className="msg-action" title="更多"><Icon.MoreHorizontal /></button>
        </div>
      </div>
      <div className="msg-body">
        <BlockList blocks={msg.blocks} defaultOpenTools={false} />
        {msg.attachments && msg.attachments.length > 0 && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 12 }}>
            {msg.attachments.map(a => (
              <div className="attached-pill" key={a.id}>
                {a.mimeType.startsWith("image/") ? <Icon.Image className="file-icon" /> : <Icon.File className="file-icon" />}
                <span>{a.fileName}</span>
                <span className="file-meta">{(a.sizeBytes / 1024).toFixed(0)}kb</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function ChatHeader({ title, model, convId, onClose }) {
  return (
    <div className="chat-header">
      <div className="chat-title-row" style={{ flexDirection: "column", alignItems: "flex-start", gap: 2 }}>
        <div className="chat-title-text">{title}</div>
        {convId && (
          <div style={{ fontSize: 11, color: "var(--fg-muted)", display: "flex", alignItems: "center", gap: 4 }}>
            <EntityRelMeta entityId={convId} />
          </div>
        )}
      </div>
      <div className="chat-header-actions">
        <div className="model-tag" title="切换模型">
          <span className="provider">{(model || "AI").slice(0, 2).toUpperCase()}</span>
          <span>{model || "deepseek-chat"}</span>
          <Icon.ChevronDown style={{ width: 10, height: 10, color: "var(--fg-faint)" }} />
        </div>
        <button className="icon-btn" title="附加 Skill / Memory"><Icon.Layers /></button>
        <button className="icon-btn" title="对话历史搜索"><Icon.Search /></button>
        <button className="icon-btn" title="对话设置"><Icon.Settings /></button>
        {onClose && <button className="icon-btn" title="关闭" onClick={onClose}><Icon.X /></button>}
      </div>
    </div>
  );
}

// ── Slash menu items ─────────────────────────────────────────────────────
const SLASH_ITEMS = [
  { kw: "skill",   label: "/skill",   desc: "提示 agent 使用某个 Skill", icon: "Sparkles" },
  { kw: "forge",   label: "/forge",   desc: "把某个 Function/Handler/Workflow 作为上下文", icon: "Hammer" },
  { kw: "file",    label: "/file",    desc: "附加文件",                  icon: "Paperclip" },
  { kw: "run",     label: "/run",     desc: "运行一个 workflow",         icon: "Play" },
  { kw: "doc",     label: "/doc",     desc: "引用一篇文档",              icon: "FileText" },
  { kw: "memory",  label: "/memory",  desc: "写一条 memory",             icon: "Brain" },
  { kw: "clear",   label: "/clear",   desc: "清空当前对话(保留 ID)",     icon: "Trash" },
  { kw: "compact", label: "/compact", desc: "压缩历史",                  icon: "Layers" },
];

function Composer({ disabled, isStreaming, onSend, onCancel }) {
  const [text, setText] = useChatState("");
  const [attached, setAttached] = useChatState([]);
  const [mentions, setMentions] = useChatState([]); // pinned context entities
  const [slash, setSlash] = useChatState(null);     // { items, idx } when "/" triggered
  const [atMenu, setAtMenu] = useChatState(null);   // { items, idx } when "@" triggered
  const [dragging, setDragging] = useChatState(false);
  const ta = useRef(null);

  useEffect(() => {
    if (!ta.current) return;
    ta.current.style.height = "auto";
    ta.current.style.height = Math.min(200, ta.current.scrollHeight) + "px";
  }, [text]);

  const send = () => {
    if (!text.trim() || disabled) return;
    onSend?.(text);
    setText("");
    setAttached([]);
    setMentions([]);
    setSlash(null);
    setAtMenu(null);
  };

  const onChange = (e) => {
    const v = e.target.value;
    setText(v);
    // Slash menu — only when "/" is first char (so user can type "TODO/ok")
    if (v.startsWith("/") && !v.includes(" ")) {
      const q = v.slice(1).toLowerCase();
      const items = SLASH_ITEMS.filter(it => it.kw.startsWith(q));
      setSlash({ items, idx: 0 });
    } else setSlash(null);
    // @ menu — last token starts with @
    const m = v.match(/(?:^|\s)@([^\s]*)$/);
    if (m) {
      const q = m[1].toLowerCase();
      const items = mentionPool().filter(it => it.label.toLowerCase().includes(q)).slice(0, 6);
      setAtMenu({ items, idx: 0, q });
    } else setAtMenu(null);
  };

  const pickSlash = (it) => {
    setText(it.label + " ");
    setSlash(null);
    ta.current?.focus();
  };

  const pickMention = (it) => {
    setMentions(ms => ms.find(x => x.id === it.id) ? ms : [...ms, it]);
    // strip the @query
    setText(t => t.replace(/(?:^|\s)@[^\s]*$/, m => m.startsWith(" ") ? " " : ""));
    setAtMenu(null);
    ta.current?.focus();
  };

  const onKey = (e) => {
    if (slash && slash.items.length > 0) {
      if (e.key === "ArrowDown") { e.preventDefault(); setSlash(s => ({ ...s, idx: Math.min(s.idx + 1, s.items.length - 1) })); return; }
      if (e.key === "ArrowUp")   { e.preventDefault(); setSlash(s => ({ ...s, idx: Math.max(s.idx - 1, 0) })); return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pickSlash(slash.items[slash.idx]); return; }
      if (e.key === "Escape") { setSlash(null); return; }
    }
    if (atMenu && atMenu.items.length > 0) {
      if (e.key === "ArrowDown") { e.preventDefault(); setAtMenu(s => ({ ...s, idx: Math.min(s.idx + 1, s.items.length - 1) })); return; }
      if (e.key === "ArrowUp")   { e.preventDefault(); setAtMenu(s => ({ ...s, idx: Math.max(s.idx - 1, 0) })); return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pickMention(atMenu.items[atMenu.idx]); return; }
      if (e.key === "Escape") { setAtMenu(null); return; }
    }
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); }
  };

  const onDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    const files = Array.from(e.dataTransfer?.files || []);
    if (files.length) {
      setAttached(a => [...a, ...files.map(f => ({ name: f.name, size: f.size }))]);
    }
  };

  return (
    <div className="composer-wrap">
      <div className="composer-inner">
        {(attached.length > 0 || mentions.length > 0) && (
          <div className="attached-strip">
            {mentions.map((m) => {
              const Mi = Icon[m.icon] || Icon.At;
              return (
              <div key={m.id} className="attached-pill is-mention">
                <Mi className="file-icon" style={{ color: "var(--accent)" }} />
                <span>{m.label}</span>
                <button className="x" onClick={() => setMentions(ms => ms.filter(x => x.id !== m.id))}><Icon.X /></button>
              </div>
              );
            })}
            {attached.map((a, i) => (
              <div className="attached-pill" key={"a" + i}>
                <Icon.File className="file-icon" />
                <span>{a.name}</span>
                <button className="x" onClick={() => setAttached(s => s.filter((_, j) => j !== i))}><Icon.X /></button>
              </div>
            ))}
          </div>
        )}

        <div
          className={"composer" + (disabled ? " is-disabled" : "") + (dragging ? " is-drop" : "")}
          onDragOver={e => { e.preventDefault(); setDragging(true); }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
        >
          {slash && slash.items.length > 0 && (
            <SlashPopover items={slash.items} idx={slash.idx} onPick={pickSlash} title="命令" />
          )}
          {atMenu && atMenu.items.length > 0 && (
            <SlashPopover items={atMenu.items} idx={atMenu.idx} onPick={pickMention} title="引用" />
          )}

          {dragging && <div className="drop-indicator">松手附加文件</div>}

          <textarea
            ref={ta}
            className="composer-textarea"
            placeholder={isStreaming ? "Agent 正在执行… (Esc 停止)" : "描述你想做的事，或向 AI 提问。试试 / 或 @"}
            value={text}
            onChange={onChange}
            onKeyDown={onKey}
            rows={2}
          />
          <div className="composer-toolbar">
            <button className="composer-tool" title="附加文件"
                    onClick={() => setAttached(a => [...a, { name: `file-${a.length + 1}.csv`, size: 4096 }])}>
              <Icon.Paperclip />
            </button>
            <button className="composer-tool" title="@ 引用实体"
                    onClick={() => { setText(t => (t.endsWith(" ") || !t ? t : t + " ") + "@"); ta.current?.focus(); }}>
              <Icon.At />
            </button>
            <div className="composer-spacer" />
            <div className="composer-mode" title="切换 agent 模式">
              <Icon.Cpu style={{ width: 12, height: 12 }} />
              <span>Agent · max 20 steps</span>
              <Icon.ChevronDown style={{ width: 10, height: 10 }} />
            </div>
            {isStreaming ? (
              <button className="send-btn is-stop" onClick={onCancel} title="停止 (Esc)">
                <Icon.Square />
              </button>
            ) : (
              <button className={"send-btn" + (!text.trim() ? " is-disabled" : "")} onClick={send} title="发送 (Enter)">
                <Icon.ArrowUp />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function SlashPopover({ items, idx, onPick, title }) {
  return (
    <div className="slash-pop">
      <div className="slash-pop-title">{title}</div>
      {items.map((it, i) => {
        const I = Icon[it.icon] || Icon.Hammer;
        return (
          <div
            key={i}
            className={"slash-pop-row" + (i === idx ? " is-active" : "")}
            onClick={() => onPick(it)}
            onMouseEnter={() => {/* could update idx */}}
          >
            <div className="slash-pop-icon"><I /></div>
            <div className="slash-pop-label">
              <span>{it.label}</span>
              <span className="slash-pop-desc">{it.desc || it.sub || ""}</span>
            </div>
            {i === idx && <Icon.CornerDownLeft style={{ width: 11, height: 11, color: "var(--fg-faint)" }} />}
          </div>
        );
      })}
    </div>
  );
}

function mentionPool() {
  return [
    ...Forgify.forges.map(f => ({
      id: f.id,
      label: f.name + " · " + f.kind,
      icon: f.kind === "function" ? "Code" : f.kind === "handler" ? "Server" : "Workflow",
    })),
    ...Forgify.skills.map(s => ({ id: s.id, label: s.name + " · skill", icon: "Sparkles" })),
    ...(Forgify.documents[0]?.children || []).filter(d => d.kind === "page").map(d => ({
      id: d.id, label: d.title + " · doc", icon: "FileText"
    })),
  ];
}

function ChatView({ conv, tweaks, isStreaming, onSend, onCancel, onClose }) {
  const streamRef = useRef(null);
  useEffect(() => {
    let raf2 = null;
    const raf1 = requestAnimationFrame(() => {
      raf2 = requestAnimationFrame(() => {
        if (streamRef.current) streamRef.current.scrollTop = streamRef.current.scrollHeight;
      });
    });
    return () => { cancelAnimationFrame(raf1); if (raf2) cancelAnimationFrame(raf2); };
  }, []);

  return (
    <div className="chat">
      <ChatHeader title={conv.title} model={conv.model} convId={conv.id} onClose={onClose} />
      <div className="chat-stream" ref={streamRef}>
        <div className="chat-stream-inner">
          <div className="day-divider">今天 · {new Date().toLocaleDateString("zh-CN")}</div>
          {Forgify.activeMessages.map(m => (
            <MessageView key={m.id} msg={m} tweaks={tweaks} />
          ))}
        </div>
      </div>
      <Composer
        disabled={false}
        isStreaming={isStreaming}
        onSend={onSend}
        onCancel={onCancel}
      />
    </div>
  );
}

window.ChatView = ChatView;
