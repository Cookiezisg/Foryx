// NoApiKeyGate — first-run empty state, click → calls onOpenSettings prop.

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NoApiKeyGate } from "./NoApiKeyGate.jsx";

describe("NoApiKeyGate", () => {
  it("rendersHeadingAndConfigButton", () => {
    render(<NoApiKeyGate onOpenSettings={vi.fn()} />);
    expect(screen.getByText(/先配一把钥匙/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /去配置/ })).toBeInTheDocument();
  });

  it("clickConfigButton_callsOnOpenSettings", async () => {
    const onOpenSettings = vi.fn();
    render(<NoApiKeyGate onOpenSettings={onOpenSettings} />);
    await userEvent.click(screen.getByRole("button", { name: /去配置/ }));
    expect(onOpenSettings).toHaveBeenCalledOnce();
  });

  it("noSecondaryButton_onlyOneActionButton", () => {
    render(<NoApiKeyGate onOpenSettings={vi.fn()} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(1);
  });
});
