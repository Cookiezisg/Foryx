// entities/handler api — query hooks coverage.
// forge.test.ts already covers accept/call/delete mutations.
// This file covers the query hooks (useHandlers, useHandler, useHandlerVersions,
// useHandlerConfig) and the reject mutation that was missing.

import { beforeEach, describe, expect, it } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { setupFetchSpy, renderQuery, renderMutation, makeClient, wrap, type FetchCall } from "../../../shared/api/_testHarness";
import {
  useHandlers,
  useHandler,
  useHandlerVersions,
  useHandlerConfig,
  useRejectHandler,
} from "./handler.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("handler query hooks", () => {
  it("useHandlers_fetchesHandlersList", async () => {
    const { result } = await renderQuery(useHandlers);
    expect(calls[0].url).toContain("/handlers");
    expect(calls[0].method).toBe("GET");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useHandler_fetchesSingleHandler", async () => {
    const { result } = await renderQuery(() => useHandler("hd_1"));
    expect(calls[0].url).toContain("/handlers/hd_1");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useHandlerVersions_fetchesVersionsList", async () => {
    const { result } = await renderQuery(() => useHandlerVersions("hd_1"));
    expect(calls[0].url).toContain("/handlers/hd_1/versions");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useHandlerConfig_fetchesConfig", async () => {
    const { result } = await renderQuery(() => useHandlerConfig("hd_1"));
    expect(calls[0].url).toContain("/handlers/hd_1/config");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useHandler_emptyId_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useHandler(""), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("handler reject mutation", () => {
  it("useRejectHandler_postsToPendingReject", async () => {
    const { result } = await renderMutation(useRejectHandler);
    result.current.mutate("hd_1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]).toMatchObject({ url: "/api/v1/handlers/hd_1/pending:reject", method: "POST" });
  });
});
