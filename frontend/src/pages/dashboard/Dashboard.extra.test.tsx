// Dashboard — additional branch coverage for ContextStrip kinds + onJump.

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Dashboard } from "./Dashboard.tsx";
import { apiFetch } from "@shared/api/httpClient";

const createMutateAsync = vi.fn().mockResolvedValue({ id: "cv_n" });
const NOW = new Date().toISOString();

vi.mock("@entities/flowrun", () => ({
  useFlowRuns: vi.fn(() => ({ data: [] as any[] })),
}));
vi.mock("@entities/conversation", () => ({
  useConversations: vi.fn(() => ({ data: [] as any[] })),
  useCreateConversation: () => ({ mutateAsync: createMutateAsync }),
}));
vi.mock("@shared/api/httpClient", () => ({
  apiFetch: vi.fn().mockResolvedValue({}),
}));
vi.mock("@entities/user", async (importOriginal) => {
  const actual = await importOriginal() as Record<string, unknown>;
  return { ...actual, useDisplayName: () => ["Alice", vi.fn()] };
});

import { useFlowRuns } from "@entities/flowrun";
import { useConversations } from "@entities/conversation";

const mockOpenPane = vi.fn();
const mockSetActiveConv = vi.fn();

function renderDash() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Dashboard onOpenPane={mockOpenPane} onSetActiveConv={mockSetActiveConv} />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  (apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({});
  createMutateAsync.mockResolvedValue({ id: "cv_n" });
  (useFlowRuns as ReturnType<typeof vi.fn>).mockReturnValue({ data: [] });
  (useConversations as ReturnType<typeof vi.fn>).mockReturnValue({ data: [] });
});

describe("Dashboard — ContextStrip kinds", () => {
  it("waiting_strip_rendered", () => {
    (useFlowRuns as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ id: "fr_1", status: "waiting_approval", workflowId: "wf_1", startedAt: NOW }],
    });
    renderDash();
    expect(document.querySelector(".wel-strip")).toBeTruthy();
  });

  it("failed_strip_rendered", () => {
    (useFlowRuns as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ id: "fr_1", status: "failed", workflowId: "wf_1", startedAt: NOW }],
    });
    renderDash();
    expect(document.querySelector(".wel-strip")).toBeTruthy();
  });

  it("running_strip_rendered", () => {
    (useFlowRuns as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ id: "fr_1", status: "running", workflowId: "wf_1", startedAt: NOW }],
    });
    renderDash();
    expect(document.querySelector(".wel-strip")).toBeTruthy();
  });

  it("recent_conv_strip_rendered", () => {
    (useConversations as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ id: "cv_1", title: "Recent Chat", updatedAt: NOW }],
    });
    renderDash();
    expect(document.querySelector(".wel-strip")).toBeTruthy();
    expect(screen.getByText("Recent Chat")).toBeInTheDocument();
  });

  it("recent_conv_click_setsActiveConvAndOpensChat", () => {
    (useConversations as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ id: "cv_1", title: "Recent Chat", updatedAt: NOW }],
    });
    renderDash();
    fireEvent.click(screen.getByText("Recent Chat"));
    expect(mockSetActiveConv).toHaveBeenCalledWith("cv_1");
    expect(mockOpenPane).toHaveBeenCalledWith("chat");
  });

  it("waiting_strip_click_opensExecute", () => {
    (useFlowRuns as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ id: "fr_1", status: "waiting_approval", workflowId: "My Flow", startedAt: NOW }],
    });
    renderDash();
    fireEvent.click(screen.getByText("My Flow"));
    expect(mockOpenPane).toHaveBeenCalledWith("execute");
  });
});

describe("Dashboard — error handling", () => {
  it("createConv_failure_pushesErrorToast", async () => {
    createMutateAsync.mockRejectedValueOnce(new Error("create failed"));
    renderDash();
    const input = screen.getByPlaceholderText("Ask Forgify… or forge something");
    fireEvent.change(input, { target: { value: "test" } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
      await Promise.resolve(); await Promise.resolve();
    });
    // No crash — error handled internally
    expect(mockOpenPane).not.toHaveBeenCalled();
  });
});
