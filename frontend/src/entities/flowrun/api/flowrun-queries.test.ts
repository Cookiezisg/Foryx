// entities/flowrun/api — query hooks coverage.
// flowrun.test.ts already covers mutation hooks.
// This file covers useFlowRuns, useFlowRun, useFlowRunNodes query hooks.

import { beforeEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { setupFetchSpy, renderQuery, makeClient, wrap, type FetchCall } from "../../../shared/api/_testHarness";
import { useFlowRuns, useFlowRun, useFlowRunNodes } from "./flowrun.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("flowrun query hooks", () => {
  it("useFlowRuns_fetchesFlowRunsList", async () => {
    const { result } = await renderQuery(useFlowRuns);
    expect(calls[0].url).toContain("/flowruns");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useFlowRun_fetchesSingleFlowRun", async () => {
    const { result } = await renderQuery(() => useFlowRun("fr_1"));
    expect(calls[0].url).toContain("/flowruns/fr_1");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useFlowRun_emptyId_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useFlowRun(""), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("useFlowRunNodes_fetchesNodes", async () => {
    const { result } = await renderQuery(() => useFlowRunNodes("fr_1"));
    expect(calls[0].url).toContain("/flowruns/fr_1/nodes");
    expect(result.current.isSuccess).toBe(true);
  });
});
