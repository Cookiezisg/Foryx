// ObservePage — header + RelGraph shell. RelGraph itself is heavy (force
// sim, all-relations query, etc.) and tested in its own suite — stub here.

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("../../../widgets/rel-graph/RelGraph.jsx", () => ({
  RelGraph: () => <div data-testid="rel-graph-stub" />,
}));

import { ObservePage } from "./ObservePage.jsx";

describe("ObservePage", () => {
  it("renders_headerTitle洞察", () => {
    render(<ObservePage />);
    expect(screen.getByText("洞察")).toBeInTheDocument();
  });

  it("renders_subtitleHintingInteractions", () => {
    render(<ObservePage />);
    expect(screen.getByText(/实体之间的引用关系/)).toBeInTheDocument();
  });

  it("mountsRelGraph_inBody", () => {
    render(<ObservePage />);
    expect(screen.getByTestId("rel-graph-stub")).toBeInTheDocument();
  });

  it("pageRoot_usesPageContainerClass", () => {
    const { container } = render(<ObservePage />);
    expect(container.querySelector(".page")).toBeTruthy();
    expect(container.querySelector(".page-header")).toBeTruthy();
  });

  it("graphHost_setsMinHeightZero_soFlexCanShrink", () => {
    const { container } = render(<ObservePage />);
    const host = container.querySelector("[data-testid='rel-graph-stub']").parentElement;
    expect(host.style.minHeight).toBe("0px");
  });
});
