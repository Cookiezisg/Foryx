// ChatHeader — title + id + model tag + close button.

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../../widgets/entity-rel-meta/EntityRelMeta.tsx", () => ({
  EntityRelMeta: (): null => null,
}));

import { ChatHeader } from "./ChatHeader.tsx";

describe("ChatHeader", () => {
  it("noConv_rendersNothing", () => {
    const { container } = render(<ChatHeader conv={null} />);
    expect(container.firstChild).toBeNull();
  });

  it("withConv_showsTitle_andId", () => {
    render(<ChatHeader conv={{ id: "cv_a", title: "Hello" }} />);
    expect(screen.getByText("Hello")).toBeInTheDocument();
    expect(screen.getByText("cv_a")).toBeInTheDocument();
  });

  it("noTitle_fallsBackToParenLabel", () => {
    render(<ChatHeader conv={{ id: "cv_a" }} />);
    expect(screen.getByText("(无标题)")).toBeInTheDocument();
  });

  it("modelDefault_whenMissing", () => {
    render(<ChatHeader conv={{ id: "cv_a" }} />);
    expect(screen.getByText("default")).toBeInTheDocument();
  });

  it("modelPresent_shownAsTagAndPrefix", () => {
    render(<ChatHeader conv={{ id: "cv_a", model: "gpt-4o" }} />);
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("GP")).toBeInTheDocument(); // 2-char provider chip
  });

  it("onClose_clickFiresCallback", async () => {
    const onClose = vi.fn();
    render(<ChatHeader conv={{ id: "cv_a" }} onClose={onClose} />);
    await userEvent.click(screen.getByTitle("关闭"));
    expect(onClose).toHaveBeenCalled();
  });

  it("noOnClose_doesNotRenderCloseButton", () => {
    render(<ChatHeader conv={{ id: "cv_a" }} />);
    expect(screen.queryByTitle("关闭")).toBeNull();
  });
});
