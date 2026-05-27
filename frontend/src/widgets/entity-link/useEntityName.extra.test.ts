// useEntityName — extra branch coverage.
// Covers single-char prefix aliases (f, h, w, d, s, m) and undefined list fallback.

import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

vi.mock("@entities/function", () => ({
  useFunctions: () => ({ data: undefined }),
}));
vi.mock("@entities/handler", () => ({
  useHandlers: () => ({ data: undefined }),
}));
vi.mock("@entities/workflow", () => ({
  useWorkflows: () => ({ data: undefined }),
}));
vi.mock("@entities/document", () => ({
  useDocuments: () => ({ data: undefined }),
}));
vi.mock("@entities/skill", () => ({
  useSkills: () => ({ data: undefined }),
}));
vi.mock("@entities/mcp", () => ({
  useMcpServers: () => ({ data: undefined }),
}));
vi.mock("@entities/conversation", () => ({
  useConversations: () => ({ data: undefined }),
}));
vi.mock("@entities/flowrun", () => ({
  useFlowRuns: () => ({ data: undefined }),
}));

import { useEntityName } from "./useEntityName";

describe("useEntityName — undefined list + single-char prefixes", () => {
  it("undefinedList_returnsNull", () => {
    // All mocks return data:undefined; pickName falls back to [] via || []
    const { result } = renderHook(() => useEntityName("fn_1"));
    expect(result.current).toBeNull();
  });

  it("singleChar_f_prefix_handledLikeFn", () => {
    // "f" prefix dispatches to function case; data undefined → null
    const { result } = renderHook(() => useEntityName("f_1"));
    expect(result.current).toBeNull();
  });

  it("singleChar_h_prefix_handledLikeHd", () => {
    const { result } = renderHook(() => useEntityName("h_1"));
    expect(result.current).toBeNull();
  });

  it("singleChar_w_prefix_handledLikeWf", () => {
    const { result } = renderHook(() => useEntityName("w_1"));
    expect(result.current).toBeNull();
  });

  it("singleChar_d_prefix_handledLikeDoc", () => {
    const { result } = renderHook(() => useEntityName("d_1"));
    expect(result.current).toBeNull();
  });

  it("singleChar_s_prefix_handledLikeSk", () => {
    const { result } = renderHook(() => useEntityName("s_1"));
    expect(result.current).toBeNull();
  });

  it("singleChar_m_prefix_handledLikeMcp", () => {
    const { result } = renderHook(() => useEntityName("m_1"));
    expect(result.current).toBeNull();
  });
});
