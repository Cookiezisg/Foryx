// ApiKeysSection — key-centric LLM key management. Tests: list renders when
// open, 对话默认 badge tracks model-config.chat, add-flow reveals grid →
// KeyVerifyField → ModelSelect on verify success.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockCreateKey = vi.fn();
const mockTestKey = vi.fn();
const mockDeleteKey = vi.fn();
const mockUpsertModel = vi.fn();

let apiKeys: any[] = [];
let modelConfigs: any[] = [];

vi.mock("@entities/apikey", () => ({
  useApiKeys: () => ({ data: apiKeys }),
  useCreateApiKey: () => ({ mutateAsync: mockCreateKey }),
  useTestApiKey: () => ({ mutate: mockTestKey, mutateAsync: mockTestKey, isPending: false }),
  useDeleteApiKey: () => ({ mutate: mockDeleteKey, mutateAsync: mockDeleteKey, isPending: false }),
}));

vi.mock("@entities/model-config", () => ({
  useProviders: () => ({ data: [
    { name: "deepseek", category: "llm", displayName: "DeepSeek", defaultBaseUrl: "https://api.deepseek.com" },
    { name: "anthropic", category: "llm", displayName: "Anthropic", defaultBaseUrl: "https://api.anthropic.com" },
    { name: "ollama", category: "llm", displayName: "Ollama (local)", defaultBaseUrl: "" },
    { name: "bocha", category: "search", displayName: "博查 Bocha", defaultBaseUrl: "https://api.bochaai.com/v1" },
  ] }),
  useModelConfigs: () => ({ data: modelConfigs }),
  useUpsertModelConfig: () => ({ mutate: mockUpsertModel, mutateAsync: mockUpsertModel, isPending: false }),
}));

import { useToastStore } from "@shared/ui/toastStore";
import { ApiKeysSection } from "./ApiKeysSection.tsx";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

const renderOpen = (props = {}) =>
  render(<ApiKeysSection open onToggle={() => {}} {...props} />, { wrapper: wrap });

// The add trigger and the section header both contain "API Key(s)"; target the
// dashed add button by its class.
const addBtn = () => document.querySelector(".set-addbtn");
const verifyBtn = () => screen.getByRole("button", { name: /验证/ });

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  mockCreateKey.mockReset().mockResolvedValue({ id: "aki_new" });
  mockTestKey.mockReset().mockResolvedValue({ ok: true, modelsFound: ["deepseek-chat", "deepseek-reasoner"] });
  mockDeleteKey.mockReset().mockResolvedValue({});
  mockUpsertModel.mockReset().mockResolvedValue({});
  apiKeys = [
    { id: "aki_ds", provider: "deepseek", displayName: "DeepSeek", keyMasked: "sk-ds...3f2a", testStatus: "ok", modelsFound: ["deepseek-chat", "deepseek-reasoner"] },
    { id: "aki_an", provider: "anthropic", displayName: "Anthropic", keyMasked: "sk-an...9c1d", testStatus: "pending", modelsFound: ["claude-sonnet-4-6", "claude-opus-4-7"] },
  ];
  modelConfigs = [{ scenario: "chat", provider: "deepseek", modelId: "deepseek-chat" }];
});

