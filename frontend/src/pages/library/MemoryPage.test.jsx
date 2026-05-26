// MemoryPage — tab filter + list + pin/delete/edit drawer.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../api/library.js", () => ({
  useMemories: vi.fn(),
  useUpdateMemory: vi.fn(),
  useDeleteMemory: vi.fn(),
  usePinMemory: vi.fn(),
}));

import { useMemories, useUpdateMemory, useDeleteMemory, usePinMemory } from "../../api/library.js";
import { useToastStore } from "../../shared/ui/toastStore.ts";
import { MemoryPage } from "./MemoryPage.jsx";

const MEMORIES = [
  { name: "auto-push",        description: "push after commit",  memType: "user",     pinned: true,  source: "user" },
  { name: "doc-sync",         description: "doc lag = bug",      memType: "feedback", pinned: false, source: "user" },
  { name: "code-review-tier", description: "tier review style",  memType: "project",  pinned: false, source: "user" },
];

let updateMutate, delMutate, pinMutate;

beforeEach(() => {
  updateMutate = vi.fn((_args, opts) => opts?.onSuccess?.());
  delMutate    = vi.fn();
  pinMutate    = vi.fn((_args, opts) => opts?.onSuccess?.());
  useMemories.mockReturnValue({ data: MEMORIES, isLoading: false });
  useUpdateMemory.mockReturnValue({ mutate: updateMutate });
  useDeleteMemory.mockReturnValue({ mutate: delMutate });
  usePinMemory.mockReturnValue({ mutate: pinMutate });
  useToastStore.setState({ toasts: [] });
});

describe("MemoryPage", () => {
  it("loading_showsLoadingHint", () => {
    useMemories.mockReturnValue({ data: undefined, isLoading: true });
    render(<MemoryPage />);
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("emptyAllTab_showsAllEmptyCopy", () => {
    useMemories.mockReturnValue({ data: [], isLoading: false });
    render(<MemoryPage />);
    expect(screen.getByText("Memory 库还是空的")).toBeInTheDocument();
  });

  it("emptyTypedTab_showsTypeSpecificCopy", async () => {
    const { container } = render(<MemoryPage />);
    useMemories.mockReturnValue({ data: [], isLoading: false });
    const tabs = container.querySelectorAll(".page-tab");
    await userEvent.click(tabs[3]); // project
    expect(screen.getByText("没有 project 类型的 memory")).toBeInTheDocument();
  });

  it("populated_listsEachMemoryName", () => {
    render(<MemoryPage />);
    expect(screen.getByText("auto-push")).toBeInTheDocument();
    expect(screen.getByText("doc-sync")).toBeInTheDocument();
  });

  it("tabSwitch_callsUseMemoriesWithType", async () => {
    const { container } = render(<MemoryPage />);
    const tabs = container.querySelectorAll(".page-tab");
    await userEvent.click(tabs[2]); // feedback
    expect(useMemories).toHaveBeenCalledWith("feedback");
  });

  it("allTab_callsUseMemoriesWithUndefined", () => {
    render(<MemoryPage />);
    expect(useMemories).toHaveBeenCalledWith(undefined);
  });

  it("pinClick_callsPinMutate_pushesToast", async () => {
    const { container } = render(<MemoryPage />);
    const rows = container.querySelectorAll(".card");
    const pinBtn = rows[0].querySelectorAll(".icon-btn")[0];
    await userEvent.click(pinBtn);
    expect(pinMutate).toHaveBeenCalledWith(
      { name: "auto-push", pinned: false },
      expect.any(Object),
    );
    expect(useToastStore.getState().toasts[0]?.title).toBe("已取消 pin");
  });

  it("deleteClick_confirmed_callsDelete", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const { container } = render(<MemoryPage />);
    const rows = container.querySelectorAll(".card");
    const trashBtn = rows[0].querySelectorAll(".icon-btn")[1];
    await userEvent.click(trashBtn);
    expect(delMutate).toHaveBeenCalledWith("auto-push");
    confirmSpy.mockRestore();
  });

  it("deleteClick_declined_skipsDelete", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const { container } = render(<MemoryPage />);
    const rows = container.querySelectorAll(".card");
    const trashBtn = rows[0].querySelectorAll(".icon-btn")[1];
    await userEvent.click(trashBtn);
    expect(delMutate).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("rowClick_opensEditDrawerWithName", async () => {
    render(<MemoryPage />);
    await userEvent.click(screen.getByText("auto-push"));
    expect(screen.getAllByText("auto-push").length).toBeGreaterThan(1);
    expect(screen.getByText("保存")).toBeInTheDocument();
  });

  it("newButton_opensDrawerWithNameInput", async () => {
    render(<MemoryPage />);
    await userEvent.click(screen.getByText(/新建/));
    expect(screen.getByText("新建 Memory")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("kebab-case-slug")).toBeInTheDocument();
  });

  it("saveDrawer_callsUpdateMutate_pushesSuccessToast", async () => {
    render(<MemoryPage />);
    await userEvent.click(screen.getByText("auto-push"));
    await userEvent.click(screen.getByText("保存"));
    expect(updateMutate).toHaveBeenCalledWith(
      expect.objectContaining({ name: "auto-push", body: expect.any(Object) }),
      expect.any(Object),
    );
    expect(useToastStore.getState().toasts[0]?.title).toBe("已保存");
  });
});
