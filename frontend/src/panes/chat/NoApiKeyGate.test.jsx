// NoApiKeyGate — first-run empty state, click → open config pane.

import { beforeEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useUIStore } from "../../store/ui.js";
import { NoApiKeyGate } from "./NoApiKeyGate.jsx";

beforeEach(() => {
  useUIStore.setState({ openPanes: ["chat"], activeNarrowPane: null });
});

describe("NoApiKeyGate", () => {
  it("rendersHeadingAndConfigButton", () => {
    render(<NoApiKeyGate />);
    expect(screen.getByText(/先配一把钥匙/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /去配置/ })).toBeInTheDocument();
  });

  it("clickConfigButton_opensConfigPane", async () => {
    render(<NoApiKeyGate />);
    await userEvent.click(screen.getByRole("button", { name: /去配置/ }));
    expect(useUIStore.getState().openPanes).toContain("config");
  });

  it("noSecondaryButton_onlyOneActionButton", () => {
    render(<NoApiKeyGate />);
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(1);
  });
});
