// ApprovalBanner — sticky banner over waiting_approval nodes with
// approve/reject + reason expand. Mutations hit /flowruns/{runId}/approvals.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { setupFetchSpy } from "../../../api/_testHarness.js";
import { useToastStore } from "../../../shared/ui/toastStore.ts";
import { ApprovalBanner } from "./ApprovalBanner.jsx";

function wrap({ children }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return createElement(QueryClientProvider, { client }, children);
}

let calls;
beforeEach(async () => {
  calls = setupFetchSpy();
  useToastStore.setState({ toasts: [] });
  const bridge = await import("../../../bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("ApprovalBanner", () => {
  it("noPendingNodes_rendersNothing", () => {
    const { container } = render(
      <ApprovalBanner runId="fr_1" nodes={[{ id: "n1", status: "completed" }]} />,
      { wrapper: wrap }
    );
    expect(container.firstChild).toBeNull();
  });

  it("emptyNodes_rendersNothing", () => {
    const { container } = render(
      <ApprovalBanner runId="fr_1" nodes={[]} />,
      { wrapper: wrap }
    );
    expect(container.firstChild).toBeNull();
  });

  it("undefinedNodes_doesNotThrow_rendersNothing", () => {
    const { container } = render(
      <ApprovalBanner runId="fr_1" nodes={undefined} />,
      { wrapper: wrap }
    );
    expect(container.firstChild).toBeNull();
  });

  it("waitingApprovalNodes_rendersBannerWithCount", () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[
          { id: "n1", status: "waiting_approval", label: "Step One" },
          { id: "n2", status: "waiting_approval", label: "Step Two" },
        ]}
      />,
      { wrapper: wrap }
    );
    expect(screen.getByText("等待审批")).toBeInTheDocument();
    expect(screen.getByText(/2 个节点需要决定/)).toBeInTheDocument();
    expect(screen.getByText("Step One")).toBeInTheDocument();
    expect(screen.getByText("Step Two")).toBeInTheDocument();
  });

  it("waitingStatusAliases_alsoCountAsPending", () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[
          { id: "a", status: "waiting", label: "W" },
          { id: "b", status: "wait", label: "X" },
        ]}
      />,
      { wrapper: wrap }
    );
    expect(screen.getByText(/2 个节点需要决定/)).toBeInTheDocument();
  });

  it("nodeKind_renderedAsChip", () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n1", status: "waiting_approval", label: "L", kind: "approval" }]}
      />,
      { wrapper: wrap }
    );
    expect(screen.getByText("approval")).toBeInTheDocument();
  });

  it("addReasonButton_expandsInputField", async () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n1", status: "waiting_approval", label: "Step" }]}
      />,
      { wrapper: wrap }
    );
    expect(screen.queryByPlaceholderText(/审批理由/)).toBeNull();
    await userEvent.click(screen.getByText("加理由"));
    expect(screen.getByPlaceholderText(/审批理由/)).toBeInTheDocument();
    expect(screen.getByText("收起")).toBeInTheDocument();
  });

  it("approveClick_postsApproveDecisionWithCorrectUrl", async () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n_a", status: "waiting_approval", label: "Step" }]}
      />,
      { wrapper: wrap }
    );
    await userEvent.click(screen.getByText("批准"));
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls[0].url).toBe("/api/v1/flowruns/fr_1/approvals/n_a");
    expect(calls[0].method).toBe("POST");
    expect(JSON.parse(calls[0].body)).toEqual({ decision: "approve", reason: "" });
  });

  it("rejectClick_postsRejectDecision", async () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n_b", status: "waiting_approval", label: "Step" }]}
      />,
      { wrapper: wrap }
    );
    await userEvent.click(screen.getByText("拒绝"));
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls[0].url).toBe("/api/v1/flowruns/fr_1/approvals/n_b");
    expect(JSON.parse(calls[0].body)).toEqual({ decision: "reject", reason: "" });
  });

  it("successApprove_replacesRowWithDecidedState_andToasts", async () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n_c", status: "waiting_approval", label: "Step Three" }]}
      />,
      { wrapper: wrap }
    );
    await userEvent.click(screen.getByText("批准"));
    await waitFor(() => expect(screen.getByText("已批准")).toBeInTheDocument());
    expect(screen.queryByText("批准")).toBeNull();
    expect(useToastStore.getState().toasts.length).toBeGreaterThan(0);
    expect(useToastStore.getState().toasts[0].kind).toBe("success");
  });

  it("successReject_showsRejectedState_andWarnToast", async () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n_d", status: "waiting_approval", label: "Step" }]}
      />,
      { wrapper: wrap }
    );
    await userEvent.click(screen.getByText("拒绝"));
    await waitFor(() => expect(screen.getByText("已拒绝")).toBeInTheDocument());
    expect(useToastStore.getState().toasts[0].kind).toBe("warn");
  });

  it("nodeWithoutLabel_fallsBackToId", () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "node_xyz", status: "waiting_approval" }]}
      />,
      { wrapper: wrap }
    );
    expect(screen.getByText("node_xyz")).toBeInTheDocument();
  });

  it("reasonTyped_includedInRequestBody", async () => {
    render(
      <ApprovalBanner
        runId="fr_1"
        nodes={[{ id: "n_r", status: "waiting_approval", label: "Step" }]}
      />,
      { wrapper: wrap }
    );
    await userEvent.click(screen.getByText("加理由"));
    await userEvent.type(screen.getByPlaceholderText(/审批理由/), "looks safe");
    await userEvent.click(screen.getByText("批准"));
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(JSON.parse(calls[0].body)).toEqual({ decision: "approve", reason: "looks safe" });
  });
});
