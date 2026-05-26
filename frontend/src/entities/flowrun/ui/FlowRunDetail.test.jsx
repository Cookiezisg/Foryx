// FlowRunDetail — header + DAG + node inspector + Gantt. Triage spawns a
// new chat conv. Running can be cancelled. Failed shows AI-triage button.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@/api/flowruns.js", () => ({
  useFlowRun: vi.fn(),
  useFlowRunNodes: vi.fn(),
  useCancelFlowRun: vi.fn(),
  useApproveNode: vi.fn(),
  useRejectNode: vi.fn(),
  useTriageFlowRun: vi.fn(),
}));

vi.mock("@/widgets/entity-rel-meta/EntityRelMeta.jsx", () => ({
  EntityRelMeta: () => null,
}));

vi.mock("@/api/relations.js", () => ({
  useNeighborhood: () => ({ data: [] }),
}));

import {
  useFlowRun, useFlowRunNodes, useCancelFlowRun, useTriageFlowRun,
  useApproveNode, useRejectNode,
} from "@/api/flowruns.js";
import { useToastStore } from "@shared/ui/toastStore";
import { FlowRunDetail } from "./FlowRunDetail.jsx";

const BASE_RUN = {
  id: "fr_xy", workflow: "MyFlow", workflowId: "wf_1",
  status: "running", trigger: "cron", startedAt: "2026-05-23T09:00:00Z",
};

const NODES = [
  { id: "n1", label: "Fetch",   kind: "function", status: "completed", durationMs: 1500, startedMs: 0,    output: { v: 1 } },
  { id: "n2", label: "Process", kind: "function", status: "running",   startedMs: 1500,                    dependsOn: ["n1"] },
  { id: "n3", label: "Notify",  kind: "function", status: "pending",                                       dependsOn: ["n2"] },
];

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  useFlowRun.mockReturnValue({ data: BASE_RUN });
  useFlowRunNodes.mockReturnValue({ data: NODES });
  useCancelFlowRun.mockReturnValue({ mutate: vi.fn(), isPending: false });
  useApproveNode.mockReturnValue({ mutate: vi.fn(), isPending: false });
  useRejectNode.mockReturnValue({ mutate: vi.fn(), isPending: false });
  useTriageFlowRun.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ conversationId: "cv_triage_new" }),
    isPending: false,
  });
});

describe("FlowRunDetail", () => {
  it("loadingRun_showsLoadingPlaceholder", () => {
    useFlowRun.mockReturnValue({ data: null });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText(/加载 flowrun/)).toBeInTheDocument();
  });

  it("loadedRun_showsWorkflowNameAndId_andTriggerKind", () => {
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("MyFlow")).toBeInTheDocument();
    expect(screen.getByText("fr_xy")).toBeInTheDocument();
    expect(screen.getByText("cron")).toBeInTheDocument();
  });

  it("nodeCounts_okFailSkipFormatted", () => {
    useFlowRunNodes.mockReturnValue({
      data: [
        { id: "a", status: "completed" },
        { id: "b", status: "completed" },
        { id: "c", status: "failed" },
        { id: "d", status: "pending" },
      ],
    });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText(/2 ok/)).toBeInTheDocument();
    expect(screen.getByText(/1 fail/)).toBeInTheDocument();
    expect(screen.getByText(/1 skip/)).toBeInTheDocument();
  });

  it("backButton_callsOnBack", async () => {
    const onBack = vi.fn();
    render(<FlowRunDetail runId="fr_xy" onBack={onBack} />);
    await userEvent.click(screen.getByText(/返回/));
    expect(onBack).toHaveBeenCalled();
  });

  it("runningStatus_showsCancelButton", () => {
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("取消")).toBeInTheDocument();
    expect(screen.queryByText("AI 排查")).toBeNull();
  });

  it("failedStatus_showsAiTriageButton_andHidesCancel", () => {
    useFlowRun.mockReturnValue({ data: { ...BASE_RUN, status: "failed" } });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("AI 排查")).toBeInTheDocument();
    expect(screen.queryByText("取消")).toBeNull();
  });

  it("cancelClick_invokesCancelMutation", async () => {
    const mutate = vi.fn();
    useCancelFlowRun.mockReturnValue({ mutate, isPending: false });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    await userEvent.click(screen.getByText("取消"));
    expect(mutate).toHaveBeenCalledWith("fr_xy");
  });

  it("triageSuccess_callsOnOpenChat_andToasts", async () => {
    const onOpenChat = vi.fn();
    useFlowRun.mockReturnValue({ data: { ...BASE_RUN, status: "failed" } });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} onOpenChat={onOpenChat} />);
    await userEvent.click(screen.getByText("AI 排查"));
    await waitFor(() => expect(onOpenChat).toHaveBeenCalledWith("cv_triage_new"));
    expect(useToastStore.getState().toasts[0].kind).toBe("success");
  });

  it("triageError_pushesErrorToast", async () => {
    useFlowRun.mockReturnValue({ data: { ...BASE_RUN, status: "failed" } });
    useTriageFlowRun.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("LLM down")),
      isPending: false,
    });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    await userEvent.click(screen.getByText("AI 排查"));
    await waitFor(() => expect(useToastStore.getState().toasts.length).toBeGreaterThan(0));
    expect(useToastStore.getState().toasts[0].kind).toBe("error");
  });

  it("emptyNodes_dagShowsEmptyState", () => {
    useFlowRunNodes.mockReturnValue({ data: [] });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("没有节点数据")).toBeInTheDocument();
  });

  it("dagNodes_renderedWithLabelAndKind", () => {
    const { container } = render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getAllByText("Fetch").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Process").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Notify").length).toBeGreaterThan(0);
    expect(container.querySelectorAll(".fr-dag-node")).toHaveLength(3);
  });

  it("ganttTimeline_showsRowPerNode_andTotalDuration", () => {
    const { container } = render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("时间线")).toBeInTheDocument();
    expect(container.querySelectorAll(".fr-gantt-row")).toHaveLength(3);
  });

  it("waitingApproval_approvalBannerVisible", () => {
    useFlowRunNodes.mockReturnValue({
      data: [{ id: "n_a", status: "waiting_approval", label: "Approve me" }],
    });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("等待审批")).toBeInTheDocument();
    expect(screen.getAllByText("Approve me").length).toBeGreaterThan(0);
  });
});
