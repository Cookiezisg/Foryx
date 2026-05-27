// useForgeReview — unit tests.
// Covers: accept/reject/revert for all three kinds (function/handler/workflow),
// toast on success, revert only defined for function, missing for handler/workflow.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockAcceptFn = vi.fn();
const mockRejectFn = vi.fn();
const mockRevertFn = vi.fn();
const mockAcceptHd = vi.fn();
const mockRejectHd = vi.fn();
const mockAcceptWf = vi.fn();
const mockRejectWf = vi.fn();
const mockPushToast = vi.fn();

vi.mock("@entities/function", () => ({
  useAcceptFunction: () => ({ mutate: mockAcceptFn }),
  useRejectFunction: () => ({ mutate: mockRejectFn }),
  useRevertFunction: () => ({ mutate: mockRevertFn }),
}));

vi.mock("@entities/handler", () => ({
  useAcceptHandler: () => ({ mutate: mockAcceptHd }),
  useRejectHandler: () => ({ mutate: mockRejectHd }),
}));

vi.mock("@entities/workflow", () => ({
  useAcceptWorkflow: () => ({ mutate: mockAcceptWf }),
  useRejectWorkflow: () => ({ mutate: mockRejectWf }),
}));

vi.mock("@shared/ui/toastStore", () => ({
  useToastStore: (sel: (s: { pushToast: typeof mockPushToast }) => unknown) =>
    sel({ pushToast: mockPushToast }),
}));

import { useForgeReview } from "./useForgeReview";

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => { vi.clearAllMocks(); });

describe("useForgeReview — function", () => {
  it("accept_callsAcceptFnMutate", () => {
    const { result } = renderHook(() => useForgeReview("function", "fn_1", "myFn"), { wrapper });
    act(() => { result.current.accept(); });
    expect(mockAcceptFn).toHaveBeenCalledWith("fn_1", expect.any(Object));
  });

  it("accept_onSuccess_pushesSuccessToast", () => {
    const { result } = renderHook(() => useForgeReview("function", "fn_1", "myFn"), { wrapper });
    act(() => { result.current.accept(); });
    const { onSuccess } = mockAcceptFn.mock.calls[0][1] as { onSuccess: () => void };
    act(() => { onSuccess(); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "success" }));
  });

  it("reject_callsRejectFnMutate", () => {
    const { result } = renderHook(() => useForgeReview("function", "fn_1", "myFn"), { wrapper });
    act(() => { result.current.reject(); });
    expect(mockRejectFn).toHaveBeenCalledWith("fn_1", expect.any(Object));
  });

  it("reject_onSuccess_pushesWarnToast", () => {
    const { result } = renderHook(() => useForgeReview("function", "fn_1", "myFn"), { wrapper });
    act(() => { result.current.reject(); });
    const { onSuccess } = mockRejectFn.mock.calls[0][1] as { onSuccess: () => void };
    act(() => { onSuccess(); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "warn" }));
  });

  it("revert_defined_andCallsRevertFnMutate", () => {
    const { result } = renderHook(() => useForgeReview("function", "fn_1", "myFn"), { wrapper });
    expect(result.current.revert).toBeDefined();
    act(() => { result.current.revert!(); });
    expect(mockRevertFn).toHaveBeenCalledWith("fn_1", expect.any(Object));
  });

  it("revert_onSuccess_pushesWarnToast", () => {
    const { result } = renderHook(() => useForgeReview("function", "fn_1", "myFn"), { wrapper });
    act(() => { result.current.revert!(); });
    const { onSuccess } = mockRevertFn.mock.calls[0][1] as { onSuccess: () => void };
    act(() => { onSuccess(); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "warn" }));
  });
});

describe("useForgeReview — handler", () => {
  it("accept_callsAcceptHdMutate", () => {
    const { result } = renderHook(() => useForgeReview("handler", "hd_1"), { wrapper });
    act(() => { result.current.accept(); });
    expect(mockAcceptHd).toHaveBeenCalledWith("hd_1", expect.any(Object));
  });

  it("accept_onSuccess_pushesSuccessToast", () => {
    const { result } = renderHook(() => useForgeReview("handler", "hd_1"), { wrapper });
    act(() => { result.current.accept(); });
    const { onSuccess } = mockAcceptHd.mock.calls[0][1] as { onSuccess: () => void };
    act(() => { onSuccess(); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "success" }));
  });

  it("reject_callsRejectHdMutate", () => {
    const { result } = renderHook(() => useForgeReview("handler", "hd_1"), { wrapper });
    act(() => { result.current.reject(); });
    expect(mockRejectHd).toHaveBeenCalledWith("hd_1", expect.any(Object));
  });

  it("revert_notDefinedForHandler", () => {
    const { result } = renderHook(() => useForgeReview("handler", "hd_1"), { wrapper });
    expect(result.current.revert).toBeUndefined();
  });
});

describe("useForgeReview — workflow", () => {
  it("accept_callsAcceptWfMutate", () => {
    const { result } = renderHook(() => useForgeReview("workflow", "wf_1"), { wrapper });
    act(() => { result.current.accept(); });
    expect(mockAcceptWf).toHaveBeenCalledWith("wf_1", expect.any(Object));
  });

  it("reject_callsRejectWfMutate", () => {
    const { result } = renderHook(() => useForgeReview("workflow", "wf_1"), { wrapper });
    act(() => { result.current.reject(); });
    expect(mockRejectWf).toHaveBeenCalledWith("wf_1", expect.any(Object));
  });

  it("reject_onSuccess_pushesWarnToast", () => {
    const { result } = renderHook(() => useForgeReview("workflow", "wf_1"), { wrapper });
    act(() => { result.current.reject(); });
    const { onSuccess } = mockRejectWf.mock.calls[0][1] as { onSuccess: () => void };
    act(() => { onSuccess(); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "warn" }));
  });

  it("revert_notDefinedForWorkflow", () => {
    const { result } = renderHook(() => useForgeReview("workflow", "wf_1"), { wrapper });
    expect(result.current.revert).toBeUndefined();
  });
});
