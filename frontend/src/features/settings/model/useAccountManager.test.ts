// useAccountManager — unit tests.
// Covers: name state, switchTo invalidates + toasts, addAccount happy path,
// addAccount empty-name bails, isAdding passthrough.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockCreateUserAsync = vi.fn();
const mockPushToast = vi.fn();
const mockInvalidateQueries = vi.fn();
const mockSetCurrentUser = vi.fn();

vi.mock("@entities/user", () => ({
  useCreateUser: () => ({ mutateAsync: mockCreateUserAsync, isPending: false }),
}));

vi.mock("@entities/session", () => {
  const store = { currentUserId: null as string | null };
  return {
    useSessionStore: Object.assign(
      (sel: (s: { setCurrentUser: typeof mockSetCurrentUser }) => unknown) =>
        sel({ setCurrentUser: mockSetCurrentUser }),
      {
        getState: () => ({
          ...store,
          setCurrentUser: (id: string) => { store.currentUserId = id; mockSetCurrentUser(id); },
        }),
      },
    ),
  };
});

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

import { useAccountManager } from "./useAccountManager";

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockCreateUserAsync.mockResolvedValue({ id: "u_new", username: "alice" });
});

describe("useAccountManager", () => {
  it("initialState_nameIsEmpty", () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    expect(result.current.name).toBe("");
  });

  it("setName_updatesNameState", () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    act(() => { result.current.setName("bob"); });
    expect(result.current.name).toBe("bob");
  });

  it("switchTo_setsCurrentUserAndInvalidatesQueries", () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    act(() => { result.current.switchTo("u_abc"); });
    expect(mockSetCurrentUser).toHaveBeenCalledWith("u_abc");
    expect(mockInvalidateQueries).toHaveBeenCalled();
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "success" }));
  });

  it("addAccount_emptyName_bails_noMutation", async () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    // name is "" by default
    await act(async () => { await result.current.addAccount(); });
    expect(mockCreateUserAsync).not.toHaveBeenCalled();
  });

  it("addAccount_whitespaceOnly_bails", async () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    act(() => { result.current.setName("   "); });
    await act(async () => { await result.current.addAccount(); });
    expect(mockCreateUserAsync).not.toHaveBeenCalled();
  });

  it("addAccount_happyPath_createsUserAndSwitchesAndClearsName", async () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    act(() => { result.current.setName("alice"); });
    await act(async () => { await result.current.addAccount(); });
    expect(mockCreateUserAsync).toHaveBeenCalledWith({ username: "alice" });
    expect(mockSetCurrentUser).toHaveBeenCalledWith("u_new");
    expect(result.current.name).toBe("");
  });

  it("addAccount_error_silenced_noUnhandledRejection", async () => {
    mockCreateUserAsync.mockRejectedValueOnce(new Error("server error"));
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    act(() => { result.current.setName("carol"); });
    // Should not throw
    await act(async () => { await result.current.addAccount(); });
    expect(mockPushToast).not.toHaveBeenCalledWith(expect.objectContaining({ kind: "error" }));
  });

  it("isAdding_reflectsCreateUserPending", () => {
    const { result } = renderHook(() => useAccountManager(), { wrapper });
    expect(result.current.isAdding).toBe(false);
  });
});
