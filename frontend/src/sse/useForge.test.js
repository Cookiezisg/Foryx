// useForge — 4-event SSE stream → useForgeProgress store + cache
// invalidation on completion.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { MockEventSource } from "../test-setup.js";
import { useSessionStore } from "@entities/session";
import { setUserIdProvider } from "@shared/api/authProvider";
import { useForge, useForgeProgress } from "./useForge.js";

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
  useForgeProgress.setState({ active: {} });
  const bridge = await import("../bridge/wails.js");
  await bridge.initBaseUrl();
});

afterEach(() => vi.restoreAllMocks());

const SCOPE = { kind: "function", id: "fn_1" };
const KEY = "function:fn_1";

describe("useForge", () => {
  it("forgeStarted_seedsProgress", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("forge_started", {
      scope: SCOPE, operation: "create", conversationId: "cv_1", toolCallId: "tc_1",
    }));
    const p = useForgeProgress.getState().active[KEY];
    expect(p.status).toBe("running");
    expect(p.ops).toEqual([]);
  });

  it("forgeOpApplied_appendsOp", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("forge_started", { scope: SCOPE }));
    act(() => es.emit("forge_op_applied", { scope: SCOPE, index: 1, op: { type: "edit" } }));
    expect(useForgeProgress.getState().active[KEY].ops).toEqual([{ index: 1, op: { type: "edit" } }]);
  });

  it("forgeOpApplied_unknownScope_noop", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    // No started event first
    act(() => es.emit("forge_op_applied", { scope: SCOPE, index: 1, op: { type: "edit" } }));
    expect(useForgeProgress.getState().active[KEY]).toBeUndefined();
  });

  it("forgeEnvAttempt_appendsAttempt", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("forge_started", { scope: SCOPE }));
    act(() => es.emit("forge_env_attempt", {
      scope: SCOPE, attempt: 1, status: "running", stage: "install",
    }));
    expect(useForgeProgress.getState().active[KEY].envAttempts).toHaveLength(1);
  });

  it("forgeCompleted_setsTerminalStatusAndInvalidates", async () => {
    const { client, wrap } = makeWrapper();
    const spy = vi.spyOn(client, "invalidateQueries");
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("forge_started", { scope: SCOPE }));
    act(() => es.emit("forge_completed", {
      scope: SCOPE, status: "completed", versionId: "fv_1", envStatus: "ready", attemptsUsed: 1,
    }));
    const p = useForgeProgress.getState().active[KEY];
    expect(p.status).toBe("completed");
    expect(p.versionId).toBe("fv_1");
    expect(spy).toHaveBeenCalledWith({ queryKey: ["functions"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["function", "fn_1"] });
  });

  it("forgeCompleted_withoutStartedFirst_stillStoresFinalState", async () => {
    const { wrap } = makeWrapper();
    renderHook(() => useForge(), { wrapper: wrap });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    const es = MockEventSource.instances[0];
    act(() => es.emit("forge_completed", {
      scope: SCOPE, status: "failed", error: "oom",
    }));
    expect(useForgeProgress.getState().active[KEY].status).toBe("failed");
  });
});

describe("useForgeProgress store", () => {
  it("put_addsToActiveMap", () => {
    useForgeProgress.getState().put("function:x", { status: "running" });
    expect(useForgeProgress.getState().active["function:x"]).toEqual({ status: "running" });
  });

  it("clear_removesEntry", () => {
    useForgeProgress.getState().put("function:x", { status: "running" });
    useForgeProgress.getState().clear("function:x");
    expect(useForgeProgress.getState().active["function:x"]).toBeUndefined();
  });
});
