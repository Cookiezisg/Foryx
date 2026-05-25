// ObservePane — header + RelGraph shell. RelGraph itself is heavy (force
// sim, all-relations query, etc.) and tested in its own suite — stub here.

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("../../components/shared/RelGraph.jsx", () => ({
  RelGraph: () => <div data-testid="rel-graph-stub" />,
}));

import { ObservePane } from "./ObservePane.jsx";

describe("ObservePane", () => {
  it("renders_headerTitle洞察", () => {
    render(<ObservePane />);
    expect(screen.getByText("洞察")).toBeInTheDocument();
  });

  it("renders_subtitleHintingInteractions", () => {
    render(<ObservePane />);
    expect(screen.getByText(/实体之间的引用关系/)).toBeInTheDocument();
  });

  it("mountsRelGraph_inBody", () => {
    render(<ObservePane />);
    expect(screen.getByTestId("rel-graph-stub")).toBeInTheDocument();
  });

  it("pageRoot_usesPageContainerClass", () => {
    const { container } = render(<ObservePane />);
    expect(container.querySelector(".page")).toBeTruthy();
    expect(container.querySelector(".page-header")).toBeTruthy();
  });

  it("graphHost_setsMinHeightZero_soFlexCanShrink", () => {
    const { container } = render(<ObservePane />);
    const host = container.querySelector("[data-testid='rel-graph-stub']").parentElement;
    expect(host.style.minHeight).toBe("0px");
  });
});
