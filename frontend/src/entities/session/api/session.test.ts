// entities/session api — fetchUsers smoke test.

import { beforeEach, describe, expect, it } from "vitest";
import { setupFetchSpy, type FetchCall } from "../../../shared/api/_testHarness";
import { fetchUsers } from "./session.js";

let calls: FetchCall[];
beforeEach(async () => {
  calls = setupFetchSpy();
  const bridge = await import("../../../shared/bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("fetchUsers", () => {
  it("getsFetchesUsersEndpoint", async () => {
    await fetchUsers();
    expect(calls[0].url).toContain("/users");
    expect(calls[0].method).toBe("GET");
  });
});
