// DocEditor — Notion-style WYSIWYG built on Tiptap. Renders the doc as
// final-look (h/list/code/quote/link), markdown shortcuts auto-format
// (`# `, `- `, `> `, `\`\`\`` etc. via StarterKit input rules), `/`
// opens a command panel (slash menu), `@` opens a doc reference picker.
//
// Persistence: doc.content is stored as Markdown on the backend. We use
// tiptap-markdown for round-trip — `editor.storage.markdown.getMarkdown()`
// returns markdown for save; `editor.commands.setContent(md)` parses it
// back on load.
//
// DocEditor —— Notion 风格所见即所得；后台仍存 markdown，靠 tiptap-markdown
// 做双向转换。

import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useEditor, EditorContent, ReactRenderer, ReactNodeViewRenderer } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import Mention from "@tiptap/extension-mention";
import CodeBlockLowlight from "@tiptap/extension-code-block-lowlight";
import { Markdown } from "tiptap-markdown";
import { lowlight } from "../../shared/lib/highlight/index.js";
import { CodeBlockNode } from "../../pages/library/ui/CodeBlockNode.jsx";

// Extend CodeBlockLowlight to render its node via our React component
// (language <select> in the corner). Must extend BEFORE configure().
const CodeBlockWithLangPicker = CodeBlockLowlight.extend({
  addNodeView() {
    return ReactNodeViewRenderer(CodeBlockNode);
  },
});
import { Extension } from "@tiptap/core";
import Suggestion from "@tiptap/suggestion";
import tippy from "tippy.js";
import "tippy.js/dist/tippy.css";

import { Icon } from "../../components/primitives/Icon.jsx";

// ── Slash command vocabulary ─────────────────────────────────────────
function makeSlashItems(t) {
  return [
    { key: "h1",    title: t("editor.h1Title"),    desc: t("editor.h1Desc"),    icon: "Hash",       run: (chain) => chain.toggleHeading({ level: 1 }) },
    { key: "h2",    title: t("editor.h2Title"),    desc: t("editor.h2Desc"),    icon: "Hash",       run: (chain) => chain.toggleHeading({ level: 2 }) },
    { key: "h3",    title: t("editor.h3Title"),    desc: t("editor.h3Desc"),    icon: "Hash",       run: (chain) => chain.toggleHeading({ level: 3 }) },
    { key: "ul",    title: t("editor.ulTitle"),    desc: t("editor.ulDesc"),    icon: "List",       run: (chain) => chain.toggleBulletList() },
    { key: "ol",    title: t("editor.olTitle"),    desc: t("editor.olDesc"),    icon: "ListChecks", run: (chain) => chain.toggleOrderedList() },
    { key: "todo",  title: t("editor.todoTitle"),  desc: t("editor.todoDesc"),  icon: "Check",      run: (chain) => chain.toggleTaskList?.() },
    { key: "quote", title: t("editor.quoteTitle"), desc: t("editor.quoteDesc"), icon: "Quote",      run: (chain) => chain.toggleBlockquote() },
    { key: "code",  title: t("editor.codeTitle"),  desc: t("editor.codeDesc"),  icon: "Code",       run: (chain) => chain.toggleCodeBlock() },
    { key: "hr",    title: t("editor.hrTitle"),    desc: t("editor.hrDesc"),    icon: "Minus",      run: (chain) => chain.setHorizontalRule() },
  ];
}

function iconOf(name) {
  return Icon[name] || Icon.Hash || Icon.Hammer;
}

// ── React component that the suggestion popup renders ────────────────
import { useState as useReactState, useEffect as useReactEffect, useImperativeHandle as useReactIH, forwardRef as fwd } from "react";

const SuggestionList = fwd(function SuggestionList({ items, command, kind }, ref) {
  const [idx, setIdx] = useReactState(0);
  const { t } = useTranslation("library");
  useReactEffect(() => setIdx(0), [items]);
  useReactIH(ref, () => ({
    onKeyDown({ event }) {
      if (event.key === "ArrowDown") { setIdx((n) => (n + 1) % items.length); return true; }
      if (event.key === "ArrowUp")   { setIdx((n) => (n - 1 + items.length) % items.length); return true; }
      if (event.key === "Enter")     { const it = items[idx]; if (it) command(it); return true; }
      if (event.key === "Tab")       { const it = items[idx]; if (it) command(it); return true; }
      return false;
    },
  }), [items, idx, command]);

  if (!items.length) {
    return <div className="doc-floating-menu" style={{ position: "relative", inset: "auto" }}>
      <div className="doc-floating-menu-empty">{t("editor.slashEmpty")}</div>
    </div>;
  }
  return (
    <div className="doc-floating-menu" style={{ position: "relative", inset: "auto" }}>
      <div className="doc-floating-menu-head">{kind === "slash" ? t("editor.slashHeadSlash") : t("editor.slashHeadMention")}</div>
      {items.map((it, i) => {
        const I = kind === "slash" ? iconOf(it.icon) : Icon.FileText;
        return (
          <button key={it.key || it.id} className={i === idx ? "is-active" : ""}
                  onClick={() => command(it)}>
            <I style={{ width: 12, height: 12 }} />
            <span style={{ flex: 1 }}>{kind === "slash" ? it.title : it.label}</span>
            {kind === "slash"
              ? <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>{it.desc}</span>
              : <span className="cell-mono" style={{ color: "var(--fg-faint)", fontSize: 11 }}>{it.id}</span>}
          </button>
        );
      })}
    </div>
  );
});

