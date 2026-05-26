// SettingsModal — shell + single-open accordion + account region.
// Props-based: open/onClose passed directly (no store read).

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

vi.mock("@/api/users.js", () => ({
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
vi.mock("@features/settings", () => ({
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
vi.mock("@/api/config.js", () => ({
  useProviders: () => ({ data: [] }),
  useApiKeys: () => ({ data: [] }),
  useModelConfigs: () => ({ data: [] }),
  useCreateApiKey: () => ({ mutateAsync: vi.fn() }),
  useTestApiKey: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useDeleteApiKey: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUpsertModelConfig: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUpdateApiKey: (_id) => ({ mutate: vi.fn(), isPending: false }),
}));

import { useToastStore } from "@shared/ui/toastStore";
import { useSessionStore } from "@entities/session";
import { SettingsModal } from "./SettingsModal.jsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  useSessionStore.setState({ currentUserId: "u_a" });
});

describe("SettingsModal", () => {
  it("closed_rendersNothing", () => {
    const { container } = render(<SettingsModal open={false} onClose={() => {}} />, { wrapper: wrap });
    expect(container.querySelector(".set-modal")).toBeNull();
  });

  it("open_showsAllFourSectionHeaders", () => {
    render(<SettingsModal open onClose={() => {}} />, { wrapper: wrap });
    expect(screen.getByText("API Keys")).toBeInTheDocument();
    expect(screen.getByText("网络搜索")).toBeInTheDocument();
    expect(screen.getByText("外观")).toBeInTheDocument();
    expect(screen.getByText("系统")).toBeInTheDocument();
  });

  it("open_showsSettingsTitle", () => {
    render(<SettingsModal open onClose={() => {}} />, { wrapper: wrap });
    expect(screen.getByText("设置")).toBeInTheDocument();
  });

  it("open_accountRegionShows", () => {
    render(<SettingsModal open onClose={() => {}} />, { wrapper: wrap });
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("defaultOpen_apiKeysBodyVisible", () => {
    const { container } = render(<SettingsModal open onClose={() => {}} />, { wrapper: wrap });
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();
  });

  it("clickSearchHeader_opensSearch_closesKeys", async () => {
    const { container } = render(<SettingsModal open onClose={() => {}} />, { wrapper: wrap });

    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();

    await userEvent.click(screen.getByText("网络搜索"));

    expect(screen.getByText("还没有搜索密钥")).toBeInTheDocument();
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();

    await userEvent.click(screen.getByText("API Keys"));
    expect(container.querySelector(".set-addbtn")).toBeInTheDocument();
    expect(screen.queryByText("还没有搜索密钥")).not.toBeInTheDocument();
  });

  it("closeButton_callsOnClose", async () => {
    const onClose = vi.fn();
    const { container } = render(<SettingsModal open onClose={onClose} />, { wrapper: wrap });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(container.querySelector(".set-x"));
    expect(onClose).toHaveBeenCalled();
  });

  it("backdropClick_callsOnClose", async () => {
    const onClose = vi.fn();
    const { container } = render(<SettingsModal open onClose={onClose} />, { wrapper: wrap });
    await userEvent.click(container.querySelector(".set-scrim"));
    expect(onClose).toHaveBeenCalled();
  });

  it("switchButton_showsUserList", async () => {
    render(<SettingsModal open onClose={() => {}} />, { wrapper: wrap });
    await userEvent.click(screen.getByText("切换 / 新建"));
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });
});
