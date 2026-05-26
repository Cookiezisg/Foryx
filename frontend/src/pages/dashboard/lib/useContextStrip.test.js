import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useContextStrip } from "./useContextStrip.js";

vi.mock("../../../api/flowruns.js", () => ({
  useFlowRuns: vi.fn(),
}));
vi.mock("../../../api/conversations.js", () => ({
  useConversations: vi.fn(),
}));

import { useFlowRuns } from "../../../api/flowruns.js";
import { useConversations } from "../../../api/conversations.js";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-05-25T12:00:00"));
});

describe("useContextStrip", () => {
  it("returns null when there's nothing of interest", () => {
    useFlowRuns.mockReturnValue({ data: [] });
    useConversations.mockReturnValue({ data: [] });
    const { result } = renderHook(() => useContextStrip());
    expect(result.current).toBeNull();
  });

  it("P1 waiting wins over P2/P3/P4", () => {
    useFlowRuns.mockReturnValue({
      data: [
        { id: "fr_1", status: "waiting_approval", workflow: "data-pipeline", startedAt: "2026-05-25T11:00:00Z" },
        { id: "fr_2", status: "failed", workflow: "etl", startedAt: "2026-05-25T10:00:00Z" },
        { id: "fr_3", status: "running", workflow: "build", startedAt: "2026-05-25T11:30:00Z" },
      ],
    });
    useConversations.mockReturnValue({
      data: [{ id: "cv_a", title: "RAG 数据准备", updatedAt: "2026-05-25T11:55:00Z" }],
    });
    const { result } = renderHook(() => useContextStrip());
    expect(result.current.kind).toBe("waiting");
    expect(result.current.payload.count).toBe(1);
    expect(result.current.payload.flowName).toBe("data-pipeline");
  });

  it("P2 failed wins over P3/P4 when no waiting", () => {
    useFlowRuns.mockReturnValue({
      data: [{ id: "fr_x", status: "failed", workflow: "etl" }],
    });
    useConversations.mockReturnValue({ data: [{ id: "cv_a", title: "x", updatedAt: "2026-05-25T11:00:00Z" }] });
    const { result } = renderHook(() => useContextStrip());
    expect(result.current.kind).toBe("failed");
    expect(result.current.payload.count).toBe(1);
  });

  it("P3 running wins over P4 recent", () => {
    useFlowRuns.mockReturnValue({
      data: [{ id: "fr_x", status: "running", workflow: "build", startedAt: "2026-05-25T11:30:00Z" }],
    });
    useConversations.mockReturnValue({ data: [{ id: "cv_a", title: "x", updatedAt: "2026-05-25T11:00:00Z" }] });
    const { result } = renderHook(() => useContextStrip());
    expect(result.current.kind).toBe("running");
    expect(result.current.payload.count).toBe(1);
    expect(result.current.payload.latestStartedAt).toBe("2026-05-25T11:30:00Z");
  });

  it("P4 recent: shows newest conv within 24h", () => {
    useFlowRuns.mockReturnValue({ data: [] });
    useConversations.mockReturnValue({
      data: [
        { id: "cv_old", title: "stale", updatedAt: "2026-05-20T00:00:00Z" },
        { id: "cv_new", title: "RAG", updatedAt: "2026-05-25T11:00:00Z" },
      ],
    });
    const { result } = renderHook(() => useContextStrip());
    expect(result.current.kind).toBe("recent");
    expect(result.current.payload.convId).toBe("cv_new");
    expect(result.current.payload.convTitle).toBe("RAG");
  });

  it("P4 ignores convs older than 24h", () => {
    useFlowRuns.mockReturnValue({ data: [] });
    useConversations.mockReturnValue({
      data: [{ id: "cv_old", title: "stale", updatedAt: "2026-05-20T00:00:00Z" }],
    });
    const { result } = renderHook(() => useContextStrip());
    expect(result.current).toBeNull();
  });
});
