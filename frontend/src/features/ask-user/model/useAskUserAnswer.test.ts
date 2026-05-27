// useAskUserAnswer — unit tests.
// Covers: empty answer bails, happy path posts + toasts + closes,
// error case pushes error toast, submitting state lifecycle.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const mockApiFetch = vi.fn();
const mockPushToast = vi.fn();

vi.mock("@shared/api", () => ({
  apiFetch: (...args: unknown[]) => mockApiFetch(...args),
}));

vi.mock("@shared/ui/toastStore", () => ({
  useToastStore: (sel: (s: { pushToast: typeof mockPushToast }) => unknown) =>
    sel({ pushToast: mockPushToast }),
}));

import { useAskUserAnswer } from "./useAskUserAnswer";

const PENDING = {
  id: "ask_1",
  conversationId: "cv_x",
  toolCallId: "tc_x",
  question: "Which option?",
  options: [{ id: "a", text: "Option A" }],
};

beforeEach(() => { vi.clearAllMocks(); });

describe("useAskUserAnswer", () => {
  it("initialState_notSubmitting", () => {
    const { result } = renderHook(() => useAskUserAnswer({ pending: PENDING, onClose: vi.fn() }));
    expect(result.current.submitting).toBe(false);
  });

  it("submit_emptyAnswer_bails_noFetch", async () => {
    const { result } = renderHook(() => useAskUserAnswer({ pending: PENDING, onClose: vi.fn() }));
    await act(async () => { await result.current.submit(""); });
    expect(mockApiFetch).not.toHaveBeenCalled();
  });

  it("submit_happyPath_callsApiAndToastsAndCloses", async () => {
    mockApiFetch.mockResolvedValueOnce({});
    const onClose = vi.fn();
    const { result } = renderHook(() => useAskUserAnswer({ pending: PENDING, onClose }));
    await act(async () => { await result.current.submit("option-a"); });
    expect(mockApiFetch).toHaveBeenCalledWith(
      "/conversations/cv_x/pending-questions/tc_x:resolve",
      expect.objectContaining({ method: "POST", body: { answer: "option-a" } }),
    );
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "success" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("submit_error_pushesErrorToast_doesNotClose", async () => {
    mockApiFetch.mockRejectedValueOnce(new Error("network failure"));
    const onClose = vi.fn();
    const { result } = renderHook(() => useAskUserAnswer({ pending: PENDING, onClose }));
    await act(async () => { await result.current.submit("option-a"); });
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "error" }));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("submit_error_includesMessageInDesc", async () => {
    mockApiFetch.mockRejectedValueOnce(new Error("timeout"));
    const { result } = renderHook(() => useAskUserAnswer({ pending: PENDING, onClose: vi.fn() }));
    await act(async () => { await result.current.submit("option-a"); });
    expect(mockPushToast).toHaveBeenCalledWith(
      expect.objectContaining({ desc: "timeout" }),
    );
  });

  it("submit_afterSubmit_submittingReturnsFalse", async () => {
    mockApiFetch.mockResolvedValueOnce({});
    const { result } = renderHook(() => useAskUserAnswer({ pending: PENDING, onClose: vi.fn() }));
    await act(async () => { await result.current.submit("option-a"); });
    expect(result.current.submitting).toBe(false);
  });
});
