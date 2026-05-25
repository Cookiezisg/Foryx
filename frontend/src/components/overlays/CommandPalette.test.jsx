// CommandPalette — open/close, fuzzy filter, keyboard nav, item action.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

vi.mock("../../api/conversations.js", () => ({
  useConversations: () => ({ data: [{ id: "cv_1", title: "My Chat" }] }),
}));
vi.mock("../../api/forge.js", () => ({
  useFunctions: () => ({ data: [{ id: "fn_1", name: "addNumbers", desc: "adds" }] }),
  useHandlers: () => ({ data: [{ id: "hd_1", name: "MyHandler" }] }),
  useWorkflows: () => ({ data: [{ id: "wf_1", name: "MyWorkflow" }] }),
}));
vi.mock("../../api/flowruns.js", () => ({
  useFlowRuns: () => ({ data: [{ id: "fr_1", workflow: "Run X" }] }),
}));

import { useUIStore } from "../../store/ui.js";
import { CommandPalette } from "./CommandPalette.jsx";

function wrap({ children }) {
  const client = new QueryClient();
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useUIStore.setState({
    cmdkOpen: true, openPanes: [], activeConv: null, activeNarrowPane: null,
    focusEntity: {},
  });
});

describe("CommandPalette", () => {
  it("closedState_rendersNothing", () => {
    useUIStore.setState({ cmdkOpen: false });
    const { container } = render(<CommandPalette />, { wrapper: wrap });
    expect(container.querySelector(".cmdk")).toBeNull();
  });

  it("openState_rendersNavItemsAndSeededEntities", () => {
    render(<CommandPalette />, { wrapper: wrap });
    expect(screen.getAllByText("对话").length).toBeGreaterThan(0);
    expect(screen.getByText("My Chat")).toBeInTheDocument();
    expect(screen.getByText("addNumbers")).toBeInTheDocument();
  });

  it("queryFilters_matchesLabelAndDesc", async () => {
    render(<CommandPalette />, { wrapper: wrap });
    const input = screen.getByPlaceholderText(/找点什么/);
    await userEvent.type(input, "MyHandler");
    expect(screen.getByText("MyHandler")).toBeInTheDocument();
    expect(screen.queryByText("对话")).toBeNull();
  });

  it("emptyMatch_showsEmptyHintMessage", async () => {
    render(<CommandPalette />, { wrapper: wrap });
    const input = screen.getByPlaceholderText(/找点什么/);
    await userEvent.type(input, "zzz-nope-zzz");
    expect(screen.getByText(/没有匹配/)).toBeInTheDocument();
  });

  it("escapeKey_closesPalette", async () => {
    render(<CommandPalette />, { wrapper: wrap });
    await userEvent.keyboard("{Escape}");
    expect(useUIStore.getState().cmdkOpen).toBe(false);
  });

  it("enterKey_runsActiveItemAction_andCloses", async () => {
    render(<CommandPalette />, { wrapper: wrap });
    await userEvent.keyboard("{Enter}");
    expect(useUIStore.getState().cmdkOpen).toBe(false);
    expect(useUIStore.getState().openPanes).toContain("chat");
  });

  it("clickItem_runsActionAndCloses", async () => {
    render(<CommandPalette />, { wrapper: wrap });
    await userEvent.click(screen.getByText("工坊"));
    expect(useUIStore.getState().openPanes).toContain("forge");
    expect(useUIStore.getState().cmdkOpen).toBe(false);
  });
});
