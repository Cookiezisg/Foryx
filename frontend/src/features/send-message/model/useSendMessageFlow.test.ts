// useSendMessageFlow — unit tests.
// Covers: submit routing, CONVERSATION_NOT_FOUND self-heal, cancel warn toast,
// empty attachments/mentions skip, isPending passthrough.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockSendMutate = vi.fn();
const mockCancelMutate = vi.fn();
const mockPushToast = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@entities/conversation", () => ({
  useSendMessage: () => ({ mutate: mockSendMutate, isPending: false }),
  useCancelStream: () => ({ mutate: mockCancelMutate }),
}));

vi.mock("@shared/ui/toastStore", () => ({
  useToastStore: (sel: (s: { pushToast: typeof mockPushToast }) => unknown) =>
    sel({ pushToast: mockPushToast }),
}));

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual("@tanstack/react-query");
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

import { useSendMessageFlow } from "./useSendMessageFlow";

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useSendMessageFlow", () => {
  it("submit_callsMutateWithContentBody", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    act(() => { result.current.submit({ content: "hello" }); });
    expect(mockSendMutate).toHaveBeenCalledWith(
      expect.objectContaining({ content: "hello" }),
      expect.any(Object),
    );
  });

  it("submit_withAttachments_mapsFieldNames", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    act(() => {
      result.current.submit({
        content: "hi",
        attachments: [{ name: "f.txt", size: 42 }],
      });
    });
    const body = mockSendMutate.mock.calls[0][0] as Record<string, unknown>;
    expect((body.attachments as Array<{ fileName: string; sizeBytes: number }>)[0]).toEqual({
      fileName: "f.txt",
      sizeBytes: 42,
    });
  });

  it("submit_withMentions_mapsFields", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    act(() => {
      result.current.submit({
        content: "hey",
        mentions: [{ type: "function", id: "fn_1" }],
      });
    });
    const body = mockSendMutate.mock.calls[0][0] as Record<string, unknown>;
    expect((body.mentions as Array<{ type: string; id: string }>)[0]).toEqual({ type: "function", id: "fn_1" });
  });

  it("submit_emptyAttachments_omitsField", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    act(() => { result.current.submit({ content: "hi", attachments: [] }); });
    const body = mockSendMutate.mock.calls[0][0] as Record<string, unknown>;
    expect(body).not.toHaveProperty("attachments");
  });

  it("onConvGone_calledAndQueriesInvalidatedOnCONVERSATION_NOT_FOUND", () => {
    const onConvGone = vi.fn();
    const { result } = renderHook(() => useSendMessageFlow("cv_1", { onConvGone }), { wrapper });

    // Capture the onError callback passed to mutate
    act(() => { result.current.submit({ content: "hi" }); });
    const { onError } = mockSendMutate.mock.calls[0][1] as { onError: (e: Error & { code?: string }) => void };

    act(() => { onError(Object.assign(new Error("gone"), { code: "CONVERSATION_NOT_FOUND" })); });

    expect(mockInvalidateQueries).toHaveBeenCalled();
    expect(onConvGone).toHaveBeenCalled();
  });

  it("onError_otherCode_doesNotCallOnConvGone", () => {
    const onConvGone = vi.fn();
    const { result } = renderHook(() => useSendMessageFlow("cv_1", { onConvGone }), { wrapper });

    act(() => { result.current.submit({ content: "hi" }); });
    const { onError } = mockSendMutate.mock.calls[0][1] as { onError: (e: Error & { code?: string }) => void };

    act(() => { onError(Object.assign(new Error("bad"), { code: "INTERNAL_ERROR" })); });

    expect(onConvGone).not.toHaveBeenCalled();
  });

  it("cancelStream_callsCancelMutate", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    act(() => { result.current.cancelStream(); });
    expect(mockCancelMutate).toHaveBeenCalled();
  });

  it("cancelStream_onError_pushesWarnToast", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    act(() => { result.current.cancelStream(); });
    const { onError } = mockCancelMutate.mock.calls[0][1] as { onError: (e: Error) => void };

    act(() => { onError(new Error("cancel fail")); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "warn" }));
  });

  it("isPending_reflectsSendMutationState", () => {
    const { result } = renderHook(() => useSendMessageFlow("cv_1"), { wrapper });
    expect(result.current.isPending).toBe(false);
  });
});
