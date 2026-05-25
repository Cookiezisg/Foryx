// FunctionDetail — current version full view + diff view + VersionRail.
// pendingV swaps action buttons (Accept/Revert vs 试跑+AskAI).

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../api/forge.js", () => ({
  useFunction: vi.fn(),
  useFunctionVersions: vi.fn(),
  useAcceptFunction: vi.fn(),
  useRevertFunction: vi.fn(),
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

import {
  useFunction, useFunctionVersions, useAcceptFunction, useRevertFunction,
} from "../../api/forge.js";
import { useUIStore } from "../../store/ui.js";
import { FunctionDetail } from "./FunctionDetail.jsx";

const FN = { id: "fn_1", name: "addNumbers", desc: "adds", status: "ready" };

const VERSIONS_READY = [
  { id: "fv_1", label: "v1", state: "current", description: "v1 desc",
    code: "def add(a, b):\n    return a + b",
    schema: { inputs: "{a:int,b:int}", outputs: "int" },
    runtime: "python:3.11" },
];

const VERSIONS_WITH_PENDING = [
  { id: "fv_1", label: "v1", state: "current", description: "v1 desc",
    code: "def add(a, b):\n    return a + b",
    schema: { inputs: "{a:int,b:int}", outputs: "int" } },
  { id: "fv_2", label: "v2", state: "pending", description: "v2 better",
    code: "def add(a, b):\n    return a + b + 0",
    schema: { inputs: "{a:int,b:int}", outputs: "int" } },
];

beforeEach(() => {
  useUIStore.setState({ toasts: [] });
  useFunction.mockReturnValue({ data: FN });
  useFunctionVersions.mockReturnValue({ data: VERSIONS_READY });
  useAcceptFunction.mockReturnValue({ mutate: vi.fn() });
  useRevertFunction.mockReturnValue({ mutate: vi.fn() });
});

describe("FunctionDetail", () => {
  it("header_showsNameAndKindChip", () => {
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    expect(screen.getByText("addNumbers")).toBeInTheDocument();
    expect(screen.getByText("fn_1")).toBeInTheDocument();
  });

  it("readyState_showsRunButton_andAskAiTrigger", () => {
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    expect(screen.getByText("试跑")).toBeInTheDocument();
    expect(screen.getByTestId("ask-ai")).toBeInTheDocument();
    expect(screen.queryByText("接受")).toBeNull();
  });

  it("pendingVersion_showsAcceptAndRevert_hidesRun", () => {
    useFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    // Accept and Revert appear in both header and VersionRail banner — use getAllByText
    expect(screen.getAllByText("接受").length).toBeGreaterThan(0);
    expect(screen.getAllByText("还原").length).toBeGreaterThan(0);
    expect(screen.queryByText("试跑")).toBeNull();
  });

  it("backButton_callsOnBack", async () => {
    const onBack = vi.fn();
    render(<FunctionDetail forge={FN} onBack={onBack} />);
    await userEvent.click(screen.getByText(/返回/));
    expect(onBack).toHaveBeenCalled();
  });

  it("acceptClick_callsMutationWithId_andToastsOnSuccess", async () => {
    useFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const mutate = vi.fn((id, opts) => opts?.onSuccess && opts.onSuccess());
    useAcceptFunction.mockReturnValue({ mutate });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    const headerAccept = screen.getAllByText("接受")[0];
    await userEvent.click(headerAccept);
    expect(mutate).toHaveBeenCalledWith("fn_1", expect.any(Object));
    await waitFor(() => expect(useUIStore.getState().toasts[0]?.kind).toBe("success"));
  });

  it("revertClick_callsMutationWithId_andWarnToast", async () => {
    useFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const mutate = vi.fn((id, opts) => opts?.onSuccess && opts.onSuccess());
    useRevertFunction.mockReturnValue({ mutate });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    const headerRevert = screen.getAllByText("还原")[0];
    await userEvent.click(headerRevert);
    expect(mutate).toHaveBeenCalledWith("fn_1", expect.any(Object));
    await waitFor(() => expect(useUIStore.getState().toasts[0]?.kind).toBe("warn"));
  });

  it("runButton_opensRunDrawerWithFunction", async () => {
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    await userEvent.click(screen.getByText("试跑"));
    await waitFor(() => expect(screen.getByTestId("run-drawer")).toBeInTheDocument());
  });

  it("currentView_showsSchemaAndRuntime", () => {
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    expect(screen.getByText("{a:int,b:int}")).toBeInTheDocument();
    expect(screen.getByText("int")).toBeInTheDocument();
    expect(screen.getByText("python:3.11")).toBeInTheDocument();
  });

  it("noVersions_emptyState", () => {
    useFunctionVersions.mockReturnValue({ data: [] });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    expect(screen.getByText(/没有可显示的版本/)).toBeInTheDocument();
  });

  it("pendingDiff_codeChange_showsDiffSummary", async () => {
    useFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const { container } = render(<FunctionDetail forge={FN} onBack={() => {}} />);
    // pending is auto-selected as effectiveSelected → diff view rendered
    expect(container.textContent).toContain("Diff");
    expect(container.textContent).toMatch(/\d+ 处变更/);
  });
});
