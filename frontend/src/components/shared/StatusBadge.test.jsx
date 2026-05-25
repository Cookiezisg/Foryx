// StatusBadge — status → kind mapping + AI marker for pending/draft.

import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { StatusBadge } from "./StatusBadge.jsx";

describe("StatusBadge", () => {
  it("statusReady_rendersSuccessBadge", () => {
    const { container } = render(<StatusBadge status="ready" />);
    expect(container.querySelector(".badge.success")).toBeTruthy();
    expect(screen.getByText("就绪")).toBeInTheDocument();
  });

  it("statusFailed_rendersErrorBadge", () => {
    const { container } = render(<StatusBadge status="failed" />);
    expect(container.querySelector(".badge.error")).toBeTruthy();
  });

  it("statusPending_rendersWarnBadgePlusAIMarker", () => {
    const { container } = render(<StatusBadge status="pending" />);
    expect(container.querySelector(".badge.warn")).toBeTruthy();
    expect(container.querySelector(".forge-ai-mark")).toBeTruthy();
  });

  it("statusDraft_rendersInfoBadgePlusAIMarker", () => {
    const { container } = render(<StatusBadge status="draft" />);
    expect(container.querySelector(".badge.info")).toBeTruthy();
    expect(container.querySelector(".forge-ai-mark")).toBeTruthy();
  });

  it("unknownStatus_rendersAsNeutralLabel", () => {
    render(<StatusBadge status="weird" />);
    expect(screen.getByText("weird")).toBeInTheDocument();
  });
});
