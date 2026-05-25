// ForgeList — trinity (function/handler/workflow) merged table with tabs,
// search, multi-select, and per-row action menu. Sorts by updatedAt desc.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../api/forge.js", () => ({
  useFunctions: vi.fn(),
  useHandlers: vi.fn(),
  useWorkflows: vi.fn(),
  useDeleteFunction: () => ({ mutate: vi.fn() }),
  useDeleteHandler: () => ({ mutate: vi.fn() }),
  useDeleteWorkflow: () => ({ mutate: vi.fn() }),
}));

vi.mock("../../sse/useForge.js", () => ({
  useForgeProgress: (selector) => selector({ active: {} }),
}));

vi.mock("../../components/overlays/RunDrawer.jsx", () => ({
  RunDrawer: ({ open, kind, entity }) =>
    open ? <div data-testid="run-drawer">drawer-{kind}-{entity?.id}</div> : null,
}));

import { useFunctions, useHandlers, useWorkflows } from "../../api/forge.js";
import { useUIStore } from "../../store/ui.js";
import { ForgeList } from "./ForgeList.jsx";

beforeEach(() => {
  useUIStore.setState({ toasts: [] });
  useFunctions.mockReturnValue({
    data: [
      { id: "fn_1", name: "addNumbers", desc: "adds",   updatedAt: "2026-05-23T10:00:00Z", status: "ready" },
      { id: "fn_2", name: "subtract",   desc: "subs",   updatedAt: "2026-05-23T11:00:00Z", status: "pending" },
    ],
  });
  useHandlers.mockReturnValue({
    data: [
      { id: "hd_1", name: "Slack", desc: "slack handler", updatedAt: "2026-05-23T12:00:00Z" },
    ],
  });
  useWorkflows.mockReturnValue({
    data: [
      { id: "wf_1", name: "Backup", desc: "nightly", updatedAt: "2026-05-23T09:00:00Z" },
    ],
  });
});

describe("ForgeList", () => {
  it("renders_threeKindRowsMerged", () => {
    render(<ForgeList onOpen={() => {}} />);
    expect(screen.getByText("addNumbers")).toBeInTheDocument();
    expect(screen.getByText("subtract")).toBeInTheDocument();
    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("Backup")).toBeInTheDocument();
  });

  it("tabCounts_reflectEachKind", () => {
    render(<ForgeList onOpen={() => {}} />);
    const tabs = screen.getAllByRole("button");
    const fnTab = tabs.find((b) => b.textContent.startsWith("Functions"));
    const hdTab = tabs.find((b) => b.textContent.startsWith("Handlers"));
    const wfTab = tabs.find((b) => b.textContent.startsWith("Workflows"));
    expect(fnTab.textContent).toContain("2");
    expect(hdTab.textContent).toContain("1");
    expect(wfTab.textContent).toContain("1");
  });

  it("clickFunctionsTab_hidesHandlerAndWorkflowRows", async () => {
    render(<ForgeList onOpen={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: /Functions/ }));
    expect(screen.getByText("addNumbers")).toBeInTheDocument();
    expect(screen.queryByText("Slack")).toBeNull();
    expect(screen.queryByText("Backup")).toBeNull();
  });

  it("searchInput_filtersByName", async () => {
    render(<ForgeList onOpen={() => {}} />);
    await userEvent.type(screen.getByPlaceholderText(/搜索 forge/), "Backup");
    expect(screen.getByText("Backup")).toBeInTheDocument();
    expect(screen.queryByText("addNumbers")).toBeNull();
  });

  it("searchInput_filtersByDesc", async () => {
    render(<ForgeList onOpen={() => {}} />);
    await userEvent.type(screen.getByPlaceholderText(/搜索 forge/), "slack");
    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.queryByText("addNumbers")).toBeNull();
  });

  it("emptyAfterFilter_showsEmptyStateMessage", async () => {
    render(<ForgeList onOpen={() => {}} />);
    await userEvent.type(screen.getByPlaceholderText(/搜索 forge/), "nonexistent-zzz");
    expect(screen.getByText(/还没有.*工坊产物/)).toBeInTheDocument();
  });

  it("rowsSorted_byUpdatedAtDesc", () => {
    const { container } = render(<ForgeList onOpen={() => {}} />);
    const nameCells = container.querySelectorAll("tbody tr .cell-strong");
    expect(nameCells[0].textContent).toBe("Slack");       // 12:00
    expect(nameCells[1].textContent).toBe("subtract");    // 11:00
    expect(nameCells[2].textContent).toBe("addNumbers");  // 10:00
    expect(nameCells[3].textContent).toBe("Backup");      // 09:00
  });

  it("rowNameClick_callsOnOpenWithFullEntity", async () => {
    const onOpen = vi.fn();
    render(<ForgeList onOpen={onOpen} />);
    await userEvent.click(screen.getByText("addNumbers"));
    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({
      id: "fn_1", kind: "function",
    }));
  });

  it("rowCheckboxToggle_revealsBatchBar", async () => {
    const { container } = render(<ForgeList onOpen={() => {}} />);
    const firstSel = container.querySelectorAll(".row-select")[1]; // 0 is header
    await userEvent.click(firstSel);
    expect(screen.getByText(/已选 1 项/)).toBeInTheDocument();
  });

  it("newButton_pushesInfoToast_explainingChatFlow", async () => {
    render(<ForgeList onOpen={() => {}} />);
    await userEvent.click(screen.getByText("新建"));
    const t = useUIStore.getState().toasts[0];
    expect(t.kind).toBe("info");
    expect(t.title).toBe("在对话里造");
  });

  it("noData_emptyStateShown", () => {
    useFunctions.mockReturnValue({ data: [] });
    useHandlers.mockReturnValue({ data: [] });
    useWorkflows.mockReturnValue({ data: [] });
    render(<ForgeList onOpen={() => {}} />);
    expect(screen.getByText(/还没有.*工坊产物/)).toBeInTheDocument();
  });
});
