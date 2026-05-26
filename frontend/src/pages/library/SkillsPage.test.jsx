// SkillsPage — list of skill cards + search filter + detail drawer.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../api/library.js", () => ({
  useSkills: vi.fn(),
}));

import { useSkills } from "../../api/library.js";
import { SkillsPage } from "./SkillsPage.jsx";

const SKILLS = [
  { id: "sk_a", name: "code-review", description: "Review diff for bugs", tags: ["dev", "qa"], activated: true, body: "## How it works\nRun X then Y" },
  { id: "sk_b", name: "brainstorming", description: "Explore requirements upfront", tags: ["plan"], activated: false },
  { id: "sk_c", name: "verify", description: "Run the app to check behaviour", activated: false },
];

beforeEach(() => {
  useSkills.mockReturnValue({ data: SKILLS, isLoading: false });
});

describe("SkillsPage", () => {
  it("loading_showsLoadingHint", () => {
    useSkills.mockReturnValue({ data: undefined, isLoading: true });
    render(<SkillsPage />);
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("emptyList_showsEmptyStateWithHint", () => {
    useSkills.mockReturnValue({ data: [], isLoading: false });
    render(<SkillsPage />);
    expect(screen.getByText("还没有 Skill")).toBeInTheDocument();
  });

  it("populated_listsEachSkillByName", () => {
    render(<SkillsPage />);
    expect(screen.getByText("code-review")).toBeInTheDocument();
    expect(screen.getByText("brainstorming")).toBeInTheDocument();
    expect(screen.getByText("verify")).toBeInTheDocument();
  });

  it("activatedSkill_rendersActivatedBadge", () => {
    render(<SkillsPage />);
    expect(screen.getByText("已激活")).toBeInTheDocument();
  });

  it("countFooter_matchesFilteredLength", () => {
    render(<SkillsPage />);
    expect(screen.getByText("3 项")).toBeInTheDocument();
  });

  it("searchByName_filtersList", async () => {
    render(<SkillsPage />);
    const input = screen.getByPlaceholderText("搜技能名 / 描述…");
    await userEvent.type(input, "brain");
    expect(screen.queryByText("code-review")).toBeNull();
    expect(screen.getByText("brainstorming")).toBeInTheDocument();
    expect(screen.getByText("1 项")).toBeInTheDocument();
  });

  it("searchByDescription_matchesSubstring", async () => {
    render(<SkillsPage />);
    const input = screen.getByPlaceholderText("搜技能名 / 描述…");
    await userEvent.type(input, "diff");
    expect(screen.getByText("code-review")).toBeInTheDocument();
    expect(screen.queryByText("brainstorming")).toBeNull();
  });

  it("searchNoMatch_showsZeroCount", async () => {
    render(<SkillsPage />);
    const input = screen.getByPlaceholderText("搜技能名 / 描述…");
    await userEvent.type(input, "zzzz");
    expect(screen.getByText("0 项")).toBeInTheDocument();
  });

  it("cardClick_opensDrawerWithSkillBody", async () => {
    render(<SkillsPage />);
    await userEvent.click(screen.getByText("code-review"));
    expect(screen.getByText("Run X then Y", { exact: false })).toBeInTheDocument();
  });

  it("drawerClose_dismissesDrawer", async () => {
    const { container } = render(<SkillsPage />);
    await userEvent.click(screen.getByText("code-review"));
    expect(container.querySelector(".drawer")).toBeTruthy();
    const closeBtn = container.querySelector(".drawer .icon-btn");
    await userEvent.click(closeBtn);
    expect(container.querySelector(".drawer")).toBeNull();
  });

  it("tagPills_renderedUpToFive", () => {
    render(<SkillsPage />);
    expect(screen.getByText("dev")).toBeInTheDocument();
    expect(screen.getByText("qa")).toBeInTheDocument();
    expect(screen.getByText("plan")).toBeInTheDocument();
  });
});
