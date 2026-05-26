// sse/shared — createSSE factory: URL construction (incl. currentUserId
// query param), event handler wiring, status callbacks.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MockEventSource } from "../test-setup.js";

beforeEach(async () => {
  MockEventSource.reset();
  globalThis.EventSource = MockEventSource;
  const bridge = await import("../bridge/wails.js");
  await bridge.initBaseUrl();
  // Wire session store as the userId provider so createSSE sees currentUserId.
  const { useSessionStore } = await import("@entities/session");
  const { setUserIdProvider } = await import("@shared/api/authProvider");
  setUserIdProvider(() => useSessionStore.getState().currentUserId);
  useSessionStore.setState({ currentUserId: null });
});

afterEach(() => vi.restoreAllMocks());

describe("createSSE", () => {
  it("createSSE_noActiveUser_skipsConnection", async () => {
    const { createSSE } = await import("./shared.js");
    const onStatus = vi.fn();
    const ctrl = createSSE({ path: "/eventlog", eventHandlers: {}, onStatus });
    // No EventSource constructed when currentUserId is null — would 401 anyway.
    expect(MockEventSource.instances.length).toBe(0);
    expect(onStatus).toHaveBeenCalledWith("disconnected");
    // Controller is a no-op so callers can still call close() safely.
    expect(() => ctrl.close()).not.toThrow();
  });

  it("createSSE_activeUserId_appendsUserIdQuery", async () => {
    const { useSessionStore } = await import("@entities/session");
    useSessionStore.setState({ currentUserId: "u_test_123" });
    const { createSSE } = await import("./shared.js");
    createSSE({ path: "/eventlog", eventHandlers: {} });
    const es = MockEventSource.instances.at(-1);
    expect(es.url).toContain("userID=u_test_123");
    useSessionStore.setState({ currentUserId: null });
  });

  it("createSSE_pathWithExistingQuery_usesAmpersand", async () => {
    const { useSessionStore } = await import("@entities/session");
    useSessionStore.setState({ currentUserId: "u_q" });
    const { createSSE } = await import("./shared.js");
    createSSE({ path: "/eventlog?a=1", eventHandlers: {} });
    const es = MockEventSource.instances.at(-1);
    expect(es.url).toBe("/api/v1/eventlog?a=1&userID=u_q");
    useSessionStore.setState({ currentUserId: null });
  });

  it("createSSE_dispatchesParsedJSONToHandler", async () => {
    const { useSessionStore } = await import("@entities/session");
    useSessionStore.setState({ currentUserId: "u_d" });
    const { createSSE } = await import("./shared.js");
    const onDelta = vi.fn();
    createSSE({ path: "/eventlog", eventHandlers: { block_delta: onDelta } });
    const es = MockEventSource.instances.at(-1);
    es.emit("block_delta", { id: "blk_1", delta: "hi" }, "42");
    expect(onDelta).toHaveBeenCalledWith({ id: "blk_1", delta: "hi" }, { seq: 42, raw: '{"id":"blk_1","delta":"hi"}' });
    useSessionStore.setState({ currentUserId: null });
  });

  it("createSSE_invalidJSON_passesNullPayload", async () => {
    const { useSessionStore } = await import("@entities/session");
    useSessionStore.setState({ currentUserId: "u_j" });
    const { createSSE } = await import("./shared.js");
    const handler = vi.fn();
    createSSE({ path: "/eventlog", eventHandlers: { x: handler } });
    const es = MockEventSource.instances.at(-1);
    es.emit("x", "not-json{");
    expect(handler.mock.calls[0][0]).toBeNull();
    useSessionStore.setState({ currentUserId: null });
  });

  it("createSSE_handlerThrows_catchesAndDoesNotPropagate", async () => {
    const { useSessionStore } = await import("@entities/session");
    useSessionStore.setState({ currentUserId: "u_t" });
    const { createSSE } = await import("./shared.js");
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    createSSE({
      path: "/eventlog",
      eventHandlers: { bad: () => { throw new Error("boom"); } },
    });
    const es = MockEventSource.instances.at(-1);
    expect(() => es.emit("bad", {})).not.toThrow();
    expect(errSpy).toHaveBeenCalled();
    useSessionStore.setState({ currentUserId: null });
  });

  it("createSSE_close_callsEventSourceClose", async () => {
    const { useSessionStore } = await import("@entities/session");
    useSessionStore.setState({ currentUserId: "u_close" });
    const { createSSE } = await import("./shared.js");
    const ctrl = createSSE({ path: "/eventlog", eventHandlers: {} });
    const es = MockEventSource.instances.at(-1);
    ctrl.close();
    expect(es.readyState).toBe(MockEventSource.CLOSED);
    useSessionStore.setState({ currentUserId: null });
  });

  it("createSSE_closedWhileUidStillActive_callsNotifyAuthFailure", async () => {
    const { useSessionStore } = await import("@entities/session");
    const { setOnAuthFailure } = await import("@shared/api/authProvider");
    const mockAuthFailure = vi.fn();
    setOnAuthFailure(mockAuthFailure);
    useSessionStore.setState({ currentUserId: "u_heal" });
    const { createSSE } = await import("./shared.js");
    createSSE({ path: "/eventlog", eventHandlers: {} });
    const es = MockEventSource.instances.at(-1);
    // Simulate backend rejection: connection closed permanently.
    es.readyState = MockEventSource.CLOSED;
    es.emit("error", null);
    // Self-heal triggered notifyAuthFailure.
    expect(mockAuthFailure).toHaveBeenCalled();
    setOnAuthFailure(() => {});
  });

  it("createSSE_closedAfterUidChanged_doesNotCallNotify", async () => {
    const { useSessionStore } = await import("@entities/session");
    const { setOnAuthFailure } = await import("@shared/api/authProvider");
    const mockAuthFailure = vi.fn();
    setOnAuthFailure(mockAuthFailure);
    useSessionStore.setState({ currentUserId: "u_old" });
    const { createSSE } = await import("./shared.js");
    createSSE({ path: "/eventlog", eventHandlers: {} });
    const es = MockEventSource.instances.at(-1);
    // User already switched accounts before this connection died.
    useSessionStore.setState({ currentUserId: "u_new" });
    es.readyState = MockEventSource.CLOSED;
    es.emit("error", null);
    // Self-heal must NOT fire because uid no longer matches current provider value.
    expect(mockAuthFailure).not.toHaveBeenCalled();
    setOnAuthFailure(() => {});
    useSessionStore.setState({ currentUserId: null });
  });
});
