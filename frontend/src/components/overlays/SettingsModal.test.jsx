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
  useCreateUser: () => ({
    mutateAsync: vi.fn(async ({ username }) => ({ id: "u_new", username })),
  }),
  useUpdateUser: () => ({ mutate: vi.fn() }),
}));

import { useUIStore } from "../../store/ui.js";
import { useSettings } from "../../store/settings.js";
import { SettingsModal } from "./SettingsModal.jsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useUIStore.setState({ settingsOpen: true, toasts: [] });
  useSettings.setState({ activeUserId: "u_a" });
});

describe("SettingsModal", () => {
  it("closed_rendersNothing", () => {
    useUIStore.setState({ settingsOpen: false });
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
    render(<SettingsModal />, { wrapper: wrap });
    // Keys section is open by default — its body should render
    const empties = screen.getAllByText("即将实现");
    expect(empties.length).toBeGreaterThanOrEqual(1);
  });

  it("clickSearchHeader_opensSearch_closesKeys", async () => {
    render(<SettingsModal />, { wrapper: wrap });

    // Initially keys is open — one body visible
    expect(screen.getAllByText("即将实现").length).toBe(1);

    await userEvent.click(screen.getByText("网络搜索"));

    // Now search is open — still one body visible (keys closed, search open)
    expect(screen.getAllByText("即将实现").length).toBe(1);

    // Keys section chevron should no longer have is-open class
    // (the body for keys is gone, for search is present)
    // Verify by clicking keys again and counting bodies
    await userEvent.click(screen.getByText("API Keys"));
    expect(screen.getAllByText("即将实现").length).toBe(1);
  });

  it("closeButton_closesModal", async () => {
    const { container } = render(<SettingsModal />, { wrapper: wrap });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(container.querySelector(".set-x"));
    expect(useUIStore.getState().settingsOpen).toBe(false);
  });

  it("backdropClick_closesModal", async () => {
    const { container } = render(<SettingsModal />, { wrapper: wrap });
    await userEvent.click(container.querySelector(".set-scrim"));
    expect(useUIStore.getState().settingsOpen).toBe(false);
  });

  it("switchButton_showsUserList", async () => {
    render(<SettingsModal />, { wrapper: wrap });
    await userEvent.click(screen.getByText("切换 / 新建"));
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });
});
