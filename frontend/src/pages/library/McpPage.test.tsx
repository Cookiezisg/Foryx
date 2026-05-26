// McpPage — server list + status badge + reconnect / remove + toast.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/mcp", () => ({
  useMcpServers: vi.fn(),
  useReconnectMcp: vi.fn(),
  useRemoveMcp: vi.fn(),
}));

vi.mock("../../shared/ui/RelTime.tsx", () => ({
  RelTime: ({ ts }: { ts: any }) => <span data-testid="reltime">{ts}</span>,
}));

import { useMcpServers, useReconnectMcp, useRemoveMcp } from "@entities/mcp";
import { useToastStore } from "../../shared/ui/toastStore.ts";
import { McpPage } from "./McpPage.tsx";

const mockUseMcpServers = useMcpServers as any;
const mockUseReconnectMcp = useReconnectMcp as any;
const mockUseRemoveMcp = useRemoveMcp as any;

const SERVERS = [
  { name: "github",  status: "ready",      tools: [{}, {}], totalCalls: 12, totalFailures: 0, consecutiveFailures: 0, connectedAt: "2026-05-24T10:00:00Z" },
  { name: "slack",   status: "degraded",   tools: [{}],     totalCalls: 5,  totalFailures: 2, consecutiveFailures: 2 },
  { name: "linear",  status: "failed",     tools: [],       totalCalls: 0,  totalFailures: 1, consecutiveFailures: 1 },
];

let reconnectMutate: any, removeMutate: any;

beforeEach(() => {
  reconnectMutate = vi.fn((_id, opts) => opts?.onSuccess?.());
  removeMutate    = vi.fn((_id, opts) => opts?.onSuccess?.());
  mockUseMcpServers.mockReturnValue({ data: SERVERS, isLoading: false });
  mockUseReconnectMcp.mockReturnValue({ mutate: reconnectMutate });
  mockUseRemoveMcp.mockReturnValue({ mutate: removeMutate });
  useToastStore.setState({ toasts: [] });
});

describe("McpPage", () => {
  it("loading_showsLoadingHint", () => {
    mockUseMcpServers.mockReturnValue({ data: undefined, isLoading: true });
    render(<McpPage />);
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("emptyList_showsEmptyState", () => {
    mockUseMcpServers.mockReturnValue({ data: [], isLoading: false });
    render(<McpPage />);
    expect(screen.getByText("还没有 MCP server")).toBeInTheDocument();
  });

  it("populated_listsEachServerByName", () => {
    render(<McpPage />);
    expect(screen.getByText("github")).toBeInTheDocument();
    expect(screen.getByText("slack")).toBeInTheDocument();
    expect(screen.getByText("linear")).toBeInTheDocument();
  });

  it("statusBadge_reflectsEachServerStatus", () => {
    render(<McpPage />);
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(screen.getByText("degraded")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
  });

  it("toolCounter_showsToolAndCallStats", () => {
    render(<McpPage />);
    expect(screen.getByText(/2 个 tool · 12 calls · 0 fail/)).toBeInTheDocument();
    expect(screen.getByText(/1 个 tool · 5 calls · 2 fail/)).toBeInTheDocument();
  });

  it("connectedAt_rendersRelTimeOnlyWhenPresent", () => {
    render(<McpPage />);
    const rels = screen.getAllByTestId("reltime");
    expect(rels.length).toBe(1);
    expect(rels[0].textContent).toBe("2026-05-24T10:00:00Z");
  });

  it("clickReconnect_callsMutateWithServerName_pushesToast", async () => {
    render(<McpPage />);
    const buttons = screen.getAllByText(/重连/);
    await userEvent.click(buttons[0]);
    expect(reconnectMutate).toHaveBeenCalledWith("github", expect.any(Object));
    expect(useToastStore.getState().toasts[0]?.title).toBe("重连请求已发出");
  });

  it("clickRemove_confirmed_callsMutateWithServerName", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<McpPage />);
    const buttons = screen.getAllByText(/移除/);
    await userEvent.click(buttons[0]);
    expect(removeMutate).toHaveBeenCalledWith("github", expect.any(Object));
    expect(useToastStore.getState().toasts[0]?.title).toBe("已移除");
    confirmSpy.mockRestore();
  });

  it("clickRemove_declined_skipsMutate", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<McpPage />);
    const buttons = screen.getAllByText(/移除/);
    await userEvent.click(buttons[0]);
    expect(removeMutate).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("consecutiveFailures_renderedInFooter", () => {
    render(<McpPage />);
    expect(screen.getByText(/连续失败 2/)).toBeInTheDocument();
    expect(screen.getByText(/连续失败 1/)).toBeInTheDocument();
  });
});
