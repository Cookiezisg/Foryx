// HighlightedCode — syntax-highlight a code fence.
//
// Rules (industry-standard for streaming chat UIs):
//   1. streaming + no explicit lang → plain text (autodetect is too
//      expensive to run on every delta and would flicker mid-stream)
//   2. explicit lang (registered) → single-language highlight (cheap)
//   3. no lang + completed → autodetect once
//
// Single-language `lowlight.highlight()` is O(n) over content; the
// killer was `highlightAuto()` which retries all ~37 registered
// languages on every keystroke. We never run autodetect mid-stream.
//
// HighlightedCode —— 流式中只信显式 lang；没 lang 就保持纯文本；写完
// 才跑一次 autodetect。autodetect 是真正的 CPU 炸弹。

import { createElement, memo, useMemo } from "react";
import { lowlight } from "./lowlightInstance.js";

// memo: stable code blocks (any block whose source/lang/streaming
// triple is unchanged across MarkdownView re-renders) skip rendering
// entirely. Without this, every streaming delta to ANY part of the
// message would re-create JSX for every code block in it.
//
// memo —— 防止同一 message 里非流式的代码块跟着 delta 重渲染。
export const HighlightedCode = memo(function HighlightedCode({ source, lang, streaming = false }) {
  const tree = useMemo(() => {
    if (!source) return null;
    try {
      if (lang && lowlight.registered(lang)) {
        return lowlight.highlight(lang, source);
      }
      if (streaming) return null; // plain mono until fence closes
      return lowlight.highlightAuto(source);
    } catch {
      return null;
    }
  }, [source, lang, streaming]);

  if (!tree) return source;
  return <>{tree.children.map((c, i) => hastToReact(c, i))}</>;
});

// hast → React. lowlight returns hast nodes — element / text.
// className arrives as an array on properties; join into space-string.
function hastToReact(node, key) {
  if (node.type === "text") return node.value;
  if (node.type !== "element") return null;
  const { tagName, properties = {}, children = [] } = node;
  const className = Array.isArray(properties.className)
    ? properties.className.join(" ")
    : properties.className;
  const props = { key };
  if (className) props.className = className;
  return createElement(tagName, props, ...children.map((c, i) => hastToReact(c, i)));
}
