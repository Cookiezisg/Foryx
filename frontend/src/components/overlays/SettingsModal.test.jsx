// SettingsModal — shell + single-open accordion + account region.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

vi.mock("framer-motion", async () => {
  const actual = await vi.importActual("framer-motion");
  return {
    ...actual,
    AnimatePresence: ({ children }) => children,
    motion: new Proxy({}, {
      get: (_, tag) => (props) => {
        const { initial, animate, exit, transition, layout, ...rest } = props;
        return createElement(tag, rest);
      },
    }),
  };
});

vi.mock("../../api/users.js", () => ({
  useUsers: () => ({
    data: [
      { id: "u_a", username: "alice", displayName: "Alice" },
      { id: "u_b", username: "bob",   displayName: "Bob" },
    ],
  }),
  useUpdateUser: () => ({ mutate: vi.fn() }),
}));

const mockSwitchTo = vi.fn();
const mockAddAccount = vi.fn();
vi.mock("../../features/settings/index.js", () => ({
  useAccountManager: () => ({
    name: "",
    setName: vi.fn(),
    switchTo: mockSwitchTo,
    addAccount: mockAddAccount,
    isAdding: false,
  }),
}));

// ApiKeysSection and SearchSection are real components; stub their config hooks
// so they render deterministically without network.
vi.mock("../../api/config.js", () => ({
  useProviders: () => ({ data: [] }),
  useApiKeys: () => ({ data: [] }),
  useModelConfigs: () => ({ data: [] }),
  useCreateApiKey: () => ({ mutateAsync: vi.fn() }),
  useTestApiKey: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useDeleteApiKey: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUpsertModelConfig: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUpdateApiKey: (_id) => ({ mutate: vi.fn(), isPending: false }),
}));

import { useOverlayStore } from "@app/model";
import { useToastStore } from "../../shared/ui/toastStore.ts";
import { useSessionStore } from "../../entities/session/index.ts";
import { SettingsModal } from "./SettingsModal.jsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useOverlayStore.setState({ settingsOpen: true });
  useToastStore.setState({ toasts: [] });
  useSessionStore.setState({ currentUserId: "u_a" });
});

describe("SettingsModal", () => {
  it("closed_rendersNothing", () => {
    useOverlayStore.setState({ settingsOpen: false });
    const { container } = render(<SettingsModal />, { wrapper: wrap });
    expect(container.querySelector(".set-modal")).toBeNull();
  });

  it("open_showsAllFourSectionHeaders", () => {
    render(<SettingsModal />, { wrapper: wrap });
    expect(screen.getByText("API Keys")).toBeInTheDocument();
    expect(screen.getByText("网络搜索")).toBeInTheDocument();
    expect(screen.getByText("外观")).toBeInTheDocument();
    expect(screen.getByText("系统")).toBeInTheDocument();
  });

  it("open_showsSettingsTitle", () => {
    render(<SettingsModal />, { wrapper: wrap });
    expect(screen.getByText("设置")).toBeInTheDocument();
  });

  it("open_accountRegionShows", () => {
    render(<SettingsModal />, { wrapper: wrap });
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("defaultOpen_apiKeysBodyVisible", () => {
    const { container } = render(<SettingsModal />, { wrapper: wrap });
    // Keys section is open by default — its (real) body renders the add button.
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();
  });

  it("clickSearchHeader_opensSearch_closesKeys", async () => {
    const { container } = render(<SettingsModal />, { wrapper: wrap });

    // Initially keys is open — its body (add button) is present.
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();

    await userEvent.click(screen.getByText("网络搜索"));

    // Now search is open (real SearchSection: empty state + its own add button).
    expect(screen.getByText("还没有搜索密钥")).toBeInTheDocument();
    // Both sections are now real: search add button is visible.
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();

    // Reopening keys closes search — keys body (add button) remains.
    await userEvent.click(screen.getByText("API Keys"));
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();
    expect(screen.queryByText("还没有搜索密钥")).not.toBeInTheDocument();
  });

  it("closeButton_closesModal", async () => {
    const { container } = render(<SettingsModal />, { wrapper: wrap });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(container.querySelector(".set-x"));
    expect(useOverlayStore.getState().settingsOpen).toBe(false);
  });

  it("backdropClick_closesModal", async () => {
    const { container } = render(<SettingsModal />, { wrapper: wrap });
    await userEvent.click(container.querySelector(".set-scrim"));
    expect(useOverlayStore.getState().settingsOpen).toBe(false);
  });

  it("switchButton_showsUserList", async () => {
    render(<SettingsModal />, { wrapper: wrap });
    await userEvent.click(screen.getByText("切换 / 新建"));
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });
});