// ── Suggestion render helper (shared by slash + mention) ─────────────
function makeRender() {
  let component; let popup;
  return {
    onStart(props) {
      component = new ReactRenderer(SuggestionList, { props: { ...props, kind: props.kind }, editor: props.editor });
      if (!props.clientRect) return;
      popup = tippy("body", {
        getReferenceClientRect: props.clientRect,
        appendTo: () => document.body,
        content: component.element,
        showOnCreate: true,
        interactive: true,
        trigger: "manual",
        placement: "bottom-start",
        offset: [0, 6],
        theme: "forgify",
      });
    },
    onUpdate(props) {
      component?.updateProps({ ...props, kind: props.kind });
      if (props.clientRect && popup?.[0]) popup[0].setProps({ getReferenceClientRect: props.clientRect });
    },
    onKeyDown(props) {
      if (props.event.key === "Escape") { popup?.[0]?.hide(); return true; }
      return component?.ref?.onKeyDown?.(props) || false;
    },
    onExit() { popup?.[0]?.destroy(); component?.destroy(); popup = null; component = null; },
  };
}

// Module-level ref updated by DocEditor with the current translated items.
// Allows the Extension (instantiated once) to always filter against the
// latest locale without being recreated.
const slashItemsRef = { current: [] };

// ── Slash command extension (custom Extension wrapping Suggestion) ───
const SlashCommand = Extension.create({
  name: "slashCommand",
  addOptions() {
    return {
      suggestion: {
        char: "/",
        startOfLine: false,
        kind: "slash",
        command: ({ editor, range, props }) => {
          const chain = editor.chain().focus().deleteRange(range);
          const result = props.run(chain);
          (result || chain).run();
        },
        items: ({ query }) => {
          const q = query.toLowerCase();
          return slashItemsRef.current.filter((it) =>
            it.title.toLowerCase().includes(q) || it.desc.toLowerCase().includes(q)
          ).slice(0, 9);
        },
        render: makeRender,
      },
    };
  },
  addProseMirrorPlugins() {
    return [
      Suggestion({
        editor: this.editor,
        ...this.options.suggestion,
        // suggestion's render() doesn't pass `kind` through; inject it.
        render: () => {
          const r = makeRender();
          return {
            onStart: (p) => r.onStart({ ...p, kind: "slash" }),
            onUpdate: (p) => r.onUpdate({ ...p, kind: "slash" }),
            onKeyDown: (p) => r.onKeyDown(p),
            onExit: () => r.onExit(),
          };
        },
      }),
    ];
  },
});

// ── DocEditor component ──────────────────────────────────────────────
export const DocEditor = forwardRef(function DocEditor({
  initialMarkdown,
  placeholder,
  onChange,
  documentsLookup,    // () => [{id, name}, ...] for @ mention suggestions
}, ref) {
  const { t } = useTranslation("library");
  const resolvedPlaceholder = placeholder ?? t("editor.placeholder");
  slashItemsRef.current = makeSlashItems(t);

  const cbRef = useRef(onChange);
  cbRef.current = onChange;
  const docsRef = useRef(documentsLookup);
  docsRef.current = documentsLookup;

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        codeBlock: false, // replaced by CodeBlockLowlight below
        heading: { levels: [1, 2, 3] },
      }),
      CodeBlockWithLangPicker.configure({
        lowlight,
        defaultLanguage: null,
        HTMLAttributes: { class: "code-block hljs" },
      }),
      Placeholder.configure({ placeholder: resolvedPlaceholder, emptyEditorClass: "is-empty" }),
      Markdown.configure({ html: false, tightLists: true, transformPastedText: true, breaks: true }),
      SlashCommand,
      Mention.configure({
        HTMLAttributes: { class: "doc-mention" },
        renderText: ({ node }) => `[[${node.attrs.label || node.attrs.id}]]`,
        renderHTML: ({ node, HTMLAttributes }) => [
          "a", { ...HTMLAttributes, "data-id": node.attrs.id, class: "doc-mention" },
          `[[${node.attrs.label || node.attrs.id}]]`,
        ],
        suggestion: {
          char: "@",
          items: ({ query }) => {
            const all = (docsRef.current?.() || []);
            const q = query.toLowerCase();
            return all.filter((d) => (d.name || d.id || "").toLowerCase().includes(q)).slice(0, 8);
          },
          command: ({ editor, range, props }) => {
            editor.chain().focus().deleteRange(range)
              .insertContent([
                { type: "mention", attrs: { id: props.id, label: props.name || props.id } },
                { type: "text", text: " " },
              ])
              .run();
          },
          render: () => {
            const r = makeRender();
            return {
              onStart: (p) => r.onStart({ ...p, kind: "mention" }),
              onUpdate: (p) => r.onUpdate({ ...p, kind: "mention" }),
              onKeyDown: (p) => r.onKeyDown(p),
              onExit: () => r.onExit(),
            };
          },
        },
      }),
    ],
    content: "",
    onUpdate: ({ editor }) => {
      const md = editor.storage.markdown.getMarkdown();
      cbRef.current?.(md);
    },
  });

  // Hydrate from initialMarkdown on first load / doc switch.
  useEffect(() => {
    if (!editor) return;
    const current = editor.storage.markdown.getMarkdown();
    if ((initialMarkdown || "") === current) return;
    // setContent triggers onUpdate; suppress by passing false.
    editor.commands.setContent(initialMarkdown || "", false);
  }, [editor, initialMarkdown]);

  useImperativeHandle(ref, () => ({
    focus: () => editor?.commands.focus(),
    getMarkdown: () => editor?.storage.markdown.getMarkdown() || "",
  }), [editor]);

  return <EditorContent editor={editor} className="doc-editor-rich" />;
});
