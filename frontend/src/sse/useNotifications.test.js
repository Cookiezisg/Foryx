// useNotifications — SSE dispatch + per-type query invalidation +
// unread counter + ask-modal pending state.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { MockEventSource } from "../test-setup.js";
import { useSessionStore } from "@entities/session";
import { setUserIdProvider } from "@shared/api/authProvider";
import { useOverlayStore } from "@app/model";
import { useNotifications } from "./useNotifications.js";

function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = ({ children }) => createElement(QueryClientProvider, { client }, children);
  return { client, wrap };
}

beforeEach(async () => {
  MockEventSource.reset();
  globalThis.EventSource = MockEventSource;
  setUserIdProvider(() => useSessionStore.getState().currentUserId);
  useSessionStore.setState({ currentUserId: "u_test" });
  useOverlayStore.setState({ pendingAsk: null });
  const bridge = await import("../bridge/wails.js");
  await bridge.initBaseUrl();
});

afterEach(() => vi.restoreAllMocks());

describe("useNotifications", () => {
  it("connectsOnMount_andReportsInitialStatus", async () => {
    const { wrap } = makeWrapper();
    const { result } = renderHook(() => useNotifications(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    expect(result.current.unread).toBe(0);
  });

  it("notificationEvent_incrementsUnread_invalidatesQuery", async () => {
    const { client, wrap } = makeWrapper();
    const spy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useNotifications(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("notification", { type: "conversation", id: "cv_1" }));
    expect(result.current.unread).toBe(1);
    expect(spy).toHaveBeenCalledWith({ queryKey: ["conversations"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["conv", "cv_1"] });
  });

  it("askEvent_pendingAction_setsPendingAsk", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useNotifications(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("notification", {
      type: "ask", id: "ask_1", conversationId: "cv_1",
      data: { question: "ok?", action: "pending" },
    }));
    expect(useOverlayStore.getState().pendingAsk).toMatchObject({ id: "ask_1", question: "ok?" });
  });

  it("askEvent_resolvedAction_clearsPendingAsk", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useNotifications(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    useOverlayStore.setState({ pendingAsk: { id: "x" } });
    act(() => es.emit("notification", { type: "ask", id: "ask_2", data: { action: "resolved" } }));
    expect(useOverlayStore.getState().pendingAsk).toBeNull();
  });

  it("unknownType_noop_noInvalidationsNoUnreadDoubleCount", async () => {
    const { client, wrap } = makeWrapper();
    const spy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useNotifications(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("notification", { type: "totally_unknown", id: "x" }));
    // Unknown type → no invalidate happens but unread still bumps (the
    // catch-all increment lives outside the factory lookup).
    expect(spy).not.toHaveBeenCalled();
    expect(result.current.unread).toBe(1);
  });

  it("clearUnread_resetsCounter", async () => {
    const { wrap } = makeWrapper();
    const { result } = renderHook(() => useNotifications(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("notification", { type: "conversation", id: "cv_1" }));
    act(() => es.emit("notification", { type: "conversation", id: "cv_1" }));
    expect(result.current.unread).toBe(2);
    act(() => result.current.clearUnread());
    expect(result.current.unread).toBe(0);
  });
});
