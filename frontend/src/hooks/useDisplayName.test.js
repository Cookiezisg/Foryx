import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useDisplayName } from "./useDisplayName.js";

describe("useDisplayName", () => {
  beforeEach(() => localStorage.clear());

  it("returns empty string when unset", () => {
    const { result } = renderHook(() => useDisplayName());
    expect(result.current[0]).toBe("");
  });

  it("reads from localStorage on mount", () => {
    localStorage.setItem("forgify.user.displayName", "Weilin");
    const { result } = renderHook(() => useDisplayName());
    expect(result.current[0]).toBe("Weilin");
  });

  it("persists on set", () => {
    const { result } = renderHook(() => useDisplayName());
    act(() => result.current[1]("Mia"));
    expect(localStorage.getItem("forgify.user.displayName")).toBe("Mia");
    expect(result.current[0]).toBe("Mia");
  });

  it("syncs across instances via storage event", () => {
    const a = renderHook(() => useDisplayName());
    const b = renderHook(() => useDisplayName());
    act(() => a.result.current[1]("Zoe"));
    expect(b.result.current[0]).toBe("Zoe");
  });
});
