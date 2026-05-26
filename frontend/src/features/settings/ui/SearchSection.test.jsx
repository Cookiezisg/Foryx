// SearchSection — search-key management. Tests: list renders filtered to
// search providers; 搜索默认 badge tracks key.isDefault; isDefault toggle
// fires PATCH via useUpdateApiKey; add-flow reveals grid → KeyVerifyField
// (no ModelSelect); cancel cleans up orphan key.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockCreateKey = vi.fn();
const mockTestKey = vi.fn();
const mockDeleteKey = vi.fn();
const mockUpdateKey = vi.fn();

// apiFetch is used directly in AddPanel's save for best-effort isDefault PATCH.
vi.mock("@/api/client.js", async (importOriginal) => {
  const actual = await importOriginal();
  return { ...actual, apiFetch: vi.fn().mockResolvedValue({}) };
});

let apiKeys = [];

vi.mock("@/api/config.js", () => ({
  useProviders: () => ({
    data: [
      { name: "deepseek", category: "llm", displayName: "DeepSeek", defaultBaseUrl: "https://api.deepseek.com" },
      { name: "bocha", category: "search", displayName: "博查 Bocha", defaultBaseUrl: "https://api.bochaai.com/v1" },
      { name: "brave", category: "search", displayName: "Brave Search", defaultBaseUrl: "https://api.search.brave.com" },
      { name: "serper", category: "search", displayName: "Serper", defaultBaseUrl: "https://google.serper.dev" },
      { name: "tavily", category: "search", displayName: "Tavily", defaultBaseUrl: "https://api.tavily.com" },
    ],
  }),
  useApiKeys: () => ({ data: apiKeys }),
  useCreateApiKey: () => ({ mutateAsync: mockCreateKey }),
  useTestApiKey: () => ({ mutate: mockTestKey, mutateAsync: mockTestKey, isPending: false }),
  useDeleteApiKey: () => ({ mutate: mockDeleteKey, mutateAsync: mockDeleteKey, isPending: false }),
  // useUpdateApiKey is called per-key with an id; return a consistent mutation object.
  useUpdateApiKey: (_id) => ({ mutate: mockUpdateKey, isPending: false }),
}));

import { useToastStore } from "@shared/ui/toastStore";
import { SearchSection } from "./SearchSection.jsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

const renderOpen = (props = {}) =>
  render(<SearchSection open onToggle={() => {}} {...props} />, { wrapper: wrap });

const addBtn = () => document.querySelector(".set-addbtn");
const verifyBtn = () => screen.getByRole("button", { name: /验证/ });

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  mockCreateKey.mockReset().mockResolvedValue({ id: "aki_new" });
  mockTestKey.mockReset().mockResolvedValue({ ok: true });
  mockDeleteKey.mockReset().mockResolvedValue({});
  mockUpdateKey.mockReset().mockResolvedValue({});
  apiKeys = [
    { id: "aki_bc", provider: "bocha", displayName: "博查 Bocha", keyMasked: "sk-bc...3f2a", testStatus: "ok", isDefault: true },
    { id: "aki_br", provider: "brave", displayName: "Brave Search", keyMasked: "sk-br...9c1d", testStatus: "pending", isDefault: false },
  ];
});

