// SSEProvider — deriveOverall branch coverage.
// Mocks all 3 SSE hooks to exercise "ok", "err", and "warn" outcomes.

import React from "react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { SSEProvider, useSSEHealth } from "./SSEProvider.tsx";

vi.mock("./useEventLog", () => ({
  useEventLog: vi.fn(() => "connected"),
}));
vi.mock("./useNotifications", () => ({
  useNotifications: vi.fn(() => ({ status: "connected", unread: 0, clearUnread: vi.fn() })),
}));
vi.mock("./useForge", () => ({
  useForge: vi.fn(() => "connected"),
}));

import { useEventLog } from "./useEventLog";
import { useNotifications } from "./useNotifications";
import { useForge } from "./useForge";

function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, createElement(SSEProvider, {}, children));
}

afterEach(() => vi.restoreAllMocks());

describe("SSEProvider — deriveOverall branches", () => {
  it("allConnected_overall_isOk", () => {
    vi.mocked(useEventLog).mockReturnValue("connected");
    vi.mocked(useNotifications).mockReturnValue({ status: "connected", unread: 0, clearUnread: vi.fn() });
    vi.mocked(useForge).mockReturnValue("connected");

    const { result } = renderHook(() => useSSEHealth(), { wrapper: makeWrapper() });
    expect(result.current.overall).toBe("ok");
  });

  it("oneDisconnected_overall_isErr", () => {
    vi.mocked(useEventLog).mockReturnValue("disconnected");
    vi.mocked(useNotifications).mockReturnValue({ status: "connected", unread: 0, clearUnread: vi.fn() });
    vi.mocked(useForge).mockReturnValue("connected");

    const { result } = renderHook(() => useSSEHealth(), { wrapper: makeWrapper() });
    expect(result.current.overall).toBe("err");
  });

  it("oneConnecting_overall_isWarn", () => {
    vi.mocked(useEventLog).mockReturnValue("connecting");
    vi.mocked(useNotifications).mockReturnValue({ status: "connected", unread: 0, clearUnread: vi.fn() });
    vi.mocked(useForge).mockReturnValue("connected");

    const { result } = renderHook(() => useSSEHealth(), { wrapper: makeWrapper() });
    expect(result.current.overall).toBe("warn");
  });
});
