// useEventLog — missing convId branches for message_stop and block_delta/block_stop.

import React from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { MockEventSource } from "../../test-setup.js";
import { useChatStore } from "@entities/conversation";
import { useSessionStore } from "@entities/session";
import { setUserIdProvider } from "@shared/api/authProvider";
import { useEventLog } from "./useEventLog.js";

const wrap = ({ children }: { children: React.ReactNode }) => {
  const c = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client: c }, children);
};

beforeEach(async () => {
  MockEventSource.reset();
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
  setUserIdProvider(() => useSessionStore.getState().currentUserId);
  useSessionStore.setState({ currentUserId: "u_test" });
  const bridge = await import("@shared/bridge/wails");
  await bridge.initBaseUrl();
});

afterEach(() => {});

describe("useEventLog — missing convId branches", () => {
  it("messageStop_missingConvId_silentlyDropped", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    expect(() => es.emit("message_stop", { id: "msg_x" })).not.toThrow();
  });

  it("blockDelta_missingConvId_silentlyDropped", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    expect(() => es.emit("block_delta", { id: "blk_x", delta: "hi" })).not.toThrow();
  });

  it("blockStop_missingConvId_silentlyDropped", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    expect(() => es.emit("block_stop", { id: "blk_x" })).not.toThrow();
  });

  it("blockStart_missingConvId_silentlyDropped", async () => {
    renderHook(() => useEventLog(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    expect(() => es.emit("block_start", { id: "blk_x", blockType: "text" })).not.toThrow();
  });
});
