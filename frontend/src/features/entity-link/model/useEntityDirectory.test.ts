// useEntityDirectory + normEdges — unit tests.
// Covers: normEdges filter malformed entries, node aggregation from all 7
// entity queries, edge normalisation, skill/mcp use name as id.

import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

import { normEdges, useEntityDirectory } from "./useEntityDirectory";
import type { EntityEdge } from "./useEntityDirectory";
import type { Relation, RelationKind } from "@entities/relation";

function makeRel(fromId: string, toId: string): Relation {
  return { id: "rel_1", userId: "u_1", fromKind: "function", fromId, toKind: "handler", toId, kind: "workflow_uses_function" as RelationKind, createdAt: "", updatedAt: "" };
}

// ── normEdges ──────────────────────────────────────────────────────────────

describe("normEdges", () => {
  it("mapsFromIdToIdKind", () => {
    const rels: Relation[] = [makeRel("fn_1", "hd_2")];
    const edges = normEdges(rels);
    expect(edges[0].from).toBe("fn_1");
    expect(edges[0].to).toBe("hd_2");
  });

  it("filtersOutMissingFrom", () => {
    const rels = [{ ...makeRel("", "hd_2") }];
    expect(normEdges(rels)).toHaveLength(0);
  });

  it("filtersOutMissingTo", () => {
    const rels = [{ ...makeRel("fn_1", "") }];
    expect(normEdges(rels)).toHaveLength(0);
  });

  it("emptyArray_returnsEmpty", () => {
    expect(normEdges([])).toEqual([]);
  });
});

// ── useEntityDirectory ─────────────────────────────────────────────────────

vi.mock("@entities/function", () => ({
  useFunctions: () => ({ data: [{ id: "fn_1", name: "addNums", description: "adds" }] }),
}));
vi.mock("@entities/handler", () => ({
  useHandlers: () => ({ data: [{ id: "hd_1", name: "MyHandler", description: "handles" }] }),
}));
vi.mock("@entities/workflow", () => ({
  useWorkflows: () => ({ data: [{ id: "wf_1", name: "MyFlow", description: "flows" }] }),
}));
vi.mock("@entities/document", () => ({
  useDocuments: () => ({ data: [{ id: "doc_1", name: "README" }] }),
}));
vi.mock("@entities/skill", () => ({
  useSkills: () => ({ data: [{ name: "py-runner", description: "runs python" }] }),
}));
vi.mock("@entities/mcp", () => ({
  useMcpServers: () => ({ data: [{ name: "fs-mcp", tools: [{ name: "read" }, { name: "write" }] }] }),
}));
vi.mock("@entities/conversation", () => ({
  useConversations: () => ({ data: [{ id: "cv_1", title: "Chat 1" }] }),
}));
vi.mock("@entities/relation", () => ({
  useAllRelations: () => ({
    data: [{ id: "rel_1", userId: "u_1", fromKind: "function", fromId: "fn_1", toKind: "handler", toId: "hd_1", kind: "workflow_uses_function", createdAt: "", updatedAt: "" }],
  }),
}));

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

describe("useEntityDirectory", () => {
  it("includesAllEntityKinds", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const kinds = new Set(result.current.nodes.map((n) => n.kind));
    for (const k of ["function", "handler", "workflow", "document", "skill", "mcp", "conversation"]) {
      expect(kinds).toContain(k);
    }
  });

  it("functionNode_hasCorrectFields", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const fn = result.current.nodes.find((n) => n.id === "fn_1");
    expect(fn).toBeDefined();
    expect(fn?.kind).toBe("function");
    expect(fn?.label).toBe("addNums");
    expect(fn?.sub).toBe("adds");
  });

  it("skillNode_usesNameAsId", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const sk = result.current.nodes.find((n) => n.kind === "skill");
    expect(sk?.id).toBe("py-runner");
    expect(sk?.label).toBe("py-runner");
  });

  it("mcpNode_usesNameAsIdAndCountsTools", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const mc = result.current.nodes.find((n) => n.kind === "mcp");
    expect(mc?.id).toBe("fs-mcp");
  });

  it("edges_normalisedFromRelations", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const expected: EntityEdge = { from: "fn_1", to: "hd_1", kind: "workflow_uses_function" };
    expect(result.current.edges).toContainEqual(expected);
  });
});
