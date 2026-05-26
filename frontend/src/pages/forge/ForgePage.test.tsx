// ForgePage — list ↔ detail router. focusEntity probes 3 detail endpoints
// in parallel; first non-null wins (determines kind).

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("./ui/ForgeList.tsx", () => ({
  ForgeList: ({ onOpen }: { onOpen: (e: any) => void }) => (
    <div data-testid="list">
      <button onClick={() => onOpen({ id: "fn_x", kind: "function", name: "Pick" })}>
        open function
      </button>
      <button onClick={() => onOpen({ id: "hd_x", kind: "handler", name: "PickH" })}>
        open handler
      </button>
      <button onClick={() => onOpen({ id: "wf_x", kind: "workflow", name: "PickW" })}>
        open workflow
      </button>
    </div>
  ),
}));

vi.mock("@entities/function", () => ({
  useFunction: vi.fn(),
}));

vi.mock("@entities/handler", () => ({
  useHandler: vi.fn(),
}));

vi.mock("@entities/workflow", () => ({
  useWorkflow: vi.fn(),
}));

vi.mock("./ui/FunctionDetail.tsx", () => ({
  FunctionDetail: ({ forge, onBack }: { forge: any; onBack: () => void }) => (
    <div data-testid="fn-detail">
      fn-{forge.id}-{forge.name}
      <button onClick={onBack}>back</button>
    </div>
  ),
}));

vi.mock("./ui/HandlerDetail.tsx", () => ({
  HandlerDetail: ({ forge, onBack }: { forge: any; onBack: () => void }) => (
    <div data-testid="hd-detail">
      hd-{forge.id}
      <button onClick={onBack}>back</button>
    </div>
  ),
}));

vi.mock("./ui/WorkflowDetail.tsx", () => ({
  WorkflowDetail: ({ forge, onBack }: { forge: any; onBack: () => void }) => (
    <div data-testid="wf-detail">
      wf-{forge.id}
      <button onClick={onBack}>back</button>
    </div>
  ),
}));

import { useFunction } from "@entities/function";
import { useHandler } from "@entities/handler";
import { useWorkflow } from "@entities/workflow";
import { ForgePage } from "./ForgePage.tsx";

const mockUseFunction = useFunction as any;
const mockUseHandler = useHandler as any;
const mockUseWorkflow = useWorkflow as any;
const mockConsumeFocusEntity = vi.fn();

beforeEach(() => {
  mockUseFunction.mockReturnValue({ data: null });
  mockUseHandler.mockReturnValue({ data: null });
  mockUseWorkflow.mockReturnValue({ data: null });
  mockConsumeFocusEntity.mockReset();
});

describe("ForgePage", () => {
  it("noFocus_rendersList", () => {
    render(<ForgePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    expect(screen.getByTestId("list")).toBeInTheDocument();
    expect(screen.queryByTestId("fn-detail")).toBeNull();
  });

  it("clickFunctionRow_opensFunctionDetail", async () => {
    render(<ForgePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await userEvent.click(screen.getByText("open function"));
    await waitFor(() => expect(screen.getByTestId("fn-detail")).toBeInTheDocument());
    expect(screen.getByText("fn-fn_x-Pick")).toBeInTheDocument();
  });

  it("clickHandlerRow_opensHandlerDetail", async () => {
    render(<ForgePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await userEvent.click(screen.getByText("open handler"));
    await waitFor(() => expect(screen.getByTestId("hd-detail")).toBeInTheDocument());
  });

  it("clickWorkflowRow_opensWorkflowDetail", async () => {
    render(<ForgePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await userEvent.click(screen.getByText("open workflow"));
    await waitFor(() => expect(screen.getByTestId("wf-detail")).toBeInTheDocument());
  });

  it("backFromDetail_returnsToList", async () => {
    render(<ForgePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await userEvent.click(screen.getByText("open function"));
    await waitFor(() => expect(screen.getByTestId("fn-detail")).toBeInTheDocument());
    await userEvent.click(screen.getByText("back"));
    await waitFor(() => expect(screen.getByTestId("list")).toBeInTheDocument());
  });

  it("focusEntityForge_functionProbeWins_opensFunctionDetail", async () => {
    mockUseFunction.mockReturnValue({ data: { id: "fn_focus", name: "F" } });
    render(<ForgePage focusEntity={{ forge: "fn_focus" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await waitFor(() => expect(screen.getByTestId("fn-detail")).toBeInTheDocument());
    expect(mockConsumeFocusEntity).toHaveBeenCalledWith("forge");
  });

  it("focusEntityForge_handlerProbeWins_opensHandlerDetail", async () => {
    mockUseHandler.mockReturnValue({ data: { id: "hd_focus", name: "H" } });
    render(<ForgePage focusEntity={{ forge: "hd_focus" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await waitFor(() => expect(screen.getByTestId("hd-detail")).toBeInTheDocument());
  });

  it("focusEntityForge_workflowProbeWins_opensWorkflowDetail", async () => {
    mockUseWorkflow.mockReturnValue({ data: { id: "wf_focus", name: "W" } });
    render(<ForgePage focusEntity={{ forge: "wf_focus" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await waitFor(() => expect(screen.getByTestId("wf-detail")).toBeInTheDocument());
  });

  it("focusEntityForge_noProbeReturns_staysOnList", () => {
    render(<ForgePage focusEntity={{ forge: "fn_ghost" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    expect(screen.getByTestId("list")).toBeInTheDocument();
    expect(screen.queryByTestId("fn-detail")).toBeNull();
  });
});
