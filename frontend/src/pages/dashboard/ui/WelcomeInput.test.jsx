import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { WelcomeInput } from "./WelcomeInput.jsx";

describe("WelcomeInput", () => {
  it("renders with placeholder", () => {
    render(<WelcomeInput onSubmit={() => {}} />);
    expect(screen.getByPlaceholderText("Ask Forgify… or forge something")).toBeInTheDocument();
  });

  it("calls onSubmit with text on Enter", () => {
    const fn = vi.fn();
    render(<WelcomeInput onSubmit={fn} />);
    const input = screen.getByPlaceholderText("Ask Forgify… or forge something");
    fireEvent.change(input, { target: { value: "hello" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(fn).toHaveBeenCalledWith("hello");
  });

  it("does not submit on Shift+Enter (multi-line)", () => {
    const fn = vi.fn();
    render(<WelcomeInput onSubmit={fn} />);
    const input = screen.getByPlaceholderText("Ask Forgify… or forge something");
    fireEvent.change(input, { target: { value: "hello" } });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(fn).not.toHaveBeenCalled();
  });

  it("does not submit empty / whitespace-only", () => {
    const fn = vi.fn();
    render(<WelcomeInput onSubmit={fn} />);
    const input = screen.getByPlaceholderText("Ask Forgify… or forge something");
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(fn).not.toHaveBeenCalled();
  });

  it("disables while isSubmitting", () => {
    render(<WelcomeInput onSubmit={() => {}} isSubmitting={true} />);
    expect(screen.getByPlaceholderText("Ask Forgify… or forge something")).toBeDisabled();
  });
});
