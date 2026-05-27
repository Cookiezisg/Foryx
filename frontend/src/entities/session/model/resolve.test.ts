import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useSessionStore } from "./sessionStore";
import { resolveSession } from "./resolve";

vi.mock("../api/session");
import { fetchUsers } from "../api/session";
const mockFetchUsers = vi.mocked(fetchUsers);

function makeUser(id: string) {
  return { id, username: id, displayName: id, avatarColor: "blue", language: "en", lastUsedAt: null as any, createdAt: "", updatedAt: "" };
}

beforeEach(() => {
  useSessionStore.setState({ currentUserId: null, status: "loading" });
  vi.resetAllMocks();
});

afterEach(() => {
  // Flush the in-flight guard so each test starts clean.
  vi.resetAllMocks();
});

describe("resolveSession", () => {
  it("resolveSession_staleUserId_selectsFirstAndReady", async () => {
    useSessionStore.setState({ currentUserId: "u_gone", status: "loading" });
    mockFetchUsers.mockResolvedValue([makeUser("u_real")]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_real");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_emptyUsers_onboarding", async () => {
    mockFetchUsers.mockResolvedValue([]);

    await resolveSession();

    expect(useSessionStore.getState().status).toBe("onboarding");
  });

  it("resolveSession_validUserId_keepsAndReady", async () => {
    useSessionStore.setState({ currentUserId: "u_a", status: "loading" });
    mockFetchUsers.mockResolvedValue([makeUser("u_a"), makeUser("u_b")]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_a");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_nullUserId_selectsFirst", async () => {
    useSessionStore.setState({ currentUserId: null, status: "loading" });
    mockFetchUsers.mockResolvedValue([makeUser("u_x")]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_x");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  // Covers the old computeBootState invariants now inside resolveSession:

  it("resolveSession_setsLoadingFirst_thenSettles", async () => {
    useSessionStore.setState({ currentUserId: "u_a", status: "ready" });
    let resolvePromise!: (v: ReturnType<typeof makeUser>[]) => void;
    mockFetchUsers.mockReturnValue(new Promise((r) => { resolvePromise = r; }));

    const p = resolveSession();
    // Must immediately go to loading before the fetch resolves.
    expect(useSessionStore.getState().status).toBe("loading");

    resolvePromise([makeUser("u_a")]);
    await p;
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_multipleUsers_staleId_selectsFirst", async () => {
    useSessionStore.setState({ currentUserId: "u_dead" });
    mockFetchUsers.mockResolvedValue([makeUser("u_first"), makeUser("u_second")]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_first");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_idempotent_secondCallKeepsValidId", async () => {
    useSessionStore.setState({ currentUserId: "u_keep" });
    const users = [makeUser("u_keep"), makeUser("u_other")];
    mockFetchUsers.mockResolvedValue(users);

    await resolveSession();
    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_keep");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_noCurrentId_singleUser_selectsThat", async () => {
    useSessionStore.setState({ currentUserId: null });
    mockFetchUsers.mockResolvedValue([makeUser("u_only")]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_only");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_currentIdValidSecondOfTwo_keeps", async () => {
    useSessionStore.setState({ currentUserId: "u_b" });
    mockFetchUsers.mockResolvedValue([makeUser("u_a"), makeUser("u_b")]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_b");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_fetchError_propagates", async () => {
    mockFetchUsers.mockRejectedValue(new Error("network"));

    await expect(resolveSession()).rejects.toThrow("network");
    expect(useSessionStore.getState().status).toBe("loading");
  });

  it("resolveSession_currentIdMissingFromLargeList_selectsFirst", async () => {
    useSessionStore.setState({ currentUserId: "u_phantom" });
    const users = [makeUser("u_a"), makeUser("u_b"), makeUser("u_c")];
    mockFetchUsers.mockResolvedValue(users);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBe("u_a");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_concurrentCalls_deduplicateToOneNetworkRequest", async () => {
    mockFetchUsers.mockResolvedValue([makeUser("u_x")]);

    // Fire two concurrent calls without awaiting each other.
    await Promise.all([resolveSession(), resolveSession(), resolveSession()]);

    // Only one network request should have been made.
    expect(mockFetchUsers).toHaveBeenCalledTimes(1);
    expect(useSessionStore.getState().currentUserId).toBe("u_x");
    expect(useSessionStore.getState().status).toBe("ready");
  });

  it("resolveSession_emptyUsers_clearsCurrentUserIdBeforeOnboarding", async () => {
    useSessionStore.setState({ currentUserId: "u_stale" });
    mockFetchUsers.mockResolvedValue([]);

    await resolveSession();

    expect(useSessionStore.getState().currentUserId).toBeNull();
    expect(useSessionStore.getState().status).toBe("onboarding");
  });
});
