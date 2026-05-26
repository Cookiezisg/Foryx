// ExecutePage — router between ExecuteOverview list and FlowRunDetail.
// Honours focusEntity.execute by probing useFlowRun.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

vi.mock("./ui/ExecuteOverview.tsx", () => ({
  ExecuteOverview: ({ onOpen }: { onOpen: any }) => (
    <div data-testid="overview">
      <button onClick={() => onOpen({ id: "fr_clicked" })}>open run</button>
    </div>
  ),
}));

vi.mock("@entities/flowrun", () => ({
  useFlowRun: vi.fn(),
}));

vi.mock("./ui/FlowRunDetail.tsx", () => ({
  FlowRunDetail: ({ runId, onBack }: { runId: any; onBack: any }) => (
    <div data-testid="detail">
      <span>detail-{runId}</span>
      <button onClick={onBack}>back</button>
    </div>
  ),
}));

import userEvent from "@testing-library/user-event";
import { useFlowRun } from "@entities/flowrun";
import { ExecutePage } from "./ExecutePage.tsx";

const mockUseFlowRun = useFlowRun as any;
const mockConsumeFocusEntity = vi.fn();

beforeEach(() => {
  mockUseFlowRun.mockReturnValue({ data: null });
  mockConsumeFocusEntity.mockReset();
});

describe("ExecutePage", () => {
  it("noOpenRun_rendersOverview", () => {
    render(<ExecutePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    expect(screen.getByTestId("overview")).toBeInTheDocument();
    expect(screen.queryByTestId("detail")).toBeNull();
  });

  it("openRunClick_switchesToDetail", async () => {
    render(<ExecutePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await userEvent.click(screen.getByText("open run"));
    await waitFor(() => expect(screen.getByTestId("detail")).toBeInTheDocument());
    expect(screen.getByText("detail-fr_clicked")).toBeInTheDocument();
  });

  it("backFromDetail_returnsToOverview", async () => {
    render(<ExecutePage focusEntity={{}} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await userEvent.click(screen.getByText("open run"));
    await waitFor(() => expect(screen.getByTestId("detail")).toBeInTheDocument());
    await userEvent.click(screen.getByText("back"));
    await waitFor(() => expect(screen.getByTestId("overview")).toBeInTheDocument());
  });

  it("focusEntityExecute_setBeforeMount_probesAndOpensDetail", async () => {
    mockUseFlowRun.mockReturnValue({ data: { id: "fr_focus" } });
    render(<ExecutePage focusEntity={{ execute: "fr_focus" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await waitFor(() => expect(screen.getByText("detail-fr_focus")).toBeInTheDocument());
    expect(mockConsumeFocusEntity).toHaveBeenCalledWith("execute");
  });

  it("focusEntityExecute_probeNotResolved_staysOnOverview", () => {
    mockUseFlowRun.mockReturnValue({ data: null });
    render(<ExecutePage focusEntity={{ execute: "fr_missing" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    expect(screen.getByTestId("overview")).toBeInTheDocument();
    expect(screen.queryByTestId("detail")).toBeNull();
  });
});
