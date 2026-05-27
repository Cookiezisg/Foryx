// useForge — handler and workflow invalidation branches in forgeCompleted.

import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { MockEventSource } from "../../test-setup.js";
import { useSessionStore } from "@entities/session";
import { setUserIdProvider } from "@shared/api/authProvider";
import { useForge, useForgeProgress } from "./useForge.js";

function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return { client, wrap };
}

beforeEach(async () => {
  MockEventSource.reset();
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
  setUserIdProvider(() => useSessionStore.getState().currentUserId);
  useSessionStore.setState({ currentUserId: "u_test" });
  useForgeProgress.setState({ active: {} });
  const bridge = await import("@shared/bridge/wails");
  await bridge.initBaseUrl();
});

afterEach(() => vi.restoreAllMocks());

describe("useForge — handler/workflow invalidation paths", () => {
  it("forgeCompleted_handlerKind_invalidatesHandlerQueries", async () => {
    const { client, wrap } = makeWrapper();
    const spy = vi.spyOn(client, "invalidateQueries");
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    const scope = { kind: "handler", id: "hd_1" };
    act(() => es.emit("forge_started", { scope }));
    act(() => es.emit("forge_completed", { scope, status: "completed", envStatus: "ready", attemptsUsed: 1 }));
    expect(spy).toHaveBeenCalledWith({ queryKey: ["handlers"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["handler", "hd_1"] });
  });

  it("forgeCompleted_workflowKind_invalidatesWorkflowQueries", async () => {
    const { client, wrap } = makeWrapper();
    const spy = vi.spyOn(client, "invalidateQueries");
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    const scope = { kind: "workflow", id: "wf_1" };
    act(() => es.emit("forge_started", { scope }));
    act(() => es.emit("forge_completed", { scope, status: "completed", envStatus: "ready", attemptsUsed: 1 }));
    expect(spy).toHaveBeenCalledWith({ queryKey: ["workflows"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["workflow", "wf_1"] });
  });

  it("forgeCompleted_unknownKind_noInvalidation", async () => {
    const { client, wrap } = makeWrapper();
    const spy = vi.spyOn(client, "invalidateQueries");
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    const scope = { kind: "unknown", id: "unk_1" };
    act(() => es.emit("forge_started", { scope }));
    act(() => es.emit("forge_completed", { scope, status: "completed" }));
    // No entity-level invalidation for unknown kind
    const callUrls = spy.mock.calls.map((c) => JSON.stringify(c));
    const hasEntityInvalidation = callUrls.some((c) => c.includes('"unknown"'));
    expect(hasEntityInvalidation).toBe(false);
  });
});
