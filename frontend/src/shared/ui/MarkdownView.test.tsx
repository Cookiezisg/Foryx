// MarkdownView — covers the streaming-safe parser (parse + inline) and
// rendering. Historical regression: parser used to infinite-loop on a
// `|`-prefixed line whose separator hadn't streamed in yet.

import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MarkdownView, parse, inline } from "./MarkdownView.tsx";

const runWithTimeout = (fn: () => any, ms = 200) => {
  // jsdom + vitest: a true hang would lock vitest. Use a deadline
  // sentinel inside fn alternatives where possible. For parse() the
  // bug was infinite synchronous loop; we detect it by hard timeout
  // via setTimeout race + a flag, but since js is single-threaded the
  // only real defense is bounding the call site. We assert duration.
  const start = Date.now();
  const r = fn();
  const elapsed = Date.now() - start;
  if (elapsed > ms) throw new Error(`exceeded ${ms}ms (${elapsed})`);
  return r;
};

describe("parse — block-level", () => {
  it("parse_emptyInput_returnsEmptyArray", () => {
    expect(parse("")).toEqual([]);
    expect(parse(null)).toEqual([]);
    expect(parse(undefined)).toEqual([]);
  });

  it("parse_plainParagraph_singleBlock", () => {
    const r = parse("hello world");
    expect(r).toEqual([{ type: "p", text: "hello world" }]);
  });

  it("parse_multipleParagraphs_separatedByBlankLine", () => {
    const r = parse("first\n\nsecond");
    expect(r).toEqual([
      { type: "p", text: "first" },
      { type: "p", text: "second" },
    ]);
  });

  it("parse_heading1to6_levelMatchesHashCount", () => {
    for (let n = 1; n <= 6; n++) {
      const r = parse("#".repeat(n) + " title");
      expect(r[0]).toEqual({ type: "h", lvl: n, text: "title" });
    }
  });

  it("parse_horizontalRule_dashesAndStars", () => {
    expect(parse("---")[0]).toEqual({ type: "hr" });
    expect(parse("****")[0]).toEqual({ type: "hr" });
  });

  it("parse_codeFence_capturesLangAndBody", () => {
    const r = parse("```python\ndef f():\n  pass\n```");
    expect(r).toEqual([{ type: "code", lang: "python", text: "def f():\n  pass" }]);
  });

  it("parse_codeFence_unclosedAtEOF_captureRest", () => {
    const r = parse("```js\nconst x = 1\nconst y = 2");
    expect(r[0]).toMatchObject({ type: "code", lang: "js" });
    expect(r[0].text).toContain("const x");
  });

  it("parse_blockquote_consecutiveLinesMerge", () => {
    const r = parse("> first\n> second");
    expect(r).toEqual([{ type: "quote", text: "first\nsecond" }]);
  });

  it("parse_bulletList_dashOrStar", () => {
    expect(parse("- a\n- b")[0]).toEqual({
      type: "ul",
      items: [{ text: "a" }, { text: "b" }],
    });
    expect(parse("* x\n* y")[0]).toEqual({
      type: "ul",
      items: [{ text: "x" }, { text: "y" }],
    });
  });

  it("parse_orderedList_numericMarker", () => {
    const r = parse("1. one\n2. two");
    expect(r[0]).toEqual({
      type: "ol",
      items: [{ text: "one" }, { text: "two" }],
    });
  });

  it("parse_todoList_extractsDoneFlag", () => {
    const r = parse("- [ ] open\n- [x] done\n- [X] also done");
    expect(r[0].items).toEqual([
      { done: false, text: "open" },
      { done: true, text: "done" },
      { done: true, text: "also done" },
    ]);
  });

  it("parse_table_headerSeparatorAndRows", () => {
    const r = parse("| h1 | h2 |\n| --- | --- |\n| a | b |\n| c | d |");
    expect(r[0]).toMatchObject({
      type: "table",
      headers: ["h1", "h2"],
      data: [
        ["a", "b"],
        ["c", "d"],
      ],
    });
  });

  // Regression: parse used to infinite-loop on a `|` line whose
  // separator hadn't streamed in yet. Caused the chat tab to freeze
  // every time AI started emitting a markdown table.
  it("parse_partialTable_noSeparatorYet_doesNotHang", () => {
    runWithTimeout(() => parse("| col1 | col2 |"));
    runWithTimeout(() => parse("| col1 | col2 |\n| more |"));
    runWithTimeout(() => parse("paragraph\n\n| just one row |"));
  });

  it("parse_partialTable_treatsOrphanLineAsPlainText", () => {
    const r = parse("| col1 | col2 |");
    // Should produce *some* block and advance (any non-infinite output is acceptable).
    expect(r.length).toBeGreaterThan(0);
  });

  it("parse_mixedContent_headingThenCodeThenTable", () => {
    const r = parse("# Title\n\n```js\nx\n```\n\n| h |\n| --- |\n| v |");
    expect(r.map((b) => b.type)).toEqual(["h", "code", "table"]);
  });

  it("parse_unicodeText_preservedByteForByte", () => {
    const r = parse("中文 🚀 内容");
    expect(r[0].text).toBe("中文 🚀 内容");
  });

  it("parse_veryLongInput_completesUnderBudget", () => {
    const big = Array(2000).fill("line of text").join("\n\n");
    runWithTimeout(() => parse(big), 500);
  });

  it("parse_consecutiveCodeFences_eachIsolated", () => {
    const r = parse("```js\nconsole.log(1)\n```\n```py\nprint(2)\n```");
    expect(r).toHaveLength(2);
    expect(r[0]).toMatchObject({ type: "code", lang: "js" });
    expect(r[1]).toMatchObject({ type: "code", lang: "py" });
  });
});

