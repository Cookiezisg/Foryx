// WorkflowDetail — read-only DAG canvas (selected != current) +
// WorkflowEditor (selected == current) + VersionRail. pendingV swaps
// header actions. CapabilityCheckPanel always present.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/workflow", () => ({
  useWorkflow: vi.fn(),
  useWorkflowVersions: vi.fn(),
  useCapabilityCheck: vi.fn(),
  useEditWorkflow: vi.fn(),
}));

vi.mock("@features/forge-review", () => ({
  useForgeReview: vi.fn(),
  useForgeBatchDelete: vi.fn(),
}));

vi.mock("@shared/model", () => ({
  useForgeProgress: (selector: (s: any) => any) => selector({ active: {} }),
}));

vi.mock("@/widgets/entity-rel-meta/EntityRelMeta.tsx", () => ({
  EntityRelMeta: (): null => null,
}));

vi.mock("./RunDrawer.tsx", () => ({
  RunDrawer: ({ open, entity }: { open: any; entity: any }) =>
    open ? <div data-testid="run-drawer">drawer-{entity?.id}</div> : null,
}));

vi.mock("@/widgets/ask-ai-trigger/AskAiTrigger.tsx", () => ({
  AskAiTrigger: ({ entityId }: { entityId: any }) => <div data-testid="ask-ai">ask-{entityId}</div>,
}));

vi.mock("./CapabilityCheckPanel.tsx", () => ({
  CapabilityCheckPanel: ({ workflowId }: { workflowId: any }) => (
    <div data-testid="cap-panel">cap-{workflowId}</div>
  ),
}));

vi.mock("@features/workflow-edit/ui/WorkflowEditor.jsx", () => ({
  WorkflowEditor: ({ workflowId, version }: { workflowId: any; version: any }) => (
    <div data-testid="wf-editor">editor-{workflowId}-{version?.id}</div>
  ),
}));

import {
  useWorkflow, useWorkflowVersions,
  useCapabilityCheck, useEditWorkflow,
} from "@entities/workflow";
import { useForgeReview } from "@features/forge-review";
import { useToastStore } from "@shared/ui/toastStore";
import { WorkflowDetail } from "./WorkflowDetail.tsx";

const mockUseWorkflow = useWorkflow as any;
const mockUseWorkflowVersions = useWorkflowVersions as any;
const mockUseForgeReview = useForgeReview as any;
const mockUseCapabilityCheck = useCapabilityCheck as any;
const mockUseEditWorkflow = useEditWorkflow as any;

const WF = { id: "wf_1", name: "Backup", desc: "nightly backup", status: "ready" };

const VERSIONS_READY = [
  { id: "wfv_1", label: "v1", state: "current",
    graph: { nodes: [{ id: "n1", kind: "trigger", x: 0, y: 0, label: "Start" }], edges: [] as any[] },
  },
];

const VERSIONS_WITH_PENDING = [
  { id: "wfv_1", label: "v1", state: "current",
    graph: { nodes: [{ id: "n1", kind: "trigger", x: 0, y: 0, label: "Start" }], edges: [] as any[] },
  },
  { id: "wfv_2", label: "v2", state: "pending",
    graph: {
      nodes: [
        { id: "n1", kind: "trigger", label: "Start" },
        { id: "n2", kind: "function", label: "Fetch" },
      ],
      edges: [{ from: "n1", to: "n2" }],
    },
  },
];

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  mockUseWorkflow.mockReturnValue({ data: WF });
  mockUseWorkflowVersions.mockReturnValue({ data: VERSIONS_READY });
  mockUseForgeReview.mockReturnValue({ accept: vi.fn(), reject: vi.fn() });
  mockUseCapabilityCheck.mockReturnValue({ mutateAsync: vi.fn(), isPending: false });
  mockUseEditWorkflow.mockReturnValue({ mutate: vi.fn() });
});

describe("WorkflowDetail", () => {
  it("header_showsNameAndId", () => {
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getByText("Backup")).toBeInTheDocument();
    expect(screen.getByText("wf_1")).toBeInTheDocument();
  });

  it("readyState_showsTriggerButton_andCapabilityPanel_andAskAi", () => {
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getByText("触发")).toBeInTheDocument();
    expect(screen.getByTestId("cap-panel")).toBeInTheDocument();
    expect(screen.getByTestId("ask-ai")).toBeInTheDocument();
  });

  it("pendingState_showsAcceptAndRevert", () => {
    mockUseWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getAllByText("接受").length).toBeGreaterThan(0);
    expect(screen.getAllByText("还原").length).toBeGreaterThan(0);
  });

  it("currentSelected_rendersWorkflowEditor", () => {
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getByTestId("wf-editor")).toBeInTheDocument();
  });

  it("pendingSelected_rendersReadOnlyDagCanvas", () => {
    mockUseWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const { container } = render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    // pending is auto-selected → read-only canvas, not editor
    expect(screen.queryByTestId("wf-editor")).toBeNull();
    expect(container.querySelector(".wf-canvas")).toBeTruthy();
    expect(container.querySelectorAll(".wf-node").length).toBe(2);
  });

  it("backButton_callsOnBack", async () => {
    const onBack = vi.fn();
    render(<WorkflowDetail forge={WF} onBack={onBack} />);
    await userEvent.click(screen.getByText(/返回/));
    expect(onBack).toHaveBeenCalled();
  });

  it("triggerClick_opensRunDrawer", async () => {
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    await userEvent.click(screen.getByText("触发"));
    await waitFor(() => expect(screen.getByTestId("run-drawer")).toBeInTheDocument());
  });

  it("acceptClick_callsAcceptAction", async () => {
    mockUseWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const accept = vi.fn();
    mockUseForgeReview.mockReturnValue({ accept, reject: vi.fn() });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    const headerAccept = screen.getAllByText("接受")[0];
    await userEvent.click(headerAccept);
    expect(accept).toHaveBeenCalled();
  });

  it("rejectClick_callsRejectAction", async () => {
    mockUseWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const reject = vi.fn();
    mockUseForgeReview.mockReturnValue({ accept: vi.fn(), reject });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    const headerRevert = screen.getAllByText("还原")[0];
    await userEvent.click(headerRevert);
    expect(reject).toHaveBeenCalled();
  });

  it("readonlyCanvas_zoomToolbar_present", () => {
    mockUseWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const { container } = render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(container.querySelector(".wf-canvas-toolbar")).toBeTruthy();
    expect(container.querySelector(".wf-zoom")).toBeTruthy();
  });

  it("readonlyCanvas_noGraph_showsEmptyState", () => {
    mockUseWorkflowVersions.mockReturnValue({
      data: [
        { id: "wfv_1", label: "v1", state: "current", graph: { nodes: [{ id: "n1", kind: "trigger" }], edges: [] } },
        { id: "wfv_2", label: "v2", state: "pending", graph: { nodes: [], edges: [] } },
      ],
    });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getByText(/没有 graph 数据/)).toBeInTheDocument();
  });
});
