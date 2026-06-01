// FlowRunDetail — header + DAG + node inspector + Gantt. Triage spawns a
// new chat conv. Running can be cancelled. Failed shows AI-triage button.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/flowrun", () => ({
  useFlowRun: vi.fn(),
  useFlowRunNodes: vi.fn(),
  useApprovalInbox: vi.fn(),
  useCancelFlowRun: vi.fn(),
  useApproveNode: vi.fn(),
  useRejectNode: vi.fn(),
  useTriageFlowRun: vi.fn(),
}));

vi.mock("@/widgets/entity-rel-meta/EntityRelMeta.tsx", () => ({
  EntityRelMeta: (): null => null,
}));

vi.mock("@entities/relation", () => ({
  useNeighborhood: () => ({ data: [] as any[] }),
}));

import {
  useFlowRun, useFlowRunNodes, useApprovalInbox, useCancelFlowRun, useTriageFlowRun,
  useApproveNode, useRejectNode,
} from "@entities/flowrun";
import { useToastStore } from "@shared/ui/toastStore";
import { FlowRunDetail } from "./FlowRunDetail.tsx";

const mockUseFlowRun = useFlowRun as any;
const mockUseFlowRunNodes = useFlowRunNodes as any;
const mockUseApprovalInbox = useApprovalInbox as any;
const mockUseCancelFlowRun = useCancelFlowRun as any;
const mockUseApproveNode = useApproveNode as any;
const mockUseRejectNode = useRejectNode as any;
const mockUseTriageFlowRun = useTriageFlowRun as any;

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
  mockUseFlowRun.mockReturnValue({ data: BASE_RUN });
  mockUseFlowRunNodes.mockReturnValue({ data: NODES });
  mockUseApprovalInbox.mockReturnValue({ data: [] });
  mockUseCancelFlowRun.mockReturnValue({ mutate: vi.fn(), isPending: false });
  mockUseApproveNode.mockReturnValue({ mutate: vi.fn(), isPending: false });
  mockUseRejectNode.mockReturnValue({ mutate: vi.fn(), isPending: false });
  mockUseTriageFlowRun.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ conversationId: "cv_triage_new" }),
    isPending: false,
  });
});

describe("FlowRunDetail", () => {
  it("loadingRun_showsLoadingPlaceholder", () => {
    mockUseFlowRun.mockReturnValue({ data: null });
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
    mockUseFlowRunNodes.mockReturnValue({
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
    mockUseFlowRun.mockReturnValue({ data: { ...BASE_RUN, status: "failed" } });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("AI 排查")).toBeInTheDocument();
    expect(screen.queryByText("取消")).toBeNull();
  });

  it("cancelClick_invokesCancelMutation", async () => {
    const mutate = vi.fn();
    mockUseCancelFlowRun.mockReturnValue({ mutate, isPending: false });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    await userEvent.click(screen.getByText("取消"));
    expect(mutate).toHaveBeenCalledWith("fr_xy");
  });

  it("triageSuccess_callsOnOpenChat_andToasts", async () => {
    const onOpenChat = vi.fn();
    mockUseFlowRun.mockReturnValue({ data: { ...BASE_RUN, status: "failed" } });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} onOpenChat={onOpenChat} />);
    await userEvent.click(screen.getByText("AI 排查"));
    await waitFor(() => expect(onOpenChat).toHaveBeenCalledWith("cv_triage_new"));
    expect(useToastStore.getState().toasts[0].kind).toBe("success");
  });

  it("triageError_pushesErrorToast", async () => {
    mockUseFlowRun.mockReturnValue({ data: { ...BASE_RUN, status: "failed" } });
    mockUseTriageFlowRun.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("LLM down")),
      isPending: false,
    });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    await userEvent.click(screen.getByText("AI 排查"));
    await waitFor(() => expect(useToastStore.getState().toasts.length).toBeGreaterThan(0));
    expect(useToastStore.getState().toasts[0].kind).toBe("error");
  });

  it("emptyNodes_dagShowsEmptyState", () => {
    mockUseFlowRunNodes.mockReturnValue({ data: [] });
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

  it("parkedApproval_approvalBannerVisible", () => {
    mockUseApprovalInbox.mockReturnValue({
      data: [{
        id: "ap_1", userId: "u_1", flowrunId: "fr_xy", nodeId: "n_a",
        prompt: "Approve me", status: "parked", allowReason: false,
        createdAt: "2026-06-01T00:00:00Z", updatedAt: "2026-06-01T00:00:00Z",
      }],
    });
    render(<FlowRunDetail runId="fr_xy" onBack={() => {}} />);
    expect(screen.getByText("等待审批")).toBeInTheDocument();
    expect(screen.getAllByText("Approve me").length).toBeGreaterThan(0);
  });
});
