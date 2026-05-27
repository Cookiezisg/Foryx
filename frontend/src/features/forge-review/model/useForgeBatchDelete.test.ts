// useForgeBatchDelete — unit tests.
// Covers: confirm cancel bails early, each kind routed to correct mutation,
// clearSel called after delete, mixed-kind batch.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockDeleteFn = vi.fn();
const mockDeleteHd = vi.fn();
const mockDeleteWf = vi.fn();

vi.mock("@entities/function", () => ({
  useDeleteFunction: () => ({ mutate: mockDeleteFn }),
}));

vi.mock("@entities/handler", () => ({
  useDeleteHandler: () => ({ mutate: mockDeleteHd }),
}));

vi.mock("@entities/workflow", () => ({
  useDeleteWorkflow: () => ({ mutate: mockDeleteWf }),
}));

import { useForgeBatchDelete } from "./useForgeBatchDelete";

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => { vi.clearAllMocks(); });

describe("useForgeBatchDelete", () => {
  it("confirmCancel_bails_noDeletions", () => {
    vi.spyOn(window, "confirm").mockReturnValueOnce(false);
    const { result } = renderHook(() => useForgeBatchDelete(), { wrapper });
    const clearSel = vi.fn();
    act(() => {
      result.current.batchDelete([{ id: "fn_1", kind: "function" }], clearSel);
    });
    expect(mockDeleteFn).not.toHaveBeenCalled();
    expect(clearSel).not.toHaveBeenCalled();
  });

  it("function_routesToDeleteFn", () => {
    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    const { result } = renderHook(() => useForgeBatchDelete(), { wrapper });
    const clearSel = vi.fn();
    act(() => {
      result.current.batchDelete([{ id: "fn_1", kind: "function" }], clearSel);
    });
    expect(mockDeleteFn).toHaveBeenCalledWith("fn_1");
    expect(clearSel).toHaveBeenCalled();
  });

  it("handler_routesToDeleteHd", () => {
    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    const { result } = renderHook(() => useForgeBatchDelete(), { wrapper });
    act(() => {
      result.current.batchDelete([{ id: "hd_1", kind: "handler" }], vi.fn());
    });
    expect(mockDeleteHd).toHaveBeenCalledWith("hd_1");
  });

  it("workflow_routesToDeleteWf", () => {
    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    const { result } = renderHook(() => useForgeBatchDelete(), { wrapper });
    act(() => {
      result.current.batchDelete([{ id: "wf_1", kind: "workflow" }], vi.fn());
    });
    expect(mockDeleteWf).toHaveBeenCalledWith("wf_1");
  });

  it("mixedBatch_callsEachKind", () => {
    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    const { result } = renderHook(() => useForgeBatchDelete(), { wrapper });
    const clearSel = vi.fn();
    act(() => {
      result.current.batchDelete([
        { id: "fn_1", kind: "function" },
        { id: "hd_1", kind: "handler" },
        { id: "wf_1", kind: "workflow" },
      ], clearSel);
    });
    expect(mockDeleteFn).toHaveBeenCalledWith("fn_1");
    expect(mockDeleteHd).toHaveBeenCalledWith("hd_1");
    expect(mockDeleteWf).toHaveBeenCalledWith("wf_1");
    expect(clearSel).toHaveBeenCalled();
  });

  it("clearSel_calledEvenIfNoItems", () => {
    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    const { result } = renderHook(() => useForgeBatchDelete(), { wrapper });
    const clearSel = vi.fn();
    act(() => { result.current.batchDelete([], clearSel); });
    expect(clearSel).toHaveBeenCalled();
  });
});
