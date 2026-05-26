// RunDrawer — input editor + invoke for function/handler/workflow.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { setupFetchSpy } from "@/api/_testHarness.js";
import { useToastStore } from "@shared/ui/toastStore";
import { RunDrawer } from "./RunDrawer.jsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

let calls;
beforeEach(async () => {
  calls = setupFetchSpy();
  useToastStore.setState({ toasts: [] });
  const bridge = await import("@/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("RunDrawer", () => {
  it("closed_rendersNothing", () => {
    const { container } = render(
      <RunDrawer open={false} kind="function" entity={{ id: "fn_1" }} onClose={() => {}} />,
      { wrapper: wrap }
    );
    expect(container.querySelector(".run-drawer")).toBeNull();
  });

  it("functionKind_titleShowsTryRun", () => {
    render(
      <RunDrawer open kind="function" entity={{ id: "fn_1", name: "fn" }} onClose={() => {}} />,
      { wrapper: wrap }
    );
    expect(screen.getByText("跑 function")).toBeInTheDocument();
  });

  it("workflowKind_titleShowsTriggerWorkflow", () => {
    render(
      <RunDrawer open kind="workflow" entity={{ id: "wf_1" }} onClose={() => {}} />,
      { wrapper: wrap }
    );
    expect(screen.getByText("触发 workflow")).toBeInTheDocument();
  });

  it("handlerKind_methodSelector_showsAvailableMethods", async () => {
    render(
      <RunDrawer open kind="handler" entity={{ id: "hd_1", methods: [{ name: "do" }, { name: "undo" }] }} onClose={() => {}} />,
      { wrapper: wrap }
    );
    expect(screen.getByLabelText("方法")).toHaveTextContent("do");
    await userEvent.click(screen.getByLabelText("方法"));
    expect(screen.getByRole("option", { name: "undo" })).toBeInTheDocument();
  });

  it("invalidJSON_showsParseError", async () => {
    render(
      <RunDrawer open kind="function" entity={{ id: "fn_1" }} onClose={() => {}} />,
      { wrapper: wrap }
    );
    const ta = document.querySelector(".run-drawer-input");
    await userEvent.clear(ta);
    await userEvent.type(ta, "not json");
    await userEvent.click(screen.getByText("提交"));
    expect(screen.getByText(/JSON 不对/)).toBeInTheDocument();
  });

  it("submitFunction_postsRunEndpoint", async () => {
    render(
      <RunDrawer open kind="function" entity={{ id: "fn_1" }} onClose={() => {}} />,
      { wrapper: wrap }
    );
    const ta = document.querySelector(".run-drawer-input");
    await userEvent.clear(ta);
    await userEvent.type(ta, '{{"x":1}');
    await userEvent.click(screen.getByText("提交"));
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls[0].url).toBe("/api/v1/functions/fn_1:run");
  });

  it("escapeKey_callsOnClose", async () => {
    const onClose = vi.fn();
    render(
      <RunDrawer open kind="function" entity={{ id: "fn_1" }} onClose={onClose} />,
      { wrapper: wrap }
    );
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });
});
