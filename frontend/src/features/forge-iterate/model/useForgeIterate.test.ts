// useForgeIterate — unit tests.
// Covers: submit happy path returns conversationId, missing cid pushes warn,
// mutate error returns null (no toast here — global handles), isPending passthrough.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockMutateAsync = vi.fn();
const mockPushToast = vi.fn();

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual("@tanstack/react-query");
  return {
    ...actual,
    useMutation: () => ({
      mutateAsync: mockMutateAsync,
      isPending: false,
    }),
  };
});

vi.mock("@shared/api", () => ({
  apiFetch: vi.fn(),
}));

vi.mock("@shared/ui/toastStore", () => ({
  useToastStore: (sel: (s: { pushToast: typeof mockPushToast }) => unknown) =>
    sel({ pushToast: mockPushToast }),
}));

import { useForgeIterate } from "./useForgeIterate";

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => { vi.clearAllMocks(); });

describe("useForgeIterate", () => {
  it("submit_happyPath_returnsConversationId", async () => {
    mockMutateAsync.mockResolvedValueOnce({ conversationId: "cv_new" });
    const { result } = renderHook(() => useForgeIterate(), { wrapper });
    let cid: string | null = null;
    await act(async () => {
      cid = await result.current.submit("function", "fn_1", "refactor this");
    });
    expect(cid).toBe("cv_new");
  });

  it("submit_usesFallbackIdField", async () => {
    mockMutateAsync.mockResolvedValueOnce({ id: "cv_fallback" });
    const { result } = renderHook(() => useForgeIterate(), { wrapper });
    let cid: string | null = null;
    await act(async () => {
      cid = await result.current.submit("handler", "hd_1", "add logging");
    });
    expect(cid).toBe("cv_fallback");
  });

  it("submit_missingCid_pushesWarnToastAndReturnsNull", async () => {
    mockMutateAsync.mockResolvedValueOnce({});
    const { result } = renderHook(() => useForgeIterate(), { wrapper });
    let cid: string | null = undefined as unknown as string | null;
    await act(async () => {
      cid = await result.current.submit("workflow", "wf_1", "add step");
    });
    expect(cid).toBeNull();
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "warn" }));
  });

  it("submit_mutateThrows_returnsNull_noToast", async () => {
    mockMutateAsync.mockRejectedValueOnce(new Error("network error"));
    const { result } = renderHook(() => useForgeIterate(), { wrapper });
    let cid: string | null = undefined as unknown as string | null;
    await act(async () => {
      cid = await result.current.submit("function", "fn_1", "fix it");
    });
    expect(cid).toBeNull();
    expect(mockPushToast).not.toHaveBeenCalled();
  });

  it("isPending_falseByDefault", () => {
    const { result } = renderHook(() => useForgeIterate(), { wrapper });
    expect(result.current.isPending).toBe(false);
  });
});
