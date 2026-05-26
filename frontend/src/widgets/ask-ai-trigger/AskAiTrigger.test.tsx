// AskAiTrigger — opens popover, calls iterate, jumps to chat with
// returned conversationId.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { setupFetchSpy } from "@shared/lib/testHarness";
import { usePaneStore } from "@app/model";
import { setNavigator } from "@shared/lib/navigation";
import { useToastStore } from "../../shared/ui/toastStore.ts";
import { AskAiTrigger } from "./AskAiTrigger.tsx";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

let calls: any;
beforeEach(async () => {
  calls = setupFetchSpy();
  usePaneStore.setState({ openPanes: ["forge"], activeConv: null });
  setNavigator({
    openConv: (id) => { usePaneStore.getState().setActiveConv(id); usePaneStore.getState().openPane("chat"); },
    openEntity: (pane, id) => usePaneStore.getState().openEntity(pane, id),
    openPane: (pane) => usePaneStore.getState().openPane(pane),
    setActiveDocument: (id) => { usePaneStore.getState().setActiveDocument(id); usePaneStore.getState().openPane("documents"); },
  });
  const bridge = await import("@shared/bridge/wails");
  await bridge.initBaseUrl();
});

describe("AskAiTrigger", () => {
  it("triggerButton_visible_byDefaultPopoverClosed", () => {
    render(<AskAiTrigger kind="function" entityId="fn_1" />, { wrapper: wrap });
    expect(screen.getByText(/AI · 迭代/)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/告诉 AI/)).toBeNull();
  });

  it("clickTrigger_opensPopoverWithTextarea", async () => {
    render(<AskAiTrigger kind="function" entityId="fn_1" />, { wrapper: wrap });
    await userEvent.click(screen.getByText(/AI · 迭代/));
    expect(screen.getByPlaceholderText(/告诉 AI/)).toBeInTheDocument();
  });

  it("suggestionChips_clickSubmitsThatSuggestion", async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true, status: 200,
      json: async () => ({ data: { conversationId: "cv_iter" } }),
    });
    render(<AskAiTrigger kind="function" entityId="fn_1" suggestions={["add docstring"]} />, { wrapper: wrap });
    await userEvent.click(screen.getByText(/AI · 迭代/));
    await userEvent.click(screen.getByText("add docstring"));
    await waitFor(() => expect(usePaneStore.getState().activeConv).toBe("cv_iter"));
  });

  it("submitOnEnter_buildsCorrectRequest", async () => {
    render(<AskAiTrigger kind="workflow" entityId="wf_1" />, { wrapper: wrap });
    await userEvent.click(screen.getByText(/AI · 迭代/));
    const ta = screen.getByPlaceholderText(/告诉 AI/);
    await userEvent.type(ta, "rename it{enter}");
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls[0].url).toBe("/api/v1/workflows/wf_1:iterate");
    expect(JSON.parse(calls[0].body)).toEqual({ prompt: "rename it" });
  });

  it("emptyResponse_pushesWarningToast", async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true, status: 200, json: async () => ({ data: {} }),
    });
    render(<AskAiTrigger kind="function" entityId="fn_1" />, { wrapper: wrap });
    await userEvent.click(screen.getByText(/AI · 迭代/));
    const ta = screen.getByPlaceholderText(/告诉 AI/);
    await userEvent.type(ta, "x{enter}");
    await waitFor(() => expect(useToastStore.getState().toasts.length).toBeGreaterThan(0));
    expect(useToastStore.getState().toasts[0].kind).toBe("warn");
  });
});
