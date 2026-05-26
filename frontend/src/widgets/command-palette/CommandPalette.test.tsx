// CommandPalette — open/close, fuzzy filter, keyboard nav, item action.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

vi.mock("@entities/conversation", () => ({
  useConversations: () => ({ data: [{ id: "cv_1", title: "My Chat" }] }),
}));
vi.mock("@entities/function", () => ({
  useFunctions: () => ({ data: [{ id: "fn_1", name: "addNumbers", desc: "adds" }] }),
}));
vi.mock("@entities/handler", () => ({
  useHandlers: () => ({ data: [{ id: "hd_1", name: "MyHandler" }] }),
}));
vi.mock("@entities/workflow", () => ({
  useWorkflows: () => ({ data: [{ id: "wf_1", name: "MyWorkflow" }] }),
}));
vi.mock("@entities/flowrun", () => ({
  useFlowRuns: () => ({ data: [{ id: "fr_1", workflow: "Run X" }] }),
}));

import { usePaneStore, useOverlayStore } from "@app/model";
import { CommandPalette } from "./CommandPalette.tsx";

function makeProps(overrides = {}) {
  const pane = usePaneStore.getState();
  const overlay = useOverlayStore.getState();
  return {
    open: overlay.cmdkOpen,
    onClose: () => overlay.setCmdkOpen(false),
    onOpenPane: pane.openPane,
    onOpenEntity: pane.openEntity,
    onSetActiveConv: pane.setActiveConv,
    onOpenSettings: () => overlay.setSettingsOpen(true),
    ...overrides,
  };
}

function wrap({ children }: { children: any }) {
  const client = new QueryClient();
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useOverlayStore.setState({ cmdkOpen: true });
  usePaneStore.setState({ openPanes: [], activeConv: null, activeNarrowPane: null, focusEntity: {} });
});

describe("CommandPalette", () => {
  it("closedState_rendersNothing", () => {
    useOverlayStore.setState({ cmdkOpen: false });
    const { container } = render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    expect(container.querySelector(".cmdk")).toBeNull();
  });

  it("openState_rendersNavItemsAndSeededEntities", () => {
    render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    expect(screen.getAllByText("对话").length).toBeGreaterThan(0);
    expect(screen.getByText("My Chat")).toBeInTheDocument();
    expect(screen.getByText("addNumbers")).toBeInTheDocument();
  });

  it("queryFilters_matchesLabelAndDesc", async () => {
    render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    const input = screen.getByPlaceholderText(/找点什么/);
    await userEvent.type(input, "MyHandler");
    expect(screen.getByText("MyHandler")).toBeInTheDocument();
    expect(screen.queryByText("对话")).toBeNull();
  });

  it("emptyMatch_showsEmptyHintMessage", async () => {
    render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    const input = screen.getByPlaceholderText(/找点什么/);
    await userEvent.type(input, "zzz-nope-zzz");
    expect(screen.getByText(/没有匹配/)).toBeInTheDocument();
  });

  it("escapeKey_closesPalette", async () => {
    render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    await userEvent.keyboard("{Escape}");
    expect(useOverlayStore.getState().cmdkOpen).toBe(false);
  });

  it("enterKey_runsActiveItemAction_andCloses", async () => {
    render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    await userEvent.keyboard("{Enter}");
    expect(useOverlayStore.getState().cmdkOpen).toBe(false);
    expect(usePaneStore.getState().openPanes).toContain("chat");
  });

  it("clickItem_runsActionAndCloses", async () => {
    render(<CommandPalette {...makeProps()} />, { wrapper: wrap });
    await userEvent.click(screen.getByText("工坊"));
    expect(usePaneStore.getState().openPanes).toContain("forge");
    expect(useOverlayStore.getState().cmdkOpen).toBe(false);
  });
});
