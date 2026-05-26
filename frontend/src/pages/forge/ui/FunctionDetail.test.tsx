// FunctionDetail — current version full view + diff view + VersionRail.
// pendingV swaps action buttons (Accept/Revert vs 试跑+AskAI).

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/function", () => ({
  useFunction: vi.fn(),
  useFunctionVersions: vi.fn(),
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

import {
  useFunction, useFunctionVersions,
} from "@entities/function";
import { useForgeReview } from "@features/forge-review";
import { useToastStore } from "@shared/ui/toastStore";
import { FunctionDetail } from "./FunctionDetail.tsx";

const mockUseFunction = useFunction as any;
const mockUseFunctionVersions = useFunctionVersions as any;
const mockUseForgeReview = useForgeReview as any;

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
  useToastStore.setState({ toasts: [] });
  mockUseFunction.mockReturnValue({ data: FN });
  mockUseFunctionVersions.mockReturnValue({ data: VERSIONS_READY });
  mockUseForgeReview.mockReturnValue({ accept: vi.fn(), reject: vi.fn(), revert: vi.fn() });
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
    mockUseFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
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

  it("acceptClick_callsAcceptAction", async () => {
    mockUseFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const accept = vi.fn();
    mockUseForgeReview.mockReturnValue({ accept, reject: vi.fn(), revert: vi.fn() });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    const headerAccept = screen.getAllByText("接受")[0];
    await userEvent.click(headerAccept);
    expect(accept).toHaveBeenCalled();
  });

  it("revertClick_callsRevertAction", async () => {
    mockUseFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const revert = vi.fn();
    mockUseForgeReview.mockReturnValue({ accept: vi.fn(), reject: vi.fn(), revert });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    const headerRevert = screen.getAllByText("还原")[0];
    await userEvent.click(headerRevert);
    expect(revert).toHaveBeenCalled();
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
    mockUseFunctionVersions.mockReturnValue({ data: [] });
    render(<FunctionDetail forge={FN} onBack={() => {}} />);
    expect(screen.getByText(/没有可显示的版本/)).toBeInTheDocument();
  });

  it("pendingDiff_codeChange_showsDiffSummary", async () => {
    mockUseFunctionVersions.mockReturnValue({ data: VERSIONS_WITH_PENDING });
    const { container } = render(<FunctionDetail forge={FN} onBack={() => {}} />);
    // pending is auto-selected as effectiveSelected → diff view rendered
    expect(container.textContent).toContain("Diff");
    expect(container.textContent).toMatch(/\d+ 处变更/);
  });
});
