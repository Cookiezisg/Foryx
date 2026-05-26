// MarkdownView — pure-JS lightweight markdown renderer. Handles the
// formats AI replies actually use: headings, lists (bullet + numbered +
// todo), blockquote, code blocks, inline code, bold, italic, links,
// tables, horizontal rule, [[wikilink]] entity refs. Stream-friendly —
// safe to call on partial markdown mid-token (unclosed code-fence falls
// through to EOF; unmatched bold pattern just renders as plain text).
//
// MarkdownView —— 极简 markdown 渲染器；流式安全；专给 chat 流式回复 +
// 文档预览用。第三方库太大 + 全部我们用不到的安全功能，不值得拉。

import React, { useMemo } from "react";
import { HighlightedCode } from "./HighlightedCode.tsx";

interface MarkdownViewProps {
  source?: string;
  streaming?: boolean;
}

type ListItem = { text: string; done?: boolean };
type MdBlock =
  | { type: "h"; lvl: number; text: string }
  | { type: "hr" }
  | { type: "code"; lang: string; text: string }
  | { type: "quote"; text: string }
  | { type: "ul" | "ol"; items: ListItem[] }
  | { type: "table"; headers: string[]; data: string[][] }
  | { type: "p"; text: string };

export function MarkdownView({ source, streaming = false }: MarkdownViewProps) {
  const blocks = useMemo(() => parse(source || ""), [source]);
  return (
    <div className="md-body">
      {blocks.map((b, i) => renderBlock(b, i, streaming))}
    </div>
  );
}

// ── Block-level parsing ──────────────────────────────────────────────
// Exported for direct unit testing — never call from production code,
// use <MarkdownView /> instead. Tests need access to verify the
// streaming-safe parser (partial table / unclosed fence / etc).
export function parse(src: string | null | undefined): MdBlock[] {
  if (!src) return [];
  const lines = src.split("\n");
  const out: MdBlock[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];

    // Heading 1-6
    const hm = /^(#{1,6})\s+(.*)/.exec(line);
    if (hm) {
      out.push({ type: "h", lvl: hm[1].length, text: hm[2] });
      i++; continue;
    }

    // Horizontal rule
    if (/^---+\s*$/.test(line) || /^\*\*\*+\s*$/.test(line)) {
      out.push({ type: "hr" });
      i++; continue;
    }

    // Code fence
    if (/^```/.test(line)) {
      const lang = line.slice(3).trim();
      const buf = [];
      i++;
      while (i < lines.length && !/^```/.test(lines[i])) { buf.push(lines[i]); i++; }
      if (i < lines.length) i++; // consume closing ```
      out.push({ type: "code", lang, text: buf.join("\n") });
      continue;
    }

    // Blockquote (consecutive lines starting with >)
    if (/^>\s?/.test(line)) {
      const buf = [];
      while (i < lines.length && /^>\s?/.test(lines[i])) { buf.push(lines[i].replace(/^>\s?/, "")); i++; }
      out.push({ type: "quote", text: buf.join("\n") });
      continue;
    }

    // Bullet / numbered / todo list
    if (/^\s*[-*]\s+/.test(line) || /^\s*\d+\.\s+/.test(line)) {
      const items = [];
      const ordered = /^\s*\d+\./.test(line);
      while (i < lines.length && (/^\s*[-*]\s+/.test(lines[i]) || /^\s*\d+\.\s+/.test(lines[i]))) {
        const l = lines[i];
        const todo = /^\s*[-*]\s+\[( |x|X)\]\s*/.exec(l);
        if (todo) {
          items.push({ done: todo[1].toLowerCase() === "x", text: l.replace(/^\s*[-*]\s+\[( |x|X)\]\s*/, "") });
        } else {
          items.push({ text: l.replace(/^\s*[-*]\s+|^\s*\d+\.\s+/, "") });
        }
        i++;
      }
      out.push({ type: (ordered ? "ol" : "ul") as "ol" | "ul", items });
      continue;
    }

    // Table (| header | header |\n| --- | --- |\n| a | b |)
    if (/^\s*\|/.test(line) && i + 1 < lines.length && /^\s*\|[\s:|-]+\|\s*$/.test(lines[i + 1])) {
      const rows = [];
      while (i < lines.length && /^\s*\|/.test(lines[i])) { rows.push(lines[i]); i++; }
      if (rows.length >= 2) {
        const cells = (r: string) => r.split("|").slice(1, -1).map((s: string) => s.trim());
        out.push({ type: "table", headers: cells(rows[0]), data: rows.slice(2).map(cells) });
      }
      continue;
    }

    // Paragraph (consume consecutive non-special lines).
    // CRITICAL: always consume the current line first to guarantee
    // forward progress. The earlier version started buf empty AND
    // the while-loop exclusion regex matches `|`-prefixed lines —
    // so a streaming-mid table whose header arrived but separator
    // hadn't yet would push an empty paragraph without advancing i
    // → infinite loop → tab freeze. Treat the orphan `|` line as
    // plain text; once the separator streams in, the next parse
    // pass identifies it as a table.
    //
    // CRITICAL —— 必须先消费当前行，否则流式中的孤立 `|` 行（表格头到了
    // 但分隔符没到）会让段落分支死循环卡死整个页面。
    if (line.trim() === "") { i++; continue; }
    const buf = [line];
    i++;
    while (i < lines.length && lines[i].trim() !== ""
      && !/^(#{1,6}\s|>\s?|---+\s*$|```|\s*[-*]\s+|\s*\d+\.\s+|\s*\|)/.test(lines[i])) {
      buf.push(lines[i]); i++;
    }
    out.push({ type: "p", text: buf.join(" ") });
  }
  return out;
}

