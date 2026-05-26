// CodeBlockNode — Tiptap React node view for code blocks.
//
// Language picker: plain text trigger ("Auto ▾" / "JavaScript ▾") in the
// top-right; clicking opens a searchable floating popover (Notion-style,
// not a system <select>). Trigger is hidden until the block is hovered
// or the caret is inside — same affordance pattern as ActionMenu's
// rel-more-btn. Popover styling matches .action-menu + .cmdk-row.
//
// CodeBlockNode —— Tiptap React node view；右上角文字按钮 + 浮层选择
// 语言，风格对齐 .action-menu / .cmdk-row。

import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { NodeViewContent, NodeViewWrapper } from "@tiptap/react";
import {
  useFloating, autoUpdate, offset, flip, shift,
  useDismiss, useInteractions, useClick, useRole, FloatingPortal,
} from "@floating-ui/react";
import { Icon } from "@shared/ui/Icon";
import { lowlight } from "@shared/lib/highlight";

// Friendly labels for languages registered via `common`. Anything not in
// here falls through to the raw id (already readable: python / go / yaml).
const LABEL = {
  cpp: "C++", csharp: "C#", javascript: "JavaScript", typescript: "TypeScript",
  objectivec: "Objective-C", "php-template": "PHP (template)",
  "python-repl": "Python REPL", vbnet: "VB.NET",
  xml: "XML / HTML", yaml: "YAML", json: "JSON", sql: "SQL",
  bash: "Bash", shell: "Shell", scss: "SCSS", css: "CSS",
  go: "Go", rust: "Rust", ruby: "Ruby", swift: "Swift",
  kotlin: "Kotlin", java: "Java", lua: "Lua", perl: "Perl",
  php: "PHP", python: "Python", markdown: "Markdown", diff: "Diff",
  ini: "INI", makefile: "Makefile", plaintext: "Plain text",
  less: "Less", r: "R",
};
function label(id: string) { return id === "" ? "Auto" : ((LABEL as Record<string, string>)[id] || id); }

interface CodeBlockNodeProps {
  node: any;
  updateAttributes: (attrs: Record<string, any>) => void;
}

export function CodeBlockNode({ node, updateAttributes }: CodeBlockNodeProps) {
  // wrapper is intentionally bare (no .code-block / no .hljs) — those go
  // on the inner <pre> so it's the single styled surface. wrapper only
  // exists to anchor the absolute-positioned toolbar.
  return (
    <NodeViewWrapper className="cb-node">
      <div className="cb-toolbar" contentEditable={false}>
        <LangPicker
          current={node.attrs.language || ""}
          onChange={(id) => updateAttributes({ language: id === "" ? null : id })}
        />
      </div>
      <pre className="code-block hljs"><NodeViewContent as={"code" as any} /></pre>
    </NodeViewWrapper>
  );
}

function LangPicker({ current, onChange }: { current: string; onChange: (id: string) => void }) {
  const { t } = useTranslation("library");
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [idx, setIdx] = useState(0);

  const { refs, floatingStyles, context } = useFloating({
    open, onOpenChange: setOpen,
    placement: "bottom-end",
    middleware: [offset(6), flip(), shift({ padding: 8 })],
    whileElementsMounted: autoUpdate,
  });
  const click = useClick(context);
  const dismiss = useDismiss(context);
  const role = useRole(context, { role: "listbox" });
  const { getReferenceProps, getFloatingProps } = useInteractions([click, dismiss, role]);

  // Auto-first, then alphabetical list of registered languages.
  const languages = useMemo(() => ["", ...lowlight.listLanguages().sort()], []);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return languages;
    return languages.filter((id) =>
      label(id).toLowerCase().includes(q) || id.toLowerCase().includes(q)
    );
  }, [languages, query]);
  useEffect(() => setIdx(0), [query]);
  useEffect(() => { if (!open) setQuery(""); }, [open]);

  const pick = (id: string) => { onChange(id); setOpen(false); };

  return (
    <>
      <button
        ref={refs.setReference}
        {...getReferenceProps()}
        className={"cb-lang-btn" + (open ? " is-open" : "")}
        title={t("codeBlock.langPickerTitle")}
      >
        <span>{label(current)}</span>
        <Icon.ChevronDown style={{ width: 10, height: 10 }} />
      </button>
      {open && (
        <FloatingPortal>
          <div
            ref={refs.setFloating}
            style={floatingStyles}
            {...getFloatingProps()}
            className="cb-lang-pop"
          >
            <div className="cb-lang-search-wrap">
              <Icon.Search className="icon" style={{ width: 12, height: 12 }} />
              <input
                autoFocus
                className="cb-lang-search"
                placeholder={t("codeBlock.langSearchPlaceholder")}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "ArrowDown") { e.preventDefault(); setIdx((n) => Math.min(n + 1, filtered.length - 1)); }
                  else if (e.key === "ArrowUp") { e.preventDefault(); setIdx((n) => Math.max(n - 1, 0)); }
                  else if (e.key === "Enter") { e.preventDefault(); if (filtered[idx] != null) pick(filtered[idx]); }
                }}
              />
            </div>
            <div className="cb-lang-list">
              {filtered.length === 0 && <div className="cb-lang-empty">{t("codeBlock.langEmpty")}</div>}
              {filtered.map((id, i) => (
                <button
                  key={id || "auto"}
                  className={"cb-lang-row" + (i === idx ? " is-active" : "")}
                  onMouseEnter={() => setIdx(i)}
                  onClick={() => pick(id)}
                >
                  <span style={{ flex: 1 }}>{label(id)}</span>
                  {current === id && <Icon.Check style={{ width: 12, height: 12, color: "var(--accent)" }} />}
                </button>
              ))}
            </div>
          </div>
        </FloatingPortal>
      )}
    </>
  );
}
