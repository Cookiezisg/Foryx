// useEntityNeighborhood + guessKind — unit tests.
// Covers: guessKind prefix table, undefined/unknown prefix fallback,
// useEntityNeighborhood dedup + limit, fromId/toId edge direction, no entityId.

import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

import { guessKind, useEntityNeighborhood } from "./useEntityNeighborhood";
import type { Relation, RelationKind } from "@entities/relation";

function makeRel(fromId: string, toId: string): Relation {
  return { id: "rel_1", userId: "u_1", fromKind: "function", fromId, toKind: "handler", toId, kind: "workflow_uses_function" as RelationKind, createdAt: "", updatedAt: "" };
}

// ── guessKind ──────────────────────────────────────────────────────────────

describe("guessKind", () => {
  it("fn_prefix_returnsFunction", () => expect(guessKind("fn_abc")).toBe("function"));
  it("f_prefix_returnsFunction", () => expect(guessKind("f_abc")).toBe("function"));
  it("hd_prefix_returnsHandler", () => expect(guessKind("hd_abc")).toBe("handler"));
  it("wf_prefix_returnsWorkflow", () => expect(guessKind("wf_abc")).toBe("workflow"));
  it("cv_prefix_returnsConversation", () => expect(guessKind("cv_abc")).toBe("conversation"));
  it("doc_prefix_returnsDocument", () => expect(guessKind("doc_abc")).toBe("document"));
  it("mem_prefix_returnsMemory", () => expect(guessKind("mem_abc")).toBe("memory"));
  it("fr_prefix_returnsFlowrun", () => expect(guessKind("fr_abc")).toBe("flowrun"));
  it("mcp_prefix_returnsMcp", () => expect(guessKind("mcp_abc")).toBe("mcp"));
  it("unknown_prefix_fallsBackToFunction", () => expect(guessKind("zzz_abc")).toBe("function"));
  it("undefined_fallsBackToFunction", () => expect(guessKind(undefined)).toBe("function"));
});

// ── useEntityNeighborhood ──────────────────────────────────────────────────

const mockNeighborhoodData: Relation[] = [];

vi.mock("@entities/relation", () => ({
  useNeighborhood: (_vars: { kind: string; id: string; depth: number }) => ({
    data: mockNeighborhoodData,
  }),
}));

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

describe("useEntityNeighborhood", () => {
  it("noRelations_returnsEmptyNeighbours", () => {
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "function"), { wrapper });
    expect(result.current.neighbours).toEqual([]);
    expect(result.current.guessedKind).toBe("function");
  });

  it("guessedKind_usesKindArgWhenProvided", () => {
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "handler"), { wrapper });
    expect(result.current.guessedKind).toBe("handler");
  });

  it("guessedKind_inferredFromIdWhenKindOmitted", () => {
    const { result } = renderHook(() => useEntityNeighborhood("wf_abc"), { wrapper });
    expect(result.current.guessedKind).toBe("workflow");
  });

  it("relations_picksOtherSideOfEdge", () => {
    mockNeighborhoodData.length = 0;
    mockNeighborhoodData.push(makeRel("fn_1", "hd_2"));
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "function"), { wrapper });
    expect(result.current.neighbours).toContain("hd_2");
  });

  it("relations_picksFromIdWhenEntityIsToId", () => {
    mockNeighborhoodData.length = 0;
    mockNeighborhoodData.push(makeRel("hd_2", "fn_1"));
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "function"), { wrapper });
    expect(result.current.neighbours).toContain("hd_2");
  });

  it("dedupes_sameNeighbourAppearingTwice", () => {
    mockNeighborhoodData.length = 0;
    mockNeighborhoodData.push(makeRel("fn_1", "hd_2"));
    mockNeighborhoodData.push({ ...makeRel("fn_1", "hd_2"), id: "rel_2" });
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "function"), { wrapper });
    expect(result.current.neighbours.filter((n) => n === "hd_2")).toHaveLength(1);
  });

  it("limits_to_defaultOf3", () => {
    mockNeighborhoodData.length = 0;
    for (let i = 0; i < 5; i++) {
      mockNeighborhoodData.push({ ...makeRel("fn_1", `hd_${i}`), id: `rel_${i}` });
    }
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "function"), { wrapper });
    expect(result.current.neighbours.length).toBe(3);
  });

  it("customLimit_respected", () => {
    mockNeighborhoodData.length = 0;
    for (let i = 0; i < 10; i++) {
      mockNeighborhoodData.push({ ...makeRel("fn_1", `hd_${i}`), id: `rel_${i}` });
    }
    const { result } = renderHook(() => useEntityNeighborhood("fn_1", "function", 5), { wrapper });
    expect(result.current.neighbours.length).toBe(5);
  });
});