function renderBlock(b: MdBlock, key: number, streaming: boolean) {
  switch (b.type) {
    case "h":     return renderHeading(b.lvl, inline(b.text), key);
    case "hr":    return <hr key={key} />;
    case "code":  return (
      <pre key={key} className="code-block hljs" data-lang={b.lang || ""}>
        {b.lang && <span className="code-block-lang">{b.lang}</span>}
        <HighlightedCode source={b.text} lang={b.lang} streaming={streaming} />
      </pre>
    );
    case "quote": return <blockquote key={key}>{inline(b.text)}</blockquote>;
    case "ul":    return <ul key={key}>{b.items.map((it, j) => renderListItem(it, j))}</ul>;
    case "ol":    return <ol key={key}>{b.items.map((it, j) => renderListItem(it, j))}</ol>;
    case "table": return (
      <table key={key} className="md-table">
        <thead><tr>{b.headers.map((h, j) => <th key={j}>{inline(h)}</th>)}</tr></thead>
        <tbody>{b.data.map((row, r) => <tr key={r}>{row.map((c, k) => <td key={k}>{inline(c)}</td>)}</tr>)}</tbody>
      </table>
    );
    case "p":     return <p key={key}>{inline(b.text)}</p>;
    default:      return null;
  }
}

function renderHeading(lvl: number, children: unknown, key: number) {
  const Tag = `h${Math.min(6, Math.max(1, lvl))}` as React.ElementType;
  return <Tag key={key}>{children}</Tag>;
}

function renderListItem(it: ListItem, key: number) {
  if ("done" in it) {
    return (
      <li key={key} className={"md-todo" + (it.done ? " is-done" : "")}>
        <input type="checkbox" checked={it.done} readOnly />
        {inline(it.text)}
      </li>
    );
  }
  return <li key={key}>{inline(it.text)}</li>;
}

// ── Inline-level: **bold**, *italic*, `code`, [link](url), [[wikilink]] ──
const INLINE_RE = /(\*\*[^*\n]+\*\*|\*[^*\n]+\*|`[^`\n]+`|\[[^\]]+\]\([^)\s]+\)|\[\[[^\]]+\]\]|https?:\/\/\S+)/g;

export function inline(s: string | null): React.ReactNode[] | null {
  if (!s) return null;
  const out: React.ReactNode[] = [];
  let last = 0; let key = 0; let m;
  INLINE_RE.lastIndex = 0;
  while ((m = INLINE_RE.exec(s))) {
    if (m.index > last) out.push(s.slice(last, m.index));
    out.push(renderInlineMatch(m[0], key++));
    last = m.index + m[0].length;
  }
  if (last < s.length) out.push(s.slice(last));
  return out;
}

function renderInlineMatch(t: string, key: number) {
  if (t.startsWith("**")) return <strong key={key}>{t.slice(2, -2)}</strong>;
  if (t.startsWith("*"))  return <em key={key}>{t.slice(1, -1)}</em>;
  if (t.startsWith("`"))  return <code key={key}>{t.slice(1, -1)}</code>;
  if (t.startsWith("[[")) return <a key={key} className="entity-link" style={{ cursor: "default" }}>{t.slice(2, -2)}</a>;
  if (t.startsWith("http")) return <a key={key} href={t} target="_blank" rel="noopener noreferrer">{t}</a>;
  if (t.startsWith("[")) {
    const lm = /\[([^\]]+)\]\(([^)\s]+)\)/.exec(t);
    if (lm) return <a key={key} href={lm[2]} target="_blank" rel="noopener noreferrer">{lm[1]}</a>;
  }
  return t;
}