describe("SearchSection", () => {
  it("open_rendersSearchKeyListFilteredToSearchProviders", () => {
    renderOpen();
    expect(screen.getByText("博查 Bocha")).toBeInTheDocument();
    expect(screen.getByText("Brave Search")).toBeInTheDocument();
    expect(screen.getByText("sk-bc...3f2a")).toBeInTheDocument();
    // LLM key is not in search providers list
    expect(screen.queryByText("DeepSeek")).not.toBeInTheDocument();
  });

  it("closed_rendersNoBody", () => {
    render(<SearchSection open={false} onToggle={() => {}} />, { wrapper: wrap });
    expect(screen.queryByText("sk-bc...3f2a")).not.toBeInTheDocument();
  });

  it("searchDefaultBadge_onlyOnKeyWithIsDefaultTrue", () => {
    renderOpen();
    const badges = screen.getAllByText("搜索默认");
    // One badge in the collapsed row for the isDefault key; Brave has none.
    expect(badges).toHaveLength(1);
    const row = badges[0].closest(".set-kitem");
    expect(row.querySelector(".set-pn").textContent).toBe("博查 Bocha");
    expect(row.classList.contains("is-default")).toBe(true);
  });

  it("noDefaultBadge_onKeyWithIsDefaultFalse", () => {
    renderOpen();
    const braveItem = screen.getByText("Brave Search").closest(".set-kitem");
    expect(within(braveItem).queryByText("搜索默认")).not.toBeInTheDocument();
    expect(braveItem.classList.contains("is-default")).toBe(false);
  });

  it("verifiedBadge_onKeyWithTestStatusOk", () => {
    renderOpen();
    const bcItem = screen.getByText("博查 Bocha").closest(".set-kitem");
    expect(within(bcItem).queryByText("已验证")).toBeInTheDocument();
    const brItem = screen.getByText("Brave Search").closest(".set-kitem");
    expect(within(brItem).queryByText("已验证")).not.toBeInTheDocument();
  });

  it("header_sub_showsDefaultKeyName_whenDefaultExists", () => {
    renderOpen();
    expect(screen.getByText("博查 Bocha · 搜索默认")).toBeInTheDocument();
  });

  it("header_sub_showsUnconfigured_whenNoSearchKeys", () => {
    apiKeys = [];
    renderOpen();
    expect(screen.getByText("未配置")).toBeInTheDocument();
  });

  it("rowClick_expandsDetailWithSegmentedToggle_noModelSelect", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("博查 Bocha"));
    expect(screen.getByText("用途")).toBeInTheDocument();
    expect(screen.getByText("搜索默认", { selector: ".set-seg-opt" })).toBeInTheDocument();
    expect(screen.getByText("仅备用", { selector: ".set-seg-opt" })).toBeInTheDocument();
    // No model select (search keys have no model)
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });

  it("rowClick_singleOpenWithinSection", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("博查 Bocha"));
    expect(screen.getByText("用途")).toBeInTheDocument();
    await userEvent.click(screen.getByText("Brave Search"));
    expect(screen.getAllByText("用途")).toHaveLength(1);
  });

  it("clickSearchDefault_onNonDefaultKey_firesUpdateWithIsDefaultTrue", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("Brave Search"));
    const seg = within(screen.getByText("Brave Search").closest(".set-kitem"))
      .getByText("搜索默认", { selector: ".set-seg-opt" });
    expect(seg.disabled).toBe(false);
    await userEvent.click(seg);
    expect(mockUpdateKey).toHaveBeenCalledWith(
      { isDefault: true },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("clickJiyuBei_onDefaultKey_firesUpdateWithIsDefaultFalse", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("博查 Bocha"));
    const seg = within(screen.getByText("博查 Bocha").closest(".set-kitem"))
      .getByText("仅备用", { selector: ".set-seg-opt" });
    expect(seg.disabled).toBe(false);
    await userEvent.click(seg);
    expect(mockUpdateKey).toHaveBeenCalledWith(
      { isDefault: false },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("searchDefaultSegBtn_disabled_onCurrentDefaultKey", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("博查 Bocha"));
    const seg = within(screen.getByText("博查 Bocha").closest(".set-kitem"))
      .getByText("搜索默认", { selector: ".set-seg-opt" });
    expect(seg.disabled).toBe(true);
  });

  it("jiyuBeiSegBtn_disabled_onNonDefaultKey", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("Brave Search"));
    const seg = within(screen.getByText("Brave Search").closest(".set-kitem"))
      .getByText("仅备用", { selector: ".set-seg-opt" });
    expect(seg.disabled).toBe(true);
  });

  it("addButton_revealsProviderGrid_withSearchProvidersOnly", async () => {
    renderOpen();
    await userEvent.click(addBtn());
    expect(screen.getByText("添加搜索服务")).toBeInTheDocument();
    const grid = document.querySelector(".onb-grid");
    expect(grid).toBeInTheDocument();
    // Search providers shown, LLM not shown
    expect(screen.queryByText("DeepSeek")).not.toBeInTheDocument();
  });

  it("pickProvider_revealsKeyVerifyField_withoutModelSelect", async () => {
    apiKeys = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("博查 Bocha"));
    expect(screen.getByText("博查 Bocha API Key")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("填入 API Key…")).toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });

  it("verifySuccess_enablesSaveButton_noModelSelect", async () => {
    apiKeys = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("博查 Bocha"));
    fireEvent.change(screen.getByPlaceholderText("填入 API Key…"), { target: { value: "bc-key-123" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await waitFor(() => expect(mockTestKey).toHaveBeenCalled());
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    const saveBtn = screen.getByRole("button", { name: "保存" });
    await waitFor(() => expect(saveBtn.disabled).toBe(false));
  });

  it("verifyFails_showsInlineError_saveDisabled", async () => {
    apiKeys = [];
    mockTestKey.mockReset().mockRejectedValue(new Error("HTTP 401"));
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("博查 Bocha"));
    fireEvent.change(screen.getByPlaceholderText("填入 API Key…"), { target: { value: "bad-key" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(screen.getByText(/验证未通过/)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "保存" }).disabled).toBe(true);
  });

  it("cancelAfterCreate_deletesOrphanKey", async () => {
    apiKeys = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("博查 Bocha"));
    fireEvent.change(screen.getByPlaceholderText("填入 API Key…"), { target: { value: "bc-key-123" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(mockDeleteKey).toHaveBeenCalledWith("aki_new");
  });

  it("switchProvider_deletesPriorOrphanKey", async () => {
    apiKeys = [];
    renderOpen();
    await userEvent.click(addBtn());
    await userEvent.click(screen.getByText("博查 Bocha"));
    fireEvent.change(screen.getByPlaceholderText("填入 API Key…"), { target: { value: "bc-key-123" } });
    await userEvent.click(verifyBtn());
    await waitFor(() => expect(mockCreateKey).toHaveBeenCalled());
    await userEvent.click(screen.getByText("Brave Search"));
    expect(mockDeleteKey).toHaveBeenCalledWith("aki_new");
  });
});
