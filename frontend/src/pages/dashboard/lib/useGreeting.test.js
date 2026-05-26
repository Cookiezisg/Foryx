import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useGreeting } from "./useGreeting.js";
import { GREETINGS } from "./greetings.js";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-05-25T14:00:00"));
  vi.spyOn(Math, "random").mockReturnValue(0.5);
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useGreeting", () => {
  it("returns a non-empty string", () => {
    const { result } = renderHook(() => useGreeting({ hasRecentConv: false, displayName: "" }));
    expect(typeof result.current).toBe("string");
    expect(result.current.length).toBeGreaterThan(0);
  });

  it("substitutes {name} when displayName is set", () => {
    const idx = GREETINGS.findIndex((g) => g.text === "Your move, {name}.");
    vi.spyOn(Math, "random").mockImplementation(() => idx / GREETINGS.length + 1e-9);
    const { result } = renderHook(() => useGreeting({ hasRecentConv: false, displayName: "Weilin" }));
    expect(result.current).not.toContain("{name}");
  });

  it("never picks a {name}-bearing entry when displayName is empty", () => {
    for (let seed = 0; seed < 50; seed++) {
      vi.spyOn(Math, "random").mockReturnValue(seed / 50);
      const { result } = renderHook(() => useGreeting({ hasRecentConv: false, displayName: "" }));
      expect(result.current).not.toContain("{name}");
    }
  });

  it("memoizes — same inputs return same string", () => {
    const { result, rerender } = renderHook(
      (props) => useGreeting(props),
      { initialProps: { hasRecentConv: false, displayName: "" } }
    );
    const first = result.current;
    rerender({ hasRecentConv: false, displayName: "" });
    expect(result.current).toBe(first);
  });

  it("at night picks a G-night entry roughly half the time", () => {
    vi.setSystemTime(new Date("2026-05-25T23:30:00"));
    let nightHits = 0;
    const nightTexts = new Set(
      GREETINGS.filter((g) => g.tags.includes("G-night")).map((g) => g.text)
    );
    for (let seed = 0; seed < 100; seed++) {
      vi.spyOn(Math, "random").mockReturnValue(seed / 100);
      const { result } = renderHook(() => useGreeting({ hasRecentConv: false, displayName: "" }));
      if (nightTexts.has(result.current)) nightHits++;
    }
    expect(nightHits).toBeGreaterThan(30);
  });
});
