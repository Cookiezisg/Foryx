// entities/skill api — useSkills + useSkill query hooks.

import { beforeEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { setupFetchSpy, renderQuery, makeClient, wrap, type FetchCall } from "../../../shared/api/_testHarness";
import { useSkills, useSkill } from "./skill.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("skill query hooks", () => {
  it("useSkills_fetchesSkillsList", async () => {
    const { result } = await renderQuery(useSkills);
    expect(calls[0].url).toContain("/skills");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useSkill_fetchesSingleSkill", async () => {
    const { result } = await renderQuery(() => useSkill("py-runner"));
    expect(calls[0].url).toContain("/skills/py-runner");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useSkill_emptyId_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useSkill(""), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });
});
