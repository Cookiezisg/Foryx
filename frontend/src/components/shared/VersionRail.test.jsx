// VersionRail — version list + pending banner + collapsed mode.
// SplitDiff + CodeView also exported from same file; covered here.

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { VersionRail, SplitDiff, CodeView } from "./VersionRail.jsx";

const VERSIONS = [
  { id: "fv_1", label: "v1", summary: "first version", author: "user", createdAt: "2026-01-01T00:00:00Z" },
  { id: "fv_2", label: "v2", summary: "the pending one", createdAt: "2026-01-02T00:00:00Z" },
  { id: "fv_3", label: "v3", summary: "deployed one" },
];

describe("VersionRail", () => {
  it("rendersOneRowPerVersion", () => {
    render(<VersionRail versions={VERSIONS} currentId="fv_1" selectedId="fv_1" />);
    expect(screen.getByText("v1")).toBeInTheDocument();
    expect(screen.getByText("v2")).toBeInTheDocument();
    expect(screen.getByText("v3")).toBeInTheDocument();
  });

  it("pendingBanner_shownWhenPendingExists", () => {
    render(<VersionRail versions={VERSIONS} pendingId="fv_2" selectedId="fv_2" />);
    expect(screen.getByText(/pending 待处理/)).toBeInTheDocument();
    // appears in banner AND in row → use getAllByText
    expect(screen.getAllByText("the pending one").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("接受")).toBeInTheDocument();
    expect(screen.getByText("还原")).toBeInTheDocument();
  });

  it("acceptButton_clickFiresOnAccept", async () => {
    const onAccept = vi.fn();
    render(<VersionRail versions={VERSIONS} pendingId="fv_2" onAccept={onAccept} />);
    await userEvent.click(screen.getByText("接受"));
    expect(onAccept).toHaveBeenCalled();
  });

  it("revertButton_clickFiresOnRevert", async () => {
    const onRevert = vi.fn();
    render(<VersionRail versions={VERSIONS} pendingId="fv_2" onRevert={onRevert} />);
    await userEvent.click(screen.getByText("还原"));
    expect(onRevert).toHaveBeenCalled();
  });

  it("rowClick_callsOnSelectWithId", async () => {
    const onSelect = vi.fn();
    render(<VersionRail versions={VERSIONS} onSelect={onSelect} />);
    await userEvent.click(screen.getByText("v3"));
    expect(onSelect).toHaveBeenCalledWith("fv_3");
  });

  it("collapseToggle_switchesToCollapsedDots", async () => {
    const { container } = render(<VersionRail versions={VERSIONS} />);
    await userEvent.click(container.querySelector(".vr-collapse"));
    expect(container.querySelector(".vr-collapsed-list")).toBeTruthy();
    expect(container.querySelectorAll(".vr-collapsed-dot")).toHaveLength(3);
  });

  it("deployBar_visibleWhenShowDeployAndDifferentDeployed", () => {
    render(<VersionRail versions={VERSIONS} currentId="fv_2" deployedId="fv_3" showDeploy />);
    expect(screen.getByText("部署")).toBeInTheDocument();
  });

  it("currentBadge_visibleOnCurrentRow", () => {
    render(<VersionRail versions={VERSIONS} currentId="fv_1" />);
    expect(screen.getByText("当前")).toBeInTheDocument();
  });

  it("deployedBadge_visibleOnDeployedRow", () => {
    render(<VersionRail versions={VERSIONS} deployedId="fv_3" />);
    expect(screen.getByText("已发布")).toBeInTheDocument();
  });
});

describe("SplitDiff", () => {
  it("identicalSides_zeroAddsZeroDels", () => {
    render(<SplitDiff leftLabel="A" rightLabel="B" leftSrc="x\ny" rightSrc="x\ny" />);
    expect(screen.getByText("+0")).toBeInTheDocument();
    expect(screen.getByText("−0")).toBeInTheDocument();
  });

  it("addedLine_countsAsAdd", () => {
    render(<SplitDiff leftLabel="A" rightLabel="B" leftSrc="x" rightSrc={"x\ny"} />);
    expect(screen.getByText("+1")).toBeInTheDocument();
  });

  it("removedLine_countsAsDel", () => {
    render(<SplitDiff leftLabel="A" rightLabel="B" leftSrc={"x\ny"} rightSrc="x" />);
    expect(screen.getByText("−1")).toBeInTheDocument();
  });
});

describe("CodeView", () => {
  it("rendersLineNumbersAndTokens", () => {
    const { container } = render(<CodeView src={"def foo():\n    return 1"} />);
    expect(container.querySelector(".tok-kw")).toBeTruthy(); // 'def' keyword
    expect(container.textContent).toContain("foo");
  });

  it("commentLine_tokenisedAsCom", () => {
    const { container } = render(<CodeView src="# hi" />);
    expect(container.querySelector(".tok-com")).toBeTruthy();
  });

  it("stringLiteral_keepsContentEvenWithKeywordsInside", () => {
    const { container } = render(<CodeView src={'x = "def for in"'} />);
    expect(container.querySelector(".tok-str").textContent).toBe('"def for in"');
    // No keyword styling INSIDE the string
    const kws = container.querySelectorAll(".tok-kw");
    expect(kws).toHaveLength(0);
  });
});
