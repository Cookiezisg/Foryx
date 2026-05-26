// useOnboardingFlow — unit tests for orchestration logic.
// Covers: verify multi-step (keyId change judgement, fallback model), 6-step
// advancement, pickProvider orphan-key deletion, finish invalidation.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockCreateUser = vi.fn();
const mockCreateKey = vi.fn();
const mockTestKey = vi.fn();
const mockUpsertModel = vi.fn();
const mockDeleteKey = vi.fn();
const mockPushToast = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@entities/user", () => ({
  useCreateUser: () => ({ mutateAsync: mockCreateUser }),
}));

vi.mock("@entities/apikey", () => ({
  useCreateApiKey: () => ({ mutateAsync: mockCreateKey }),
  useTestApiKey: () => ({ mutateAsync: mockTestKey }),
  useDeleteApiKey: () => ({ mutate: mockDeleteKey, mutateAsync: mockDeleteKey }),
}));

vi.mock("@entities/model-config", () => ({
  useProviders: () => ({ data: [
    { name: "deepseek", category: "llm", displayName: "DeepSeek", defaultBaseUrl: "https://api.deepseek.com" },
    { name: "anthropic", category: "llm", displayName: "Anthropic", defaultBaseUrl: "https://api.anthropic.com" },
    { name: "bocha", category: "search", displayName: "博查 Bocha", defaultBaseUrl: "https://api.bochaai.com/v1" },
  ] }),
  useUpsertModelConfig: () => ({ mutateAsync: mockUpsertModel }),
}));

vi.mock("@shared/ui/toastStore", () => ({
  useToastStore: (sel: (s: { pushToast: typeof mockPushToast }) => unknown) =>
    sel({ pushToast: mockPushToast }),
}));

// eslint-disable-next-line boundaries/dependencies
vi.mock("../../../store/settings.js", () => {
  let state = { onboarded: false };
  return {
    useSettings: () => ({
      ...state,
      set: (patch: Partial<typeof state>) => { state = { ...state, ...patch }; },
    }),
    __resetState: () => { state = { onboarded: false }; },
  };
});

vi.mock("@entities/session", () => {
  const store = {
    currentUserId: null as string | null,
    status: "loading" as "loading" | "onboarding" | "ready",
    setCurrentUser: (id: string | null) => { store.currentUserId = id; },
    setStatus: (s: "loading" | "onboarding" | "ready") => { store.status = s; },
  };
  return {
    useSessionStore: Object.assign(
      (sel: (s: typeof store) => unknown) => sel(store),
      { getState: () => store, setState: (patch: Partial<typeof store>) => Object.assign(store, patch) }
    ),
  };
});

// eslint-disable-next-line boundaries/dependencies
vi.mock("../../../components/overlays/onboarding-strings.js", () => ({
  ACCENTS: [["claude", "#d97757"], ["blue", "#2383e2"]],
  PROVIDER_DEFAULT_MODEL: { anthropic: "claude-sonnet-4-6" },
}));

vi.mock("@shared/lib/i18n/index.js", () => ({
  default: { changeLanguage: vi.fn() },
}));

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual("@tanstack/react-query");
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

import { useOnboardingFlow } from "./useOnboardingFlow";

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockCreateUser.mockResolvedValue({ id: "u_1", username: "alice" });
  mockCreateKey.mockResolvedValue({ id: "aki_1" });
  mockTestKey.mockResolvedValue({ ok: true, modelsFound: ["deepseek-chat", "deepseek-reasoner"] });
  mockUpsertModel.mockResolvedValue({});
  mockDeleteKey.mockResolvedValue({});
});

