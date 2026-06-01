// ApprovalBanner — sticky banner over the run's PARKED approvals. Data comes
// from the inbox endpoint (GET /approvals) filtered by runId + status==="parked";
// rows approve/reject via POST /flowruns/{runId}/approvals/{nodeId}.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { useToastStore } from "../../../shared/ui/toastStore.ts";
import { ApprovalBanner } from "./ApprovalBanner.tsx";
import type { Approval } from "@entities/flowrun";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return createElement(QueryClientProvider, { client }, children);
}

function approval(p: Partial<Approval>): Approval {
  return {
    id: "ap_" + (p.nodeId || "x"),
    userId: "u_1",
    flowrunId: "fr_1",
    nodeId: "n1",
    prompt: "",
    status: "parked",
    allowReason: false,
    createdAt: "2026-06-01T00:00:00Z",
    updatedAt: "2026-06-01T00:00:00Z",
    ...p,
  };
}

interface Call { url: string; method: string; body: string }

// Route GET /approvals → the parked inbox; capture every call (POST decisions
// land here too). The inbox URL ends in "/approvals"; the decision URL ends in
// the nodeId, so endsWith cleanly distinguishes them.
function mockInbox(parked: Approval[]): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = vi.fn(async (url: string | URL, init: RequestInit = {}) => {
    const u = typeof url === "string" ? url : url.toString();
    const method = (init.method as string) || "GET";
    calls.push({ url: u, method, body: (init.body as string) ?? "" });
    if (method === "GET" && u.endsWith("/approvals")) {
      return { ok: true, status: 200, json: async () => ({ data: parked }) };
    }
    return { ok: true, status: 200, json: async () => ({ data: {} }) };
  }) as unknown as typeof fetch;
  return calls;
}

beforeEach(async () => {
  useToastStore.setState({ toasts: [] });
  const bridge = await import("@shared/bridge/wails");
  await bridge.initBaseUrl();
});

describe("ApprovalBanner", () => {
  it("noParkedApprovals_rendersNothing", async () => {
    const calls = mockInbox([]);
    const { container } = render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await waitFor(() => expect(calls.some((c) => c.url.endsWith("/approvals"))).toBe(true));
    expect(container.firstChild).toBeNull();
  });

  it("parkedApprovals_rendersBannerWithCount", async () => {
    mockInbox([
      approval({ nodeId: "n1", prompt: "Step One" }),
      approval({ nodeId: "n2", prompt: "Step Two" }),
    ]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    expect(await screen.findByText("等待审批")).toBeInTheDocument();
    expect(screen.getByText(/2 个节点需要决定/)).toBeInTheDocument();
    expect(screen.getByText("Step One")).toBeInTheDocument();
    expect(screen.getByText("Step Two")).toBeInTheDocument();
  });

  it("filtersToRunId_showsOnlyMatchingRun", async () => {
    mockInbox([
      approval({ flowrunId: "fr_other", nodeId: "n_other", prompt: "Other" }),
      approval({ flowrunId: "fr_1", nodeId: "n_mine", prompt: "Mine" }),
    ]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    expect(await screen.findByText("Mine")).toBeInTheDocument();
    expect(screen.queryByText("Other")).toBeNull();
    expect(screen.getByText(/1 个节点需要决定/)).toBeInTheDocument();
  });

  it("filtersToParkedStatus_decidedRowsHidden", async () => {
    mockInbox([
      approval({ nodeId: "n_done", prompt: "Done", status: "approved" }),
      approval({ nodeId: "n_wait", prompt: "Wait", status: "parked" }),
    ]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    expect(await screen.findByText("Wait")).toBeInTheDocument();
    expect(screen.queryByText("Done")).toBeNull();
  });

  it("approvalWithoutPrompt_fallsBackToNodeId", async () => {
    mockInbox([approval({ nodeId: "node_xyz", prompt: "" })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    expect(await screen.findByText("node_xyz")).toBeInTheDocument();
  });

  it("allowReasonTrue_showsReasonButton_thatExpandsInput", async () => {
    mockInbox([approval({ nodeId: "n1", prompt: "Step", allowReason: true })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await screen.findByText("Step");
    expect(screen.queryByPlaceholderText(/审批理由/)).toBeNull();
    await userEvent.click(screen.getByText("加理由"));
    expect(screen.getByPlaceholderText(/审批理由/)).toBeInTheDocument();
    expect(screen.getByText("收起")).toBeInTheDocument();
  });

  it("allowReasonFalse_hidesReasonButton", async () => {
    mockInbox([approval({ nodeId: "n1", prompt: "Step", allowReason: false })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await screen.findByText("Step");
    expect(screen.queryByText("加理由")).toBeNull();
  });

  it("approveClick_postsApproveDecisionWithCorrectUrl", async () => {
    const calls = mockInbox([approval({ nodeId: "n_a", prompt: "Step" })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await userEvent.click(await screen.findByText("批准"));
    let post: Call | undefined;
    await waitFor(() => {
      post = calls.find((c) => c.method === "POST");
      expect(post).toBeTruthy();
    });
    expect(post!.url).toBe("/api/v1/flowruns/fr_1/approvals/n_a");
    expect(JSON.parse(post!.body)).toEqual({ decision: "approved", reason: "" });
  });

  it("rejectClick_postsRejectDecision", async () => {
    const calls = mockInbox([approval({ nodeId: "n_b", prompt: "Step" })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await userEvent.click(await screen.findByText("拒绝"));
    let post: Call | undefined;
    await waitFor(() => {
      post = calls.find((c) => c.method === "POST");
      expect(post).toBeTruthy();
    });
    expect(post!.url).toBe("/api/v1/flowruns/fr_1/approvals/n_b");
    expect(JSON.parse(post!.body)).toEqual({ decision: "rejected", reason: "" });
  });

  it("successApprove_replacesRowWithDecidedState_andToasts", async () => {
    mockInbox([approval({ nodeId: "n_c", prompt: "Step Three" })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await userEvent.click(await screen.findByText("批准"));
    await waitFor(() => expect(screen.getByText("已批准")).toBeInTheDocument());
    expect(screen.queryByText("批准")).toBeNull();
    expect(useToastStore.getState().toasts.length).toBeGreaterThan(0);
    expect(useToastStore.getState().toasts[0].kind).toBe("success");
  });

  it("successReject_showsRejectedState_andWarnToast", async () => {
    mockInbox([approval({ nodeId: "n_d", prompt: "Step" })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await userEvent.click(await screen.findByText("拒绝"));
    await waitFor(() => expect(screen.getByText("已拒绝")).toBeInTheDocument());
    expect(useToastStore.getState().toasts[0].kind).toBe("warn");
  });

  it("reasonTyped_includedInRequestBody", async () => {
    const calls = mockInbox([approval({ nodeId: "n_r", prompt: "Step", allowReason: true })]);
    render(<ApprovalBanner runId="fr_1" />, { wrapper: wrap });
    await userEvent.click(await screen.findByText("加理由"));
    await userEvent.type(screen.getByPlaceholderText(/审批理由/), "looks safe");
    await userEvent.click(screen.getByText("批准"));
    let post: Call | undefined;
    await waitFor(() => {
      post = calls.find((c) => c.method === "POST");
      expect(post).toBeTruthy();
    });
    expect(JSON.parse(post!.body)).toEqual({ decision: "approved", reason: "looks safe" });
  });
});
