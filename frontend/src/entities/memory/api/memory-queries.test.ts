// entities/memory/api — query hooks coverage.
// library.test.ts covers mutations; this covers useMemories (with/without type)
// and useMemory query hooks.

import { beforeEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { setupFetchSpy, renderQuery, makeClient, wrap, type FetchCall } from "../../../shared/api/_testHarness";
import { useMemories, useMemory } from "./memory.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("memory query hooks", () => {
  it("useMemories_noType_fetchesAllMemories", async () => {
    const { result } = await renderQuery(() => useMemories());
    expect(calls[0].url).toContain("/memories");
    expect(calls[0].url).not.toContain("?type=");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useMemories_withType_appendsTypeParam", async () => {
    const { result } = await renderQuery(() => useMemories("user"));
    expect(calls[0].url).toContain("?type=user");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useMemory_fetchesSingleMemory", async () => {
    const { result } = await renderQuery(() => useMemory("my-note"));
    expect(calls[0].url).toContain("/memories/my-note");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useMemory_emptyName_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useMemory(""), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });
});
