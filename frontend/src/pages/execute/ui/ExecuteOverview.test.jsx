// ExecuteOverview — KPI strip + tabs (runs/approvals/triggers) + flowrun
// table with search + status filter. Data via useFlowRuns().

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../../api/flowruns.js", () => ({
  useFlowRuns: vi.fn(),
  useApproveNode: () => ({ mutate: vi.fn(), isPending: false }),
  useRejectNode: () => ({ mutate: vi.fn(), isPending: false }),
}));

import { useFlowRuns } from "../../../api/flowruns.js";
import { useToastStore } from "../../../shared/ui/toastStore.ts";
import { ExecuteOverview } from "./ExecuteOverview.jsx";

const SAMPLE = [
  {
    id: "fr_run1", workflow: "DailyReport", status: "running",
    trigger: "cron", startedAt: "2026-05-23T12:00:00Z",
    nodes: { done: 2, total: 5 }, durationMs: 1500,
  },
  {
    id: "fr_run2", workflow: "BackupJob", status: "completed",
    trigger: "manual", startedAt: "2026-05-23T11:00:00Z",
    nodes: { done: 4, total: 4 }, durationMs: 8000,
  },
  {
    id: "fr_run3", workflow: "DataSync", status: "failed",
    trigger: "webhook", startedAt: "2026-05-23T10:00:00Z",
    nodes: { done: 1, total: 3 }, durationMs: 500,
  },
  {
    id: "fr_run4", workflow: "ApprovalFlow", status: "waiting_approval",
    trigger: "manual", startedAt: "2026-05-23T09:00:00Z",
    nodes: { done: 2, total: 4 }, pausedNodeId: "node_appr",
  },
];

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  useFlowRuns.mockReturnValue({ data: SAMPLE, isLoading: false });
});

describe("ExecuteOverview", () => {
  it("header_showsExecuteTitle", () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    expect(screen.getByText("执行")).toBeInTheDocument();
    expect(screen.getByText(/运行历史/)).toBeInTheDocument();
  });

  it("kpiStrip_showsCounts_andSuccessRate", () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    expect(screen.getByText("运行总数")).toBeInTheDocument();
    expect(screen.getByText("25% 成功率")).toBeInTheDocument(); // 1/4 completed
    expect(screen.getByText("需关注")).toBeInTheDocument();
  });

  it("emptyData_kpiStripShowsZero_andNoTableRows", () => {
    useFlowRuns.mockReturnValue({ data: [], isLoading: false });
    render(<ExecuteOverview onOpen={() => {}} />);
    expect(screen.getByText("0% 成功率")).toBeInTheDocument();
    expect(screen.getByText("没有匹配的运行")).toBeInTheDocument();
  });

  it("loading_showsLoadingPlaceholder", () => {
    useFlowRuns.mockReturnValue({ data: [], isLoading: true });
    render(<ExecuteOverview onOpen={() => {}} />);
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("runsTab_showsAllRunsByDefault", () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    expect(screen.getByText("DailyReport")).toBeInTheDocument();
    expect(screen.getByText("BackupJob")).toBeInTheDocument();
    expect(screen.getByText("DataSync")).toBeInTheDocument();
    expect(screen.getByText("ApprovalFlow")).toBeInTheDocument();
  });

  it("searchInput_filtersRowsByWorkflowName", async () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    const input = screen.getByPlaceholderText(/搜 workflow/);
    await userEvent.type(input, "Backup");
    expect(screen.getByText("BackupJob")).toBeInTheDocument();
    expect(screen.queryByText("DailyReport")).toBeNull();
  });

  it("searchInput_filtersByRunId", async () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    await userEvent.type(screen.getByPlaceholderText(/搜 workflow/), "fr_run3");
    expect(screen.getByText("DataSync")).toBeInTheDocument();
    expect(screen.queryByText("DailyReport")).toBeNull();
  });

  it("statusFilter_failedOnly_showsOnlyFailedRow", async () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: "失败" }));
    expect(screen.getByText("DataSync")).toBeInTheDocument();
    expect(screen.queryByText("DailyReport")).toBeNull();
  });

  it("clearFilterButton_visibleAfterFiltering_andResets", async () => {
    render(<ExecuteOverview onOpen={() => {}} />);
    await userEvent.type(screen.getByPlaceholderText(/搜 workflow/), "Backup");
    expect(screen.getByText(/清除筛选/)).toBeInTheDocument();
    await userEvent.click(screen.getByText(/清除筛选/));
    expect(screen.getByText("DailyReport")).toBeInTheDocument();
  });

  it("rowClick_callsOnOpenWithFullRun", async () => {
    const onOpen = vi.fn();
    render(<ExecuteOverview onOpen={onOpen} />);
    await userEvent.click(screen.getByText("DailyReport"));
    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ id: "fr_run1" }));
  });

  it("approvalsTab_showsCardWithWaitingRun", async () => {
    const { container } = render(<ExecuteOverview onOpen={() => {}} />);
    const approvalsTab = container.querySelector(".page-tabs .page-tab:nth-of-type(2)");
    await userEvent.click(approvalsTab);
    expect(screen.getByText("ApprovalFlow")).toBeInTheDocument();
    expect(screen.getByText("批准并继续")).toBeInTheDocument();
  });

  it("approvalsTab_empty_showsEmptyState", async () => {
    useFlowRuns.mockReturnValue({
      data: SAMPLE.filter((f) => f.status !== "waiting_approval"),
      isLoading: false,
    });
    const { container } = render(<ExecuteOverview onOpen={() => {}} />);
    const approvalsTab = container.querySelector(".page-tabs .page-tab:nth-of-type(2)");
    await userEvent.click(approvalsTab);
    expect(screen.getByText("没有待批准的任务")).toBeInTheDocument();
  });

  it("triggersTab_listsAllSupportedTriggerKinds", async () => {
    const { container } = render(<ExecuteOverview onOpen={() => {}} />);
    const triggersTab = container.querySelector(".page-tabs .page-tab:nth-of-type(3)");
    await userEvent.click(triggersTab);
    expect(screen.getByText("Cron 定时")).toBeInTheDocument();
    expect(screen.getByText("文件触发")).toBeInTheDocument();
    expect(screen.getByText("Webhook")).toBeInTheDocument();
    expect(screen.getByText("手动入口")).toBeInTheDocument();
  });
});