describe("useOnboardingFlow", () => {
  it("initialState_stepZeroWelcome", () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    expect(result.current.step).toBe(0);
    expect(result.current.stepKey).toBe("welcome");
    expect(result.current.busy).toBe(false);
  });

  it("next_fromWelcome_advancesToWorkspace", () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.next(); });
    expect(result.current.step).toBe(1);
    expect(result.current.stepKey).toBe("workspace");
  });

  it("back_fromStep1_returnsToStep0", () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.next(); }); // 0→1 (welcome → workspace, synchronous default branch)
    expect(result.current.step).toBe(1);
    act(() => { result.current.back(); }); // 1→0
    expect(result.current.step).toBe(0);
  });

  it("canNext_workspace_falseWhenNameEmpty_trueWhenFilled", () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.next(); }); // → workspace
    expect(result.current.canNext()).toBe(false);
    act(() => { result.current.setName("alice"); });
    expect(result.current.canNext()).toBe(true);
  });

  it("workspace_next_createsUserAndAdvances", async () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.next(); }); // → workspace
    act(() => { result.current.setName("alice"); });
    act(() => { result.current.next(); }); // workspace next → create user + advance
    await waitFor(() => expect(mockCreateUser).toHaveBeenCalled());
    await waitFor(() => expect(result.current.step).toBe(2));
    expect(mockCreateUser.mock.calls[0][0].displayName).toBe("alice");
    expect(mockCreateUser.mock.calls[0][0].username).toBe("alice");
  });

  it("verify_createsKeyAndTests_populatesModels", async () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    // Navigate to model step
    act(() => { result.current.next(); }); // → workspace
    act(() => { result.current.setName("alice"); });
    act(() => { result.current.next(); }); // → appearance
    await waitFor(() => expect(result.current.step).toBe(2));
    act(() => { result.current.next(); }); // → model
    await waitFor(() => expect(result.current.step).toBe(3));

    act(() => { result.current.pickProvider("deepseek"); });
    act(() => { result.current.onKeyChange("sk-test"); });
    act(() => { result.current.verify(); });

    await waitFor(() => expect(result.current.verified).toBe(true));
    expect(result.current.models).toEqual(["deepseek-chat", "deepseek-reasoner"]);
    expect(result.current.modelId).toBe("deepseek-chat");
    expect(mockCreateKey).toHaveBeenCalledWith(expect.objectContaining({ provider: "deepseek", key: "sk-test" }));
    expect(mockTestKey).toHaveBeenCalledWith("aki_1");
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "success" }));
  });

  it("verify_keyChanged_deletesOldKeyAndCreatesNew", async () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    // Advance to model step
    act(() => { result.current.next(); }); // → workspace
    act(() => { result.current.setName("bob"); });
    act(() => { result.current.next(); }); // → appearance
    await waitFor(() => expect(result.current.step).toBe(2));
    act(() => { result.current.next(); }); // → model
    await waitFor(() => expect(result.current.step).toBe(3));

    act(() => { result.current.pickProvider("deepseek"); });
    act(() => { result.current.onKeyChange("sk-first"); });
    act(() => { result.current.verify(); });
    await waitFor(() => expect(result.current.verified).toBe(true));
    // Key id is now "aki_1". Change key text:
    act(() => { result.current.onKeyChange("sk-second"); });
    // Re-verify — old key should be deleted, new key created
    mockCreateKey.mockResolvedValue({ id: "aki_2" });
    act(() => { result.current.verify(); });
    await waitFor(() => expect(mockDeleteKey).toHaveBeenCalledWith("aki_1"));
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalledWith(expect.objectContaining({ key: "sk-second" })));
    await waitFor(() => expect(mockTestKey).toHaveBeenCalledWith("aki_2"));
  });

  it("verify_fallbackModelWhenNoModelsFound", async () => {
    // anthropic :test returns no modelsFound → use PROVIDER_DEFAULT_MODEL fallback
    mockTestKey.mockResolvedValue({ ok: true, modelsFound: [] });
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    // Advance to model
    act(() => { result.current.next(); }); // → workspace
    act(() => { result.current.setName("carol"); });
    act(() => { result.current.next(); }); // → appearance
    await waitFor(() => expect(result.current.step).toBe(2));
    act(() => { result.current.next(); }); // → model
    await waitFor(() => expect(result.current.step).toBe(3));

    act(() => { result.current.pickProvider("anthropic"); });
    act(() => { result.current.onKeyChange("sk-ant"); });
    act(() => { result.current.verify(); });

    await waitFor(() => expect(result.current.verified).toBe(true));
    expect(result.current.models).toEqual(["claude-sonnet-4-6"]);
    expect(result.current.modelId).toBe("claude-sonnet-4-6");
  });

  it("verify_testKeyFails_setsVerifyError", async () => {
    mockTestKey.mockRejectedValue(new Error("HTTP 401"));
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.next(); }); // → workspace
    act(() => { result.current.setName("dave"); });
    act(() => { result.current.next(); }); // → appearance
    await waitFor(() => expect(result.current.step).toBe(2));
    act(() => { result.current.next(); }); // → model
    await waitFor(() => expect(result.current.step).toBe(3));

    act(() => { result.current.pickProvider("deepseek"); });
    act(() => { result.current.onKeyChange("sk-bad"); });
    act(() => { result.current.verify(); });
    await waitFor(() => expect(result.current.verified).toBe(false));
    expect(result.current.verifyError).toBeTruthy();
  });

  it("pickProvider_deletesOrphanKey_resetsState", async () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.next(); }); // → workspace
    act(() => { result.current.setName("eve"); });
    act(() => { result.current.next(); }); // → appearance
    await waitFor(() => expect(result.current.step).toBe(2));
    act(() => { result.current.next(); }); // → model
    await waitFor(() => expect(result.current.step).toBe(3));

    // First pick + verify to create a key
    act(() => { result.current.pickProvider("deepseek"); });
    act(() => { result.current.onKeyChange("sk-abc"); });
    act(() => { result.current.verify(); });
    await waitFor(() => expect(result.current.verified).toBe(true));

    // Switch provider — should delete orphaned key
    act(() => { result.current.pickProvider("anthropic"); });
    expect(mockDeleteKey).toHaveBeenCalledWith("aki_1");
    expect(result.current.verified).toBe(false);
    expect(result.current.apiKey).toBe("");
    expect(result.current.models).toEqual([]);
  });

  it("finish_setsOnboardedAndInvalidatesQueries", async () => {
    const onFinish = vi.fn();
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    act(() => { result.current.finish(onFinish); });
    expect(mockInvalidateQueries).toHaveBeenCalled();
    expect(mockPushToast).toHaveBeenCalledWith(expect.objectContaining({ kind: "success" }));
    expect(onFinish).toHaveBeenCalled();
  });

  it("llmProviders_filtersOutMockAndCustom", () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    expect(result.current.llmProviders.every((p) => p.name !== "mock" && p.name !== "custom")).toBe(true);
    expect(result.current.llmProviders.map((p) => p.name)).toContain("deepseek");
  });

  it("searchProviders_filtersByCategorySearch", () => {
    const { result } = renderHook(() => useOnboardingFlow(), { wrapper });
    expect(result.current.searchProviders.every((p) => p.category === "search")).toBe(true);
    expect(result.current.searchProviders.map((p) => p.name)).toContain("bocha");
  });
});
