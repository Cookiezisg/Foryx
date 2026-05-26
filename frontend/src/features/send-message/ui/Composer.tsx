// Composer — message input. textarea auto-grows up to 200px.
// @-mention menu activates when the last token begins with @ — searches
// across functions / handlers / workflows / documents.
// Drag-drop attaches files. Enter sends, Shift+Enter inserts newline,
// Esc cancels a streaming run.
//
// Composer —— 文本输入；只支持 @ 引用实体；拖拽附件；
// Enter 发送 / Shift+Enter 换行 / Esc 取消流式。
// 不做 / slash 菜单（用户明确不要）。

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { useFunctions } from "@entities/function";
import { useHandlers } from "@entities/handler";
import { useWorkflows } from "@entities/workflow";
import { useDocuments } from "@entities/document";

export function Composer({ disabled, isStreaming, onSend, onCancel }: { disabled?: any; isStreaming?: any; onSend?: any; onCancel?: any }) {
  const { t } = useTranslation("conv");
  const [text, setText] = useState("");
  const [attached, setAttached] = useState([]);
  const [mentions, setMentions] = useState([]);
  const [atMenu, setAtMenu] = useState(null);
  const [dragging, setDragging] = useState(false);
  const ta = useRef(null);
  const fileInput = useRef(null);

  const { data: functions = [] } = useFunctions();
  const { data: handlers = [] } = useHandlers();
  const { data: workflows = [] } = useWorkflows();
  const { data: documents = [] } = useDocuments();

  useEffect(() => {
    if (!ta.current) return;
    ta.current.style.height = "auto";
    ta.current.style.height = Math.min(200, ta.current.scrollHeight) + "px";
  }, [text]);

  const send = () => {
    const t = text.trim();
    if (!t || disabled) return;
    onSend?.({ content: t, attachments: attached, mentions });
    setText("");
    setAttached([]);
    setMentions([]);
    setAtMenu(null);
  };

  const mentionPool = () => [
    ...functions.map((f) => ({ type: "function", id: f.id, label: f.name + " · function", icon: "Code" })),
    ...handlers.map((h) => ({ type: "handler", id: h.id, label: h.name + " · handler", icon: "Server" })),
    ...workflows.map((w) => ({ type: "workflow", id: w.id, label: w.name + " · workflow", icon: "Workflow" })),
    ...documents.map((d) => { const da = d as any; return { type: "document", id: da.id, label: (da.name || da.title || da.id) + " · doc", icon: "FileText" }; }),
  ];

  const onChange = (e) => {
    const v = e.target.value;
    setText(v);
    const m = v.match(/(?:^|\s)@([^\s]*)$/);
    if (m) {
      const q = m[1].toLowerCase();
      const items = mentionPool().filter((it) => it.label.toLowerCase().includes(q)).slice(0, 8);
      setAtMenu({ items, idx: 0, q });
    } else {
      setAtMenu(null);
    }
  };

  const pickMention = (it) => {
    setMentions((ms) => (ms.find((x) => x.id === it.id) ? ms : [...ms, it]));
    setText((t) => t.replace(/(?:^|\s)@[^\s]*$/, (m) => (m.startsWith(" ") ? " " : "")));
    setAtMenu(null);
    ta.current?.focus();
  };

  const onKey = (e) => {
    if (atMenu?.items.length) {
      if (e.key === "ArrowDown") { e.preventDefault(); setAtMenu((s) => ({ ...s, idx: Math.min(s.idx + 1, s.items.length - 1) })); return; }
      if (e.key === "ArrowUp")   { e.preventDefault(); setAtMenu((s) => ({ ...s, idx: Math.max(s.idx - 1, 0) })); return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pickMention(atMenu.items[atMenu.idx]); return; }
      if (e.key === "Escape") { setAtMenu(null); return; }
    }
    if (e.key === "Escape" && isStreaming) { onCancel?.(); return; }
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); }
  };

  const onDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    const files = Array.from(e.dataTransfer?.files || []);
    if (files.length) onPickFiles(files);
  };
  const onPickFiles = (files) => {
    setAttached((a) => [...a, ...files.map((f) => ({ name: f.name, size: f.size, file: f }))]);
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
                  <button className="x" onClick={() => setMentions((ms) => ms.filter((x) => x.id !== m.id))}>
                    <Icon.X />
                  </button>
                </div>
              );
            })}
            {attached.map((a, i) => (
              <div className="attached-pill" key={"a" + i}>
                <Icon.File className="file-icon" />
                <span>{a.name}</span>
                <button className="x" onClick={() => setAttached((s) => s.filter((_, j) => j !== i))}>
                  <Icon.X />
                </button>
              </div>
            ))}
          </div>
        )}

        <div
          className={"composer" + (disabled ? " is-disabled" : "") + (dragging ? " is-drop" : "")}
          onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
        >
          {atMenu?.items.length > 0 && (
            <MentionPopover items={atMenu.items} idx={atMenu.idx} onPick={pickMention} title={t("composer.mentionPopoverTitle")} />
          )}
          {dragging && <div className="drop-indicator">{t("composer.dropIndicator")}</div>}

          <textarea
            ref={ta}
            className="composer-textarea"
            placeholder={isStreaming ? t("composer.placeholderStreaming") : t("composer.placeholderIdle")}
            value={text}
            onChange={onChange}
            onKeyDown={onKey}
            rows={2}
            disabled={disabled}
          />

          <input
            ref={fileInput}
            type="file"
            multiple
            style={{ display: "none" }}
            onChange={(e) => { onPickFiles(Array.from(e.target.files || [])); e.target.value = ""; }}
          />

          <div className="composer-toolbar">
            <button className="composer-tool" title={t("composer.attachFile")} onClick={() => fileInput.current?.click()}>
              <Icon.Paperclip />
            </button>
            <button
              className="composer-tool"
              title={t("composer.mentionEntity")}
              onClick={() => { setText((t) => (t.endsWith(" ") || !t ? t : t + " ") + "@"); ta.current?.focus(); }}
            >
              <Icon.At />
            </button>
            <div className="composer-spacer" />
            {isStreaming ? (
              <button className="send-btn is-stop" onClick={onCancel} title={t("composer.stopStreaming")}>
                <Icon.Square />
              </button>
            ) : (
              <button
                className={"send-btn" + (!text.trim() ? " is-disabled" : "")}
                onClick={send}
                title={t("composer.send")}
                disabled={!text.trim() || disabled}
              >
                <Icon.ArrowUp />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function MentionPopover({ items, idx, onPick, title }) {
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
          >
            <div className="slash-pop-icon"><I /></div>
            <div className="slash-pop-label">
              <span>{it.label}</span>
            </div>
            {i === idx && <Icon.CornerDownLeft style={{ width: 11, height: 11, color: "var(--fg-faint)" }} />}
          </div>
        );
      })}
    </div>
  );
}
