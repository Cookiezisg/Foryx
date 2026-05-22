// CodeBlockNode — Tiptap React node view for code blocks.
//
//   - Top-right floating language <select> populated from
//     lowlight.listLanguages() (the same 36 common langs the highlighter
//     knows about). 'Auto' = leave attrs.language null → highlightAuto
//     guesses; explicit pick = lock + re-highlight under that grammar.
//   - <NodeViewContent as="code"> is the contentEditable surface;
//     selecting / typing / copying behaves normally — the toolbar
//     itself is marked contentEditable={false} so caret can't land
//     inside it.
//
// CodeBlockNode —— code block 的 Tiptap React node view；右上角浮一个
// 语言下拉，'Auto' 走 highlightAuto，显式选语言则锁定再高亮。

import { NodeViewContent, NodeViewWrapper } from "@tiptap/react";
import { useMemo } from "react";
import { lowlight } from "../../components/shared/lowlightInstance.js";

// Friendly labels for the languages we register via `common`. Everything
// not in here falls back to the raw id, which is already readable
// (python / javascript / yaml…).
const LABEL_OVERRIDES = {
  cpp: "C++", csharp: "C#", javascript: "JavaScript", typescript: "TypeScript",
  objectivec: "Objective-C", "php-template": "PHP (template)", "python-repl": "Python REPL",
  vbnet: "VB.NET", xml: "XML / HTML", yaml: "YAML", json: "JSON", sql: "SQL",
  bash: "Bash", shell: "Shell", scss: "SCSS", css: "CSS", html: "HTML",
  go: "Go", rust: "Rust", ruby: "Ruby", swift: "Swift", kotlin: "Kotlin",
  java: "Java", lua: "Lua", perl: "Perl", php: "PHP", python: "Python",
  markdown: "Markdown", diff: "Diff", ini: "INI", makefile: "Makefile",
  plaintext: "Plain text", less: "Less", r: "R",
};

function label(id) { return LABEL_OVERRIDES[id] || id; }

export function CodeBlockNode({ node, updateAttributes }) {
  const languages = useMemo(() => lowlight.listLanguages().sort(), []);
  const current = node.attrs.language || "";

  return (
    <NodeViewWrapper className="code-block hljs cb-node">
      <div className="cb-toolbar" contentEditable={false}>
        <select
          className="cb-lang-select"
          value={current}
          onChange={(e) => {
            const v = e.target.value;
            updateAttributes({ language: v === "" ? null : v });
          }}
          title="选择代码语言（Auto = 自动识别）"
        >
          <option value="">Auto</option>
          {languages.map((id) => (
            <option key={id} value={id}>{label(id)}</option>
          ))}
        </select>
      </div>
      <pre><NodeViewContent as="code" /></pre>
    </NodeViewWrapper>
  );
}
