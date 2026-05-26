// EntityRelMeta — neighborhood query → dedupe + cap + empty-render skip.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";

vi.mock("@features/entity-link", () => ({
  useEntityNeighborhood: vi.fn(),
}));

vi.mock("./EntityLink.jsx", () => ({
  EntityLink: ({ id }) => <span data-testid="entity-link">{id}</span>,
}));

vi.mock("./RelGraph.jsx", () => ({
  RelMore: () => null,
}));

import { useEntityNeighborhood } from "@features/entity-link";
import { EntityRelMeta } from "./EntityRelMeta.jsx";

beforeEach(() => useEntityNeighborhood.mockReset());

describe("EntityRelMeta", () => {
  it("missingEntityId_rendersNothing", () => {
    useEntityNeighborhood.mockReturnValue({ neighbours: [], guessedKind: "function" });
    const { container } = render(<EntityRelMeta />);
    expect(container.firstChild).toBeNull();
  });

  it("zeroRelations_rendersNothing", () => {
    useEntityNeighborhood.mockReturnValue({ neighbours: [], guessedKind: "function" });
    const { container } = render(<EntityRelMeta entityId="fn_a" kind="function" />);
    expect(container.firstChild).toBeNull();
  });

  it("pickOtherSideOfEdge_byEntityIdComparison", () => {
    useEntityNeighborhood.mockReturnValue({ neighbours: ["fn_b", "fn_c"], guessedKind: "function" });
    const { getAllByTestId } = render(<EntityRelMeta entityId="fn_a" kind="function" />);
    const ids = getAllByTestId("entity-link").map((e) => e.textContent);
    expect(ids).toEqual(["fn_b", "fn_c"]);
  });

  it("dedupes_multiEdgePairs_listEachNeighbourOnce", () => {
    useEntityNeighborhood.mockReturnValue({ neighbours: ["fn_b"], guessedKind: "function" });
    const { getAllByTestId } = render(<EntityRelMeta entityId="fn_a" kind="function" />);
    expect(getAllByTestId("entity-link")).toHaveLength(1);
  });

  it("capsToLimit", () => {
    useEntityNeighborhood.mockReturnValue({ neighbours: ["fn_b", "fn_c"], guessedKind: "function" });
    const { getAllByTestId } = render(<EntityRelMeta entityId="fn_a" kind="function" limit={2} />);
    expect(getAllByTestId("entity-link")).toHaveLength(2);
  });

  it("noKind_guessedFromPrefix", () => {
    useEntityNeighborhood.mockReturnValue({ neighbours: ["fn_b"], guessedKind: "function" });
    render(<EntityRelMeta entityId="fn_a" />);
    expect(useEntityNeighborhood).toHaveBeenCalledWith("fn_a", undefined, 3);
  });
});
