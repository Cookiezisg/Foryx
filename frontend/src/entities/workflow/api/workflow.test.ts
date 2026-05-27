// entities/workflow api — query hooks coverage.
// forge.test.ts already covers accept/update/run/edit/capability mutations.
// This file covers query hooks + reject that were uncovered.

import { beforeEach, describe, expect, it } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { setupFetchSpy, renderQuery, renderMutation, makeClient, wrap, type FetchCall } from "../../../shared/api/_testHarness";
import {
  useWorkflows,
  useWorkflow,
  useWorkflowVersions,
  useRejectWorkflow,
  useDeleteWorkflow,
} from "./workflow.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("workflow query hooks", () => {
  it("useWorkflows_fetchesWorkflowList", async () => {
    const { result } = await renderQuery(useWorkflows);
    expect(calls[0].url).toContain("/workflows");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useWorkflow_fetchesSingleWorkflow", async () => {
    const { result } = await renderQuery(() => useWorkflow("wf_1"));
    expect(calls[0].url).toContain("/workflows/wf_1");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useWorkflowVersions_fetchesVersionsList", async () => {
    const { result } = await renderQuery(() => useWorkflowVersions("wf_1"));
    expect(calls[0].url).toContain("/workflows/wf_1/versions");
    expect(result.current.isSuccess).toBe(true);
  });

  it("useWorkflow_emptyId_disabled", () => {
    const client = makeClient();
    const { result } = renderHook(() => useWorkflow(""), { wrapper: wrap(client) });
    expect(calls).toHaveLength(0);
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("workflow mutations", () => {
  it("useRejectWorkflow_postsToPendingReject", async () => {
    const { result } = await renderMutation(useRejectWorkflow);
    result.current.mutate("wf_1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]).toMatchObject({ url: "/api/v1/workflows/wf_1/pending:reject", method: "POST" });
  });

  it("useDeleteWorkflow_deletesById", async () => {
    const { result } = await renderMutation(useDeleteWorkflow);
    result.current.mutate("wf_x");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]).toMatchObject({ url: "/api/v1/workflows/wf_x", method: "DELETE" });
  });
});