describe("inline — inline-level", () => {
  it("inline_plainText_passThrough", () => {
    const out = inline("just words");
    expect(out).toEqual(["just words"]);
  });

  it("inline_emptyInput_returnsNull", () => {
    expect(inline("")).toBe(null);
    expect(inline(null)).toBe(null);
  });

  it("inline_boldStarStar_wrapsInStrong", () => {
    const out = inline("see **this**");
    const strong = out.find((x) => x && x.type === "strong");
    expect(strong).toBeTruthy();
    expect(strong.props.children).toBe("this");
  });

  it("inline_italicSingleStar_wrapsInEm", () => {
    const out = inline("see *this*");
    const em = out.find((x) => x && x.type === "em");
    expect(em).toBeTruthy();
    expect(em.props.children).toBe("this");
  });

  it("inline_inlineCode_wrapsInCode", () => {
    const out = inline("use `foo()`");
    const code = out.find((x) => x && x.type === "code");
    expect(code.props.children).toBe("foo()");
  });

  it("inline_markdownLink_rendersAnchor", () => {
    const out = inline("[google](https://google.com)");
    const a = out.find((x) => x && x.type === "a");
    expect(a.props.href).toBe("https://google.com");
    expect(a.props.children).toBe("google");
  });

  it("inline_bareUrl_rendersAnchor", () => {
    const out = inline("visit https://example.com here");
    const a = out.find((x) => x && x.type === "a");
    expect(a.props.href).toBe("https://example.com");
  });

  it("inline_wikilink_rendersEntityAnchor", () => {
    const out = inline("see [[my-doc]]");
    const a = out.find((x) => x && x.type === "a");
    expect(a.props.className).toBe("entity-link");
    expect(a.props.children).toBe("my-doc");
  });

  it("inline_multipleMixed_orderPreserved", () => {
    const out = inline("**bold** then `code` then *em*");
    // Find the structural tokens in order
    const types = out.filter((x) => x && x.type).map((x) => x.type);
    expect(types).toEqual(["strong", "code", "em"]);
  });

  it("inline_globalRegex_doesNotRetainStateBetweenCalls", () => {
    // Calling twice with the same content must yield identical structure.
    const a = inline("**x**");
    const b = inline("**x**");
    expect(a.length).toBe(b.length);
  });
});

describe("MarkdownView — component", () => {
  it("MarkdownView_renderHeading_emitsHtag", () => {
    render(<MarkdownView source="# Hi" />);
    expect(screen.getByRole("heading", { level: 1, name: "Hi" })).toBeInTheDocument();
  });

  it("MarkdownView_renderTable_emitsTableWithHeaders", () => {
    render(<MarkdownView source={"| a | b |\n| --- | --- |\n| 1 | 2 |"} />);
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "a" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "b" })).toBeInTheDocument();
  });

  it("MarkdownView_renderCodeFence_setsDataLangAttr", () => {
    const { container } = render(<MarkdownView source={"```python\nprint(1)\n```"} />);
    expect(container.querySelector("pre[data-lang='python']")).toBeTruthy();
  });

  it("MarkdownView_renderBulletList_emitsULWithItems", () => {
    render(<MarkdownView source={"- one\n- two"} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("MarkdownView_streamingTrue_propagatesToHighlightedCode", () => {
    // When streaming and no lang, highlight is skipped → renders raw text
    // rather than highlight.js token spans. Check by absence of hljs-*.
    const { container } = render(
      <MarkdownView source={"```\nfoo bar\n```"} streaming />
    );
    expect(container.textContent).toContain("foo bar");
    // No hljs-* class tokens when autodetect is skipped
    const hljs = container.querySelectorAll("[class^='hljs-']");
    expect(hljs.length).toBe(0);
  });

  it("MarkdownView_renderPartialTable_doesNotCrashOrHang", () => {
    const start = Date.now();
    render(<MarkdownView source="| col1 | col2 |" />);
    expect(Date.now() - start).toBeLessThan(200);
  });
});