describe("ApiKeysSection", () => {
  it("open_rendersKeyListFilteredToLlm", () => {
    renderOpen();
    expect(screen.getByText("DeepSeek")).toBeInTheDocument();
    expect(screen.getByText("Anthropic")).toBeInTheDocument();
    expect(screen.getByText("sk-ds...3f2a")).toBeInTheDocument();
  });

  it("closed_rendersNoBody", () => {
    render(<ApiKeysSection open={false} onToggle={() => {}} />, { wrapper: wrap });
    expect(screen.queryByText("sk-ds...3f2a")).not.toBeInTheDocument();
  });

  it("chatDefaultBadge_onlyOnKeyMatchingModelConfigChat", () => {
    renderOpen();
    const badges = screen.getAllByText("对话默认");
    // One in the collapsed row (badge); detail is collapsed so the seg label is absent.
    expect(badges).toHaveLength(1);
    const row = badges[0].closest(".set-kitem");
    expect(row.querySelector(".set-pn").textContent).toBe("DeepSeek");
    expect(row.classList.contains("is-default")).toBe(true);
  });

  it("chatDefaultModelTag_showsModelConfigModelId", () => {
    renderOpen();
    expect(screen.getByText("deepseek-chat", { selector: ".set-mtag" })).toBeInTheDocument();
  });

  it("verifiedBadge_onKeyWithTestStatusOk", () => {
    renderOpen();
    const dsItem = screen.getByText("DeepSeek").closest(".set-kitem") as HTMLElement;
    expect(within(dsItem).queryByText("已验证")).toBeInTheDocument();
    const anItem = screen.getByText("Anthropic").closest(".set-kitem") as HTMLElement;
    expect(within(anItem).queryByText("已验证")).not.toBeInTheDocument();
  });

  it("rowClick_expandsDetailWithModelSelectAndSegmented", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("DeepSeek"));
    expect(screen.getByText("用途")).toBeInTheDocument();
    expect(screen.getByLabelText("模型")).toBeInTheDocument();
  });

  it("rowClick_singleOpenWithinSection", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("DeepSeek"));
    expect(screen.getByText("用途")).toBeInTheDocument();
    await userEvent.click(screen.getByText("Anthropic"));
    // Opening Anthropic closes DeepSeek → exactly one detail (one 用途 row).
    expect(screen.getAllByText("用途")).toHaveLength(1);
  });

  it("promoteNonDefaultKey_upsertsChatModelConfigWithKeysModel", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("Anthropic"));
    // Anthropic detail: 用途 seg → click 对话默认 promotes it with its first model.
    const seg = screen.getByText("对话默认", { selector: ".set-seg-opt" });
    await userEvent.click(seg);
    expect(mockUpsertModel).toHaveBeenCalled();
    expect(mockUpsertModel.mock.calls[0][0]).toMatchObject({
      scenario: "chat", provider: "anthropic", modelId: "claude-sonnet-4-6",
    });
  });

  it("promoteDisabled_whenNonDefaultKeyHasNoModel", async () => {
    apiKeys = [
      { id: "aki_ds", provider: "deepseek", displayName: "DeepSeek", keyMasked: "sk-ds...3f2a", testStatus: "ok", modelsFound: ["deepseek-chat"] },
      { id: "aki_an", provider: "anthropic", displayName: "Anthropic", keyMasked: "sk-an...9c1d", testStatus: "pending", modelsFound: [] },
    ];
    renderOpen();
    await userEvent.click(screen.getByText("Anthropic"));
    const seg = screen.getByText("对话默认", { selector: ".set-seg-opt" });
    expect((seg as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(seg);
    expect(mockUpsertModel).not.toHaveBeenCalled();
  });

  it("changeModelOnDefaultKey_upsertsChatModelConfig", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("DeepSeek"));
    await userEvent.click(screen.getByLabelText("模型"));
    await userEvent.click(screen.getByRole("option", { name: "deepseek-reasoner" }));
    expect(mockUpsertModel).toHaveBeenCalledWith(
      { scenario: "chat", provider: "deepseek", modelId: "deepseek-reasoner" },
    );
  });

  it("addButton_revealsProviderGrid", async () => {
    renderOpen();
    await userEvent.click(addBtn());
    expect(screen.getByText("添加 API Key")).toBeInTheDocument();
    // Grid shows LLM providers (existing keys get a ✓), search providers excluded.
    expect(screen.queryByText("博查 Bocha")).not.toBeInTheDocument();
    const grid = document.querySelector(".onb-grid");
    expect(grid).toBeInTheDocument();
  });

  it("pickProvider_revealsKeyVerifyField", async () => {
    apiKeys = [];
    modelConfigs = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("DeepSeek"));
    expect(screen.getByText("DeepSeek API Key")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("sk-…")).toBeInTheDocument();
  });

  it("verifySuccess_revealsModelSelectAndEnablesSave", async () => {
    apiKeys = [];
    modelConfigs = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText("sk-…"), { target: { value: "sk-test123" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await waitFor(() => expect(mockTestKey).toHaveBeenCalled());
    const select = await screen.findByLabelText("模型");
    expect(select).toHaveTextContent("deepseek-chat");
    expect((screen.getByRole("button", { name: "保存" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("addFirstKey_saveUpsertsChatConfigWhenNoDefaultExists", async () => {
    apiKeys = [];
    modelConfigs = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText("sk-…"), { target: { value: "sk-test123" } });
    await userEvent.click(verifyBtn());
    await screen.findByLabelText("模型");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(mockUpsertModel).toHaveBeenCalled());
    expect(mockUpsertModel.mock.calls[0][0]).toMatchObject({ scenario: "chat", provider: "deepseek", modelId: "deepseek-chat" });
  });

  it("addKey_saveSkipsUpsertWhenChatDefaultAlreadyExists", async () => {
    // DeepSeek is the chat default; add Anthropic (not yet a key) → no upsert.
    apiKeys = [{ id: "aki_ds", provider: "deepseek", displayName: "DeepSeek", keyMasked: "sk-ds...3f2a", testStatus: "ok", modelsFound: ["deepseek-chat"] }];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("Anthropic"));
    fireEvent.change(screen.getByPlaceholderText("sk-…"), { target: { value: "sk-test123" } });
    mockTestKey.mockResolvedValue({ ok: true, modelsFound: ["claude-sonnet-4-6"] });
    await userEvent.click(verifyBtn());
    await screen.findByLabelText("模型");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    expect(mockUpsertModel).not.toHaveBeenCalled();
  });

  it("verifyFails_showsInlineErrorNoModelSelect", async () => {
    apiKeys = [];
    modelConfigs = [];
    mockTestKey.mockReset().mockRejectedValue(new Error("HTTP 401"));
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText("sk-…"), { target: { value: "sk-bad" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/验证未通过/)).toBeInTheDocument());
    expect(screen.queryByLabelText("模型")).not.toBeInTheDocument();
    expect((screen.getByRole("button", { name: "保存" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("cancelAfterCreate_deletesOrphanKey", async () => {
    apiKeys = [];
    modelConfigs = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText("sk-…"), { target: { value: "sk-test123" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(mockDeleteKey).toHaveBeenCalledWith("aki_new");
  });

  it("switchProvider_deletesPriorOrphanKey", async () => {
    apiKeys = [];
    modelConfigs = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("DeepSeek"));
    fireEvent.change(screen.getByPlaceholderText("sk-…"), { target: { value: "sk-test123" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await userEvent.click(screen.getByText("Anthropic"));
    expect(mockDeleteKey).toHaveBeenCalledWith("aki_new");
  });
});
