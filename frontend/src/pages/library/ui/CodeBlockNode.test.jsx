// CodeBlockNode — language picker trigger + searchable popover.
// Tiptap wrapper components are stubbed so we focus on LangPicker behaviour.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@tiptap/react", () => ({
  NodeViewWrapper: ({ children, className }) => (
    <div className={className} data-testid="nv-wrapper">{children}</div>
  ),
  NodeViewContent: ({ as: As = "code" }) => <As data-testid="nv-content" />,
}));

vi.mock("../../shared/lib/highlight/index.js", () => ({
  lowlight: {
    listLanguages: () => ["python", "javascript", "typescript", "go", "bash"],
  },
}));

import { CodeBlockNode } from "./CodeBlockNode.jsx";

function renderNode({ language = "" } = {}) {
  const updateAttributes = vi.fn();
  const node = { attrs: { language } };
  const utils = render(<CodeBlockNode node={node} updateAttributes={updateAttributes} />);
  return { updateAttributes, ...utils };
}

describe("CodeBlockNode", () => {
  it("noLanguage_triggerShowsAutoLabel", () => {
    renderNode({ language: "" });
    expect(screen.getByTitle("选择代码语言").textContent).toContain("Auto");
  });

  it("knownLanguage_triggerShowsFriendlyLabel", () => {
    renderNode({ language: "javascript" });
    expect(screen.getByTitle("选择代码语言").textContent).toContain("JavaScript");
  });

  it("unknownLanguage_triggerFallsBackToRawId", () => {
    renderNode({ language: "elixir" });
    expect(screen.getByTitle("选择代码语言").textContent).toContain("elixir");
  });

  it("popoverClosed_byDefault_noListVisible", () => {
    renderNode();
    expect(document.querySelector(".cb-lang-pop")).toBeNull();
  });

  it("clickTrigger_opensPopover_listsAutoAndLanguages", async () => {
    renderNode();
    await userEvent.click(screen.getByTitle("选择代码语言"));
    const rows = document.querySelectorAll(".cb-lang-row");
    expect(rows.length).toBeGreaterThan(1);
    expect(rows[0].textContent).toContain("Auto");
  });

  it("typeInSearch_filtersByLabelOrId", async () => {
    renderNode();
    await userEvent.click(screen.getByTitle("选择代码语言"));
    const search = document.querySelector(".cb-lang-search");
    await userEvent.type(search, "java");
    const rows = document.querySelectorAll(".cb-lang-row");
    const texts = Array.from(rows).map((r) => r.textContent);
    expect(texts.some((t) => t.includes("JavaScript"))).toBe(true);
    expect(texts.every((t) => !t.includes("Python"))).toBe(true);
  });

  it("searchNoMatch_showsEmptyHint", async () => {
    renderNode();
    await userEvent.click(screen.getByTitle("选择代码语言"));
    await userEvent.type(document.querySelector(".cb-lang-search"), "zzzz");
    expect(screen.getByText("没有匹配")).toBeInTheDocument();
  });

  it("clickRow_callsUpdateAttributes_withSelectedLanguage", async () => {
    const { updateAttributes } = renderNode();
    await userEvent.click(screen.getByTitle("选择代码语言"));
    const rows = document.querySelectorAll(".cb-lang-row");
    const goRow = Array.from(rows).find((r) => r.textContent.includes("Go"));
    await userEvent.click(goRow);
    expect(updateAttributes).toHaveBeenCalledWith({ language: "go" });
  });

  it("clickAutoRow_passesNullLanguage", async () => {
    const { updateAttributes } = renderNode({ language: "go" });
    await userEvent.click(screen.getByTitle("选择代码语言"));
    const rows = document.querySelectorAll(".cb-lang-row");
    await userEvent.click(rows[0]); // first row is Auto
    expect(updateAttributes).toHaveBeenCalledWith({ language: null });
  });

  it("currentLanguage_rowShowsCheckMark", async () => {
    renderNode({ language: "python" });
    await userEvent.click(screen.getByTitle("选择代码语言"));
    const rows = document.querySelectorAll(".cb-lang-row");
    const pyRow = Array.from(rows).find((r) => r.textContent.includes("Python"));
    expect(pyRow.querySelector("svg")).toBeTruthy();
  });

  it("enterAfterSearch_picksHighlightedRow", async () => {
    const { updateAttributes } = renderNode();
    await userEvent.click(screen.getByTitle("选择代码语言"));
    const search = document.querySelector(".cb-lang-search");
    await userEvent.type(search, "py");
    await userEvent.keyboard("{Enter}");
    expect(updateAttributes).toHaveBeenCalled();
    const arg = updateAttributes.mock.calls[0][0];
    expect(arg.language).toBe("python");
  });
});
