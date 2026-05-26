// useEventLog — single global subscription to /eventlog. Tests verify
// event dispatch to chat store + currentUserId reconnect.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { MockEventSource } from "../test-setup.js";
import { useChatStore } from "../store/chat.js";
import { useSessionStore } from "@entities/session";
import { setUserIdProvider } from "@shared/api/authProvider";
import { useEventLog } from "./useEventLog.js";

const wrap = ({ children }) => {
  const c = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client: c }, children);
};

beforeEach(async () => {
  MockEventSource.reset();
  globalThis.EventSource = MockEventSource;
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
  setUserIdProvider(() => useSessionStore.getState().currentUserId);
  useSessionStore.setState({ currentUserId: "u_test" });
  const bridge = await import("../bridge/wails.js");
  await bridge.initBaseUrl();
});

afterEach(() => vi.restoreAllMocks());

const CV = "cv_t";

describe("useEventLog", () => {
  it("useEventLog_connects_andReportsConnectedStatus", async () => {
    const { result } = renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    expect(es.url).toBe("/api/v1/eventlog?userID=u_test");
  });

  it("messageStart_event_routesToChatStore", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    es.emit("message_start", { conversationId: CV, id: "msg_1", role: "user" });
    expect(useChatStore.getState().convs[CV].messages.has("msg_1")).toBe(true);
  });

  it("eventMissingConvId_silentlyDropped", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    expect(() => es.emit("message_start", { id: "msg_x" })).not.toThrow();
  });

  it("activeUserIdChange_tearsDownAndReconnects", async () => {
    const { rerender } = renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));

    useSessionStore.setState({ currentUserId: "u_new" });
    rerender();

    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(2));
    expect(MockEventSource.instances[0].readyState).toBe(MockEventSource.CLOSED);
    expect(MockEventSource.instances[1].url).toContain("userID=u_new");
  });

  it("blockDelta_event_appendsToBlockContent", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    es.emit("message_start", { conversationId: CV, id: "msg_1", role: "assistant" });
    es.emit("block_start", { conversationId: CV, id: "blk_1", messageId: "msg_1", blockType: "text" });
    es.emit("block_delta", { conversationId: CV, id: "blk_1", delta: "hi" });
    // rAF batched; wait
    await new Promise((r) => setTimeout(r, 25));
    expect(useChatStore.getState().convs[CV].blocks.get("blk_1").content).toBe("hi");
  });
});
