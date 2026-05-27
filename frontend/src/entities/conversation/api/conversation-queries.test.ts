// entities/conversation/api — query hooks coverage.
// conversation.test.ts already covers mutation hooks.
// This file covers the query hooks (useConversations, useConversation,
// useConversationMessages) that were uncovered.

import { beforeEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { setupFetchSpy, renderQuery, makeClient, wrap, type FetchCall } from "../../../shared/api/_testHarness";
import {
  useConversations,
  useConversation,
  useConversationMessages,
} from "./conversation.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("conversation query hooks", () => {
  it("useConversations_fetchesConversationsList", async () => {
    const { result } = await renderQuery(useConversations);
    expect(calls[0].url).toContain("/conversations");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useConversation_fetchesSingleConversation", async () => {
    const { result } = await renderQuery(() => useConversation("cv_1"));
    expect(calls[0].url).toContain("/conversations/cv_1");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useConversation_nullId_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useConversation(null), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("useConversationMessages_fetchesMessages", async () => {
    const { result } = await renderQuery(() => useConversationMessages("cv_1"));
    expect(calls[0].url).toContain("/conversations/cv_1/messages");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useConversationMessages_nullId_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useConversationMessages(null), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });
});
