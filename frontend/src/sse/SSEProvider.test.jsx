// SSEProvider — composes 3 hooks + exposes combined health via context.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MockEventSource } from "../test-setup.js";
import { useSessionStore } from "@entities/session";
import { setUserIdProvider } from "@shared/api/authProvider";
import { SSEProvider, useSSEHealth } from "./SSEProvider.jsx";

function wrap(children) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <SSEProvider>{children}</SSEProvider>
    </QueryClientProvider>
  );
}

beforeEach(async () => {
  MockEventSource.reset();
  globalThis.EventSource = MockEventSource;
  setUserIdProvider(() => useSessionStore.getState().currentUserId);
  useSessionStore.setState({ currentUserId: "u_test" });
  const bridge = await import("../bridge/wails.js");
  await bridge.initBaseUrl();
});

afterEach(() => vi.restoreAllMocks());

describe("SSEProvider", () => {
  it("mountsAllThreeStreams_eventlogNotifsForge", async () => {
    render(wrap(<div />));
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(3));
    const urls = MockEventSource.instances.map((es) => es.url);
    expect(urls.some((u) => u.includes("/eventlog"))).toBe(true);
    expect(urls.some((u) => u.includes("/notifications"))).toBe(true);
    expect(urls.some((u) => u.includes("/forge"))).toBe(true);
  });

  it("useSSEHealth_outsideProvider_returnsUnknownDefaults", () => {
    const { result } = renderHook(() => useSSEHealth());
    expect(result.current.overall).toBe("unknown");
    expect(result.current.unread).toBe(0);
  });

  it("useSSEHealth_insideProvider_exposesStatusObject", async () => {
    const { result } = renderHook(() => useSSEHealth(), { wrapper: ({ children }) => wrap(children) });
    await vi.waitFor(() => expect(MockEventSource.instances.length).toBe(3));
    expect(["ok", "warn", "err", "unknown"]).toContain(result.current.overall);
    expect(typeof result.current.clearUnread).toBe("function");
  });
});
