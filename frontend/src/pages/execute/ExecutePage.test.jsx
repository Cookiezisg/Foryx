// ExecutePage — router between ExecuteOverview list and FlowRunDetail.
// Honours focusEntity.execute by probing useFlowRun.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

vi.mock("./ui/ExecuteOverview.jsx", () => ({
  ExecuteOverview: ({ onOpen }) => (
    <div data-testid="overview">
      <button onClick={() => onOpen({ id: "fr_clicked" })}>open run</button>
    </div>
  ),
}));

vi.mock("@entities/flowrun", () => ({
  FlowRunDetail: ({ runId, onBack }) => (
    <div data-testid="detail">
      <span>detail-{runId}</span>
      <button onClick={onBack}>back</button>
    </div>
  ),
}));

vi.mock("@/api/flowruns.js", () => ({
  useFlowRun: vi.fn(),
}));

import userEvent from "@testing-library/user-event";
import { useFlowRun } from "@/api/flowruns.js";
import { ExecutePage } from "./ExecutePage.jsx";

const mockConsumeFocusEntity = vi.fn();

beforeEach(() => {
  useFlowRun.mockReturnValue({ data: null });
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
    useFlowRun.mockReturnValue({ data: { id: "fr_focus" } });
    render(<ExecutePage focusEntity={{ execute: "fr_focus" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    await waitFor(() => expect(screen.getByText("detail-fr_focus")).toBeInTheDocument());
    expect(mockConsumeFocusEntity).toHaveBeenCalledWith("execute");
  });

  it("focusEntityExecute_probeNotResolved_staysOnOverview", () => {
    useFlowRun.mockReturnValue({ data: null });
    render(<ExecutePage focusEntity={{ execute: "fr_missing" }} onConsumeFocusEntity={mockConsumeFocusEntity} />);
    expect(screen.getByTestId("overview")).toBeInTheDocument();
    expect(screen.queryByTestId("detail")).toBeNull();
  });
});
