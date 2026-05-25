// WorkflowDetail — read-only DAG canvas (selected != current) +
// WorkflowEditor (selected == current) + VersionRail. pendingV swaps
// header actions. CapabilityCheckPanel always present.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../api/forge.js", () => ({
  useWorkflow: vi.fn(),
  useWorkflowVersions: vi.fn(),
  useAcceptWorkflow: vi.fn(),
  useRejectWorkflow: vi.fn(),
  useCapabilityCheck: vi.fn(),
  useEditWorkflow: vi.fn(),
}));

vi.mock("../../sse/useForge.js", () => ({
  useForgeProgress: (selector) => selector({ active: {} }),
}));

vi.mock("../../components/shared/EntityRelMeta.jsx", () => ({
  EntityRelMeta: () => null,
}));

vi.mock("../../components/overlays/RunDrawer.jsx", () => ({
  RunDrawer: ({ open, entity }) =>
    open ? <div data-testid="run-drawer">drawer-{entity?.id}</div> : null,
}));

vi.mock("../../components/shared/AskAiTrigger.jsx", () => ({
  AskAiTrigger: ({ entityId }) => <div data-testid="ask-ai">ask-{entityId}</div>,
}));

vi.mock("./CapabilityCheckPanel.jsx", () => ({
  CapabilityCheckPanel: ({ workflowId }) => (
    <div data-testid="cap-panel">cap-{workflowId}</div>
  ),
}));

vi.mock("./WorkflowEditor.jsx", () => ({
  WorkflowEditor: ({ workflowId, version }) => (
    <div data-testid="wf-editor">editor-{workflowId}-{version?.id}</div>
  ),
}));

import {
  useWorkflow, useWorkflowVersions, useAcceptWorkflow, useRejectWorkflow,
  useCapabilityCheck, useEditWorkflow,
} from "../../api/forge.js";
import { useUIStore } from "../../store/ui.js";
import { WorkflowDetail } from "./WorkflowDetail.jsx";

const WF = { id: "wf_1", name: "Backup", desc: "nightly backup", status: "ready" };

const VERSIONS_READY = [
  { id: "wfv_1", label: "v1", state: "current",
    graph: { nodes: [{ id: "n1", kind: "trigger", x: 0, y: 0, label: "Start" }], edges: [] },
  },
];

const VERSIONS_WITH_PENDING = [
  { id: "wfv_1", label: "v1", state: "current",
    graph: { nodes: [{ id: "n1", kind: "trigger", x: 0, y: 0, label: "Start" }], edges: [] },
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
  useUIStore.setState({ toasts: [] });
  useWorkflow.mockReturnValue({ data: WF });
  useWorkflowVersions.mockReturnValue({ data: VERSIONS_READY });
  useAcceptWorkflow.mockReturnValue({ mutate: vi.fn() });
  useRejectWorkflow.mockReturnValue({ mutate: vi.fn() });
  useCapabilityCheck.mockReturnValue({ mutateAsync: vi.fn(), isPending: false });
  useEditWorkflow.mockReturnValue({ mutate: vi.fn() });
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
    useWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getAllByText("接受").length).toBeGreaterThan(0);
    expect(screen.getAllByText("还原").length).toBeGreaterThan(0);
  });

  it("currentSelected_rendersWorkflowEditor", () => {
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getByTestId("wf-editor")).toBeInTheDocument();
  });

  it("pendingSelected_rendersReadOnlyDagCanvas", () => {
    useWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
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

  it("acceptClick_callsAcceptMutationWithId_andToastsSuccess", async () => {
    useWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const mutate = vi.fn((id, opts) => opts?.onSuccess && opts.onSuccess());
    useAcceptWorkflow.mockReturnValue({ mutate });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    const headerAccept = screen.getAllByText("接受")[0];
    await userEvent.click(headerAccept);
    expect(mutate).toHaveBeenCalledWith("wf_1", expect.any(Object));
    await waitFor(() => expect(useUIStore.getState().toasts[0]?.kind).toBe("success"));
  });

  it("revertClick_callsRejectMutationWithId_andToastsWarn", async () => {
    useWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const mutate = vi.fn((id, opts) => opts?.onSuccess && opts.onSuccess());
    useRejectWorkflow.mockReturnValue({ mutate });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    const headerRevert = screen.getAllByText("还原")[0];
    await userEvent.click(headerRevert);
    expect(mutate).toHaveBeenCalledWith("wf_1", expect.any(Object));
    await waitFor(() => expect(useUIStore.getState().toasts[0]?.kind).toBe("warn"));
  });

  it("readonlyCanvas_zoomToolbar_present", () => {
    useWorkflowVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const { container } = render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(container.querySelector(".wf-canvas-toolbar")).toBeTruthy();
    expect(container.querySelector(".wf-zoom")).toBeTruthy();
  });

  it("readonlyCanvas_noGraph_showsEmptyState", () => {
    useWorkflowVersions.mockReturnValue({
      data: [
        { id: "wfv_1", label: "v1", state: "current", graph: { nodes: [{ id: "n1", kind: "trigger" }], edges: [] } },
        { id: "wfv_2", label: "v2", state: "pending", graph: { nodes: [], edges: [] } },
      ],
    });
    render(<WorkflowDetail forge={WF} onBack={() => {}} />);
    expect(screen.getByText(/没有 graph 数据/)).toBeInTheDocument();
  });
});
