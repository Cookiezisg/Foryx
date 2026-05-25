// SettingsPopover — theme/accent/density/lang quick-tweak + account switch.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

vi.mock("../../api/users.js", () => ({
  useUsers: () => ({ data: [
    { id: "u_a", username: "alice" },
    { id: "u_b", username: "bob" },
  ] }),
  useCreateUser: () => ({ mutateAsync: vi.fn(async ({ username }) => ({ id: "u_new", username })) }),
}));

import { useUIStore } from "../../store/ui.js";
import { useSettings } from "../../store/settings.js";
import { SettingsPopover } from "./SettingsPopover.jsx";

function wrap({ children }) {
  const client = new QueryClient();
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useUIStore.setState({ settingsPopOpen: true, toasts: [] });
  useSettings.setState({ theme: "system", accent: "claude", density: "cozy", lang: "zh", activeUserId: "u_a" });
});

describe("SettingsPopover", () => {
  it("closed_rendersNothing", () => {
    useUIStore.setState({ settingsPopOpen: false });
    const { container } = render(<SettingsPopover />, { wrapper: wrap });
    expect(container.querySelector(".settings-pop")).toBeNull();
  });

  it("opensWithThemeAccentDensityLangControls", () => {
    render(<SettingsPopover />, { wrapper: wrap });
    expect(screen.getByText("主题")).toBeInTheDocument();
    expect(screen.getByText("色调")).toBeInTheDocument();
    expect(screen.getByText("密度")).toBeInTheDocument();
    expect(screen.getByText("语言")).toBeInTheDocument();
  });

  it("clickThemeDark_updatesSettings", async () => {
    render(<SettingsPopover />, { wrapper: wrap });
    await userEvent.click(screen.getByText("暗"));
    expect(useSettings.getState().theme).toBe("dark");
  });

  it("clickDensityCompact_updatesSettings", async () => {
    render(<SettingsPopover />, { wrapper: wrap });
    await userEvent.click(screen.getByText("紧凑"));
    expect(useSettings.getState().density).toBe("compact");
  });

  it("clickLangEN_updatesSettings", async () => {
    render(<SettingsPopover />, { wrapper: wrap });
    await userEvent.click(screen.getByText("English"));
    expect(useSettings.getState().lang).toBe("en");
  });

  it("accountSection_showsActiveUsername", () => {
    render(<SettingsPopover />, { wrapper: wrap });
    expect(screen.getByText("alice")).toBeInTheDocument();
  });

  it("clickSwitch_showsUserList", async () => {
    render(<SettingsPopover />, { wrapper: wrap });
    await userEvent.click(screen.getByText("切换"));
    expect(screen.getAllByText("alice").length).toBeGreaterThan(1); // header + list row
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  it("switchToUser_updatesActiveUserId_pushesToast", async () => {
    render(<SettingsPopover />, { wrapper: wrap });
    await userEvent.click(screen.getByText("切换"));
    await userEvent.click(screen.getByText("bob"));
    expect(useSettings.getState().activeUserId).toBe("u_b");
    expect(useUIStore.getState().toasts[0]?.kind).toBe("success");
  });
});
