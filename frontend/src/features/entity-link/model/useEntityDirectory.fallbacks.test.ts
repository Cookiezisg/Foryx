// useEntityDirectory — fallback branch coverage (no name/description/title).
// Separate file so vi.mock overrides don't conflict with main test.

import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

import { useEntityDirectory } from "./useEntityDirectory";

vi.mock("@entities/function", () => ({
  useFunctions: () => ({ data: [{ id: "fn_no_name" }, { id: "fn_no_desc", name: "fn" }] }),
}));
vi.mock("@entities/handler", () => ({
  useHandlers: () => ({ data: [{ id: "hd_no_desc", name: "hd" }] }),
}));
vi.mock("@entities/workflow", () => ({
  useWorkflows: () => ({ data: [{ id: "wf_no_desc", name: "wf" }] }),
}));
vi.mock("@entities/document", () => ({
  useDocuments: () => ({ data: [{ id: "doc_no_name" }] }),
}));
vi.mock("@entities/skill", () => ({
  useSkills: () => ({ data: [{ name: "sk", description: "" }] }),
}));
vi.mock("@entities/mcp", () => ({
  useMcpServers: () => ({ data: [{ name: "mcp-srv", tools: [] }] }),
}));
vi.mock("@entities/conversation", () => ({
  useConversations: () => ({ data: [{ id: "cv_no_title" }] }),
}));
vi.mock("@entities/relation", () => ({
  useAllRelations: () => ({ data: [] }),
}));

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

describe("useEntityDirectory — fallback branches", () => {
  it("functionNode_noName_fallsBackToId", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const fn = result.current.nodes.find((n) => n.id === "fn_no_name");
    expect(fn).toBeDefined();
    expect(fn?.label).toBe("fn_no_name");
  });

  it("documentNode_noName_fallsBackToId", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const doc = result.current.nodes.find((n) => n.id === "doc_no_name");
    expect(doc).toBeDefined();
    expect(doc?.label).toBe("doc_no_name");
  });

  it("conversationNode_noTitle_fallsBackToId", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const cv = result.current.nodes.find((n) => n.id === "cv_no_title");
    expect(cv).toBeDefined();
    expect(cv?.label).toBe("cv_no_title");
  });

  it("functionNode_noDescription_emptyString", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const fn = result.current.nodes.find((n) => n.id === "fn_no_desc");
    expect(fn?.sub).toBe("");
  });

  it("mcpNode_noTools_zeroCount", () => {
    const { result } = renderHook(() => useEntityDirectory(), { wrapper });
    const mc = result.current.nodes.find((n) => n.kind === "mcp");
    expect(mc).toBeDefined();
  });
});
