// Onboarding — 6-step toB wizard. Tests navigation, validation, early user
// creation, live language switch, model verify+select, search skip, finish.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

// Strip framer-motion animation so each step renders synchronously.
vi.mock("framer-motion", async () => {
  const actual = await vi.importActual("framer-motion");
  return {
    ...actual,
    AnimatePresence: ({ children }) => children,
    motion: new Proxy({}, {
      get: (_, tag) => (props: any) => {
        const { initial, animate, exit, transition, layout, ...rest } = props;
        return createElement(tag as any, rest);
      },
    }),
  };
});

const mockCreateUser = vi.fn();
const mockCreateKey = vi.fn();
const mockTestKey = vi.fn();
const mockUpsertModel = vi.fn();
const mockDeleteKey = vi.fn();

// Mock entity hooks used by useOnboardingFlow.
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
    { name: "ollama", category: "llm", displayName: "Ollama (local)", defaultBaseUrl: "" },
    { name: "bocha", category: "search", displayName: "博查 Bocha", defaultBaseUrl: "https://api.bochaai.com/v1" },
    { name: "brave", category: "search", displayName: "Brave Search", defaultBaseUrl: "https://api.search.brave.com" },
  ] }),
  useUpsertModelConfig: () => ({ mutateAsync: mockUpsertModel }),
}));

import { useToastStore } from "@shared/ui/toastStore";
import { useSettingsStore } from "@entities/settings/model/settingsStore";
import { useSessionStore } from "@entities/session";
import { Onboarding } from "./Onboarding.tsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  useSettingsStore.setState({ theme: "system", accent: "claude", density: "cozy", lang: "zh", reasoningDefault: "collapsed" });
  useSessionStore.setState({ currentUserId: null, status: "loading" });
  mockCreateUser.mockReset().mockResolvedValue({ id: "u_new", username: "alice" });
  mockCreateKey.mockReset().mockResolvedValue({ id: "aki_1" });
  mockTestKey.mockReset().mockResolvedValue({ ok: true, modelsFound: ["deepseek-chat", "deepseek-reasoner"] });
  mockUpsertModel.mockReset().mockResolvedValue({});
  mockDeleteKey.mockReset().mockResolvedValue({});
});

const btn = (re) => screen.getByRole("button", { name: re });
const pane = () => document.querySelector(".onb-pane") as HTMLElement;
const inPane = (text: string) => within(pane()).getByText(text);

// Advance welcome → workspace → (fill name) → appearance, creating the user.
async function toAppearance(user?: any) {
  await userEvent.click(btn(/开始/));
  await userEvent.type(screen.getByPlaceholderText(/个人/), "alice");
  await userEvent.click(btn(/继续/));
  await waitFor(() => expect(mockCreateUser).toHaveBeenCalled());
  await waitFor(() => expect(inPane("外观与语言")).toBeInTheDocument());
}

describe("Onboarding", () => {
  it("welcome_renderedFirst_startAdvancesToWorkspace", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    expect(inPane("欢迎使用 Forgify")).toBeInTheDocument();
    await userEvent.click(btn(/开始/));
    expect(inPane("创建工作空间")).toBeInTheDocument();
  });

  it("workspace_emptyName_nextDisabled_filledEnabled", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await userEvent.click(btn(/开始/));
    expect((btn(/继续/) as HTMLButtonElement).disabled).toBe(true);
    await userEvent.type(screen.getByPlaceholderText(/个人/), "alice");
    expect((btn(/继续/) as HTMLButtonElement).disabled).toBe(false);
  });

  it("workspace_continue_createsUserAndSetsCurrentUserId", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    expect(mockCreateUser.mock.calls[0][0].displayName).toBe("alice");
    expect(useSessionStore.getState().currentUserId).toBe("u_new");
  });

  it("appearance_languageSwitch_isLive", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    await userEvent.click(screen.getByText("English"));
    expect(useSettingsStore.getState().lang).toBe("en");
    // Wizard copy flips live to English.
    expect(inPane("Appearance & language")).toBeInTheDocument();
  });

  it("appearance_swatchClick_updatesAccent", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    const swatches = document.querySelectorAll(".onb-swatch");
    expect(swatches.length).toBe(5);
    await userEvent.click(swatches[1]); // blue
    expect(useSettingsStore.getState().accent).toBe("blue");
  });

  it("model_verify_populatesModelsAndContinueWritesConfigFromSelection", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    await userEvent.click(btn(/继续/)); // appearance → model
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText(/sk-/), { target: { value: "sk-test123" } });
    await userEvent.click(btn(/验证/));
    await waitFor(() => expect(mockTestKey).toHaveBeenCalled());
    // Model dropdown appears, defaulting to modelsFound[0].
    const select = await screen.findByLabelText("模型");
    expect(select).toHaveTextContent("deepseek-chat");
    await userEvent.click(btn(/继续/)); // model → search, writes model-config
    await waitFor(() => expect(mockUpsertModel).toHaveBeenCalled());
    expect(mockUpsertModel.mock.calls[0][0]).toMatchObject({
      scenario: "chat", provider: "deepseek", modelId: "deepseek-chat",
    });
  });

  it("model_verifyFails_noModelConfigButStillAdvances", async () => {
    mockTestKey.mockReset().mockRejectedValue(new Error("HTTP 401"));
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    await userEvent.click(btn(/继续/));
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText(/sk-/), { target: { value: "sk-bad" } });
    await userEvent.click(btn(/验证/));
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await userEvent.click(btn(/继续/)); // advances (model optional)
    await waitFor(() => expect(inPane("联网搜索")).toBeInTheDocument());
    expect(mockUpsertModel).not.toHaveBeenCalled();
  });

  it("search_skip_goesToDone", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    await userEvent.click(btn(/继续/)); // → model
    await userEvent.click(btn(/继续/)); // → search (no model configured)
    await waitFor(() => expect(inPane("联网搜索")).toBeInTheDocument());
    await userEvent.click(btn(/跳过/));
    await waitFor(() => expect(inPane("设置完成")).toBeInTheDocument());
  });

  it("search_withKey_continueCreatesSearchKey", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await toAppearance();
    await userEvent.click(btn(/继续/)); // → model
    await userEvent.click(btn(/继续/)); // → search
    await userEvent.click(screen.getByText("博查 Bocha"));
    fireEvent.change(screen.getByPlaceholderText(/key/), { target: { value: "bocha-key" } });
    await userEvent.click(btn(/继续/));
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    expect(mockCreateKey.mock.calls.at(-1)[0]).toMatchObject({ provider: "bocha", key: "bocha-key" });
  });

  it("done_enter_setsStatusReadyAndCallsOnFinish", async () => {
    const onFinish = vi.fn();
    render(<Onboarding onFinish={onFinish} />, { wrapper: wrap });
    await toAppearance();
    await userEvent.click(btn(/继续/)); // → model
    await userEvent.click(btn(/继续/)); // → search
    await userEvent.click(btn(/跳过/)); // → done
    await userEvent.click(btn(/进入/));
    await waitFor(() => expect(useSessionStore.getState().status).toBe("ready"));
    expect(onFinish).toHaveBeenCalled();
  });

  it("back_fromWorkspace_returnsToWelcome", async () => {
    render(<Onboarding onFinish={() => {}} />, { wrapper: wrap });
    await userEvent.click(btn(/开始/));
    expect(inPane("创建工作空间")).toBeInTheDocument();
    await userEvent.click(btn(/上一步/));
    expect(inPane("欢迎使用 Forgify")).toBeInTheDocument();
  });
});
