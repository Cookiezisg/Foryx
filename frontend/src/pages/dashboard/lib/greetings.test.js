import { describe, it, expect } from "vitest";
import { GREETINGS } from "./greetings.js";

describe("GREETINGS", () => {
  it("has 380 entries", () => {
    expect(GREETINGS.length).toBe(380);
  });

  it("every entry has text + tags", () => {
    for (const g of GREETINGS) {
      expect(typeof g.text).toBe("string");
      expect(g.text.length).toBeGreaterThan(0);
      expect(Array.isArray(g.tags)).toBe(true);
      expect(g.tags.length).toBeGreaterThan(0);
    }
  });

  it("all texts are unique", () => {
    const seen = new Set();
    for (const g of GREETINGS) {
      expect(seen.has(g.text)).toBe(false);
      seen.add(g.text);
    }
  });

  it("category M entries all contain {name}", () => {
    const m = GREETINGS.filter((g) => g.tags.includes("M"));
    expect(m.length).toBeGreaterThan(10);
    for (const g of m) expect(g.text).toContain("{name}");
  });

  it("name-free subset has at least 250 entries", () => {
    const free = GREETINGS.filter((g) => !g.text.includes("{name}"));
    expect(free.length).toBeGreaterThanOrEqual(250);
  });

  it("G has morning and night sub-tags", () => {
    expect(GREETINGS.some((g) => g.tags.includes("G-morning"))).toBe(true);
    expect(GREETINGS.some((g) => g.tags.includes("G-night"))).toBe(true);
  });
});
