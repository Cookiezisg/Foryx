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
import { lowlight } from "@shared/lib/highlight";
import { CodeBlockNode } from "./CodeBlockNode.tsx";

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

import { Icon } from "@shared/ui/Icon";

// ── Slash command vocabulary ─────────────────────────────────────────
function makeSlashItems(t: (key: string) => string) {
  // Tiptap chain API has no exported type; `any` is unavoidable here.
  return [
    { key: "h1",    title: t("editor.h1Title"),    desc: t("editor.h1Desc"),    icon: "Hash",       run: (chain: any) => chain.toggleHeading({ level: 1 }) }, // Tiptap chain
    { key: "h2",    title: t("editor.h2Title"),    desc: t("editor.h2Desc"),    icon: "Hash",       run: (chain: any) => chain.toggleHeading({ level: 2 }) }, // Tiptap chain
    { key: "h3",    title: t("editor.h3Title"),    desc: t("editor.h3Desc"),    icon: "Hash",       run: (chain: any) => chain.toggleHeading({ level: 3 }) }, // Tiptap chain
    { key: "ul",    title: t("editor.ulTitle"),    desc: t("editor.ulDesc"),    icon: "List",       run: (chain: any) => chain.toggleBulletList() },           // Tiptap chain
    { key: "ol",    title: t("editor.olTitle"),    desc: t("editor.olDesc"),    icon: "ListChecks", run: (chain: any) => chain.toggleOrderedList() },          // Tiptap chain
    { key: "todo",  title: t("editor.todoTitle"),  desc: t("editor.todoDesc"),  icon: "Check",      run: (chain: any) => chain.toggleTaskList?.() },           // Tiptap chain
    { key: "quote", title: t("editor.quoteTitle"), desc: t("editor.quoteDesc"), icon: "Quote",      run: (chain: any) => chain.toggleBlockquote() },           // Tiptap chain
    { key: "code",  title: t("editor.codeTitle"),  desc: t("editor.codeDesc"),  icon: "Code",       run: (chain: any) => chain.toggleCodeBlock() },            // Tiptap chain
    { key: "hr",    title: t("editor.hrTitle"),    desc: t("editor.hrDesc"),    icon: "Minus",      run: (chain: any) => chain.setHorizontalRule() },          // Tiptap chain
  ];
}

function iconOf(name: string) {
  return (Icon as any)[name] || Icon.Hammer; // Icon map has no string index signature
}

// ── React component that the suggestion popup renders ────────────────
import { useState as useReactState, useEffect as useReactEffect, useImperativeHandle as useReactIH, forwardRef as fwd } from "react";

// Tiptap suggestion API: items/command shapes have no exported types.
const SuggestionList = fwd(function SuggestionList({ items, command, kind }: { items: any[]; command: any; kind?: string }, ref) { // Tiptap suggestion
  const [idx, setIdx] = useReactState(0);
  const { t } = useTranslation("library");
  useReactEffect(() => setIdx(0), [items]);
  useReactIH(ref, () => ({
    onKeyDown({ event }: { event: KeyboardEvent }) {
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
  let component: any; // Tiptap ReactRenderer — no exported type
  let popup: any;     // tippy instance array — no typed return from tippy("body",...)
  return {
    onStart(props: any) { // Tiptap SuggestionProps — untyped
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
    onUpdate(props: any) { // Tiptap SuggestionProps — untyped
      component?.updateProps({ ...props, kind: props.kind });
      if (props.clientRect && popup?.[0]) popup[0].setProps({ getReferenceClientRect: props.clientRect });
    },
    onKeyDown(props: any) { // Tiptap SuggestionKeyDownProps — untyped
      if (props.event.key === "Escape") { popup?.[0]?.hide(); return true; }
      return component?.ref?.onKeyDown?.(props) || false;
    },
    onExit() { popup?.[0]?.destroy(); component?.destroy(); popup = null; component = null; },
  };
}

// Module-level ref updated by DocEditor with the current translated items.
// Allows the Extension (instantiated once) to always filter against the
// latest locale without being recreated.
const slashItemsRef: { current: any[] } = { current: [] }; // slash item shape is internal; no typed cache needed

// ── Slash command extension (custom Extension wrapping Suggestion) ───
const SlashCommand = Extension.create({
  name: "slashCommand",
  addOptions() {
    return {
      suggestion: {
        char: "/",
        startOfLine: false,
        kind: "slash",
        command: ({ editor, range, props }: { editor: any; range: any; props: any }) => { // Tiptap Extension command callback — untyped
          const chain = editor.chain().focus().deleteRange(range);
          const result = props.run(chain);
          (result || chain).run();
        },
        items: ({ query }: { query: string }) => {
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
interface DocEditorProps {
  initialMarkdown?: string;
  placeholder?: string;
  onChange?: (md: string) => void;
  documentsLookup?: () => { id: string; name: string }[];
}

export interface DocEditorHandle {
  focus: () => void;
  getMarkdown: () => string;
}

export const DocEditor = forwardRef<DocEditorHandle, DocEditorProps>(function DocEditor({
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
      (Mention as any).configure({ // Tiptap Mention has no exported configure() type
        HTMLAttributes: { class: "doc-mention" },
        renderText: ({ node }: any) => `[[${node.attrs.label || node.attrs.id}]]`,         // Tiptap node view callback — untyped
        renderHTML: ({ node, HTMLAttributes }: any) => [                                   // Tiptap node view callback — untyped
          "a", { ...HTMLAttributes, "data-id": node.attrs.id, class: "doc-mention" },
          `[[${node.attrs.label || node.attrs.id}]]`,
        ],
        suggestion: {
          char: "@",
          items: ({ query }: any) => {                                                     // Tiptap suggestion items — untyped
            const all = (docsRef.current?.() || []);
            const q = query.toLowerCase();
            return all.filter((d) => (d.name || d.id || "").toLowerCase().includes(q)).slice(0, 8);
          },
          command: ({ editor, range, props }: any) => {                                    // Tiptap suggestion command — untyped
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
              onStart: (p: any) => r.onStart({ ...p, kind: "mention" }),   // Tiptap SuggestionProps — untyped
              onUpdate: (p: any) => r.onUpdate({ ...p, kind: "mention" }),  // Tiptap SuggestionProps — untyped
              onKeyDown: (p: any) => r.onKeyDown(p),                        // Tiptap SuggestionKeyDownProps — untyped
              onExit: () => r.onExit(),
            };
          },
        },
      }),
    ],
    content: "",
    onUpdate: ({ editor }) => {
      const md = (editor.storage as any).markdown.getMarkdown(); // tiptap-markdown storage — not typed
      cbRef.current?.(md);
    },
  });

  // Hydrate from initialMarkdown on first load / doc switch.
  useEffect(() => {
    if (!editor) return;
    const current = (editor.storage as any).markdown.getMarkdown(); // tiptap-markdown storage — not typed
    if ((initialMarkdown || "") === current) return;
    // setContent triggers onUpdate; suppress by passing false.
    (editor.commands as any).setContent(initialMarkdown || "", false); // Tiptap commands omit second `emitUpdate` param in types
  }, [editor, initialMarkdown]);

  useImperativeHandle(ref, () => ({
    focus: () => editor?.commands.focus(),
    getMarkdown: () => (editor?.storage as any)?.markdown.getMarkdown() || "", // tiptap-markdown storage — not typed
  }), [editor]);

  return <EditorContent editor={editor} className="doc-editor-rich" />;
});
