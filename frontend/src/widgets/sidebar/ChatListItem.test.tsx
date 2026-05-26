// ChatListItem — status dot variants + click → active conv + ActionMenu actions.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/conversation", () => ({
  useUpdateConversation: vi.fn(),
  useDeleteConversation: vi.fn(),
}));

import { useUpdateConversation, useDeleteConversation } from "@entities/conversation";
import { usePaneStore } from "@app/model";
import { useToastStore } from "../../shared/ui/toastStore.ts";
import { ChatListItem } from "./ChatListItem.tsx";

const mockUseUpdateConversation = useUpdateConversation as any;
const mockUseDeleteConversation = useDeleteConversation as any;

let updateMutate: any, delMutate: any;

beforeEach(() => {
  updateMutate = vi.fn();
  delMutate    = vi.fn((_id: any, opts: any) => opts?.onSuccess?.());
  mockUseUpdateConversation.mockReturnValue({ mutate: updateMutate });
  mockUseDeleteConversation.mockReturnValue({ mutate: delMutate });
  usePaneStore.setState({ activeConv: null, openPanes: [] });
  useToastStore.setState({ toasts: [] });
});

function renderItem(conv: any, extraProps = {}) {
  const pane = usePaneStore.getState();
  return render(<ChatListItem
    conv={conv}
    openPanes={pane.openPanes}
    activeConv={pane.activeConv}
    onSetActiveConv={pane.setActiveConv}
    onOpenPane={pane.openPane}
    {...extraProps}
  />);
}

describe("ChatListItem", () => {
  it("titlePresent_rendersTitleText", () => {
    renderItem({ id: "cv_a", title: "Hello" });
    expect(screen.getByText("Hello")).toBeInTheDocument();
  });

  it("titleMissing_fallsBackToParenLabel", () => {
    renderItem({ id: "cv_a" });
    expect(screen.getByText("(无标题)")).toBeInTheDocument();
  });

  it("idleStatus_rendersNoDot", () => {
    const { container } = renderItem({ id: "cv_a", title: "Hi", status: "idle" });
    expect(container.querySelector(".cv-dot")).toBeNull();
  });

  it("streamingStatus_rendersStreamingDot", () => {
    const { container } = renderItem({ id: "cv_a", title: "Hi", status: "streaming" });
    expect(container.querySelector(".cv-dot.is-streaming")).toBeTruthy();
  });

  it("approvalStatus_rendersApprovalDot", () => {
    const { container } = renderItem({ id: "cv_a", title: "Hi", status: "approval" });
    expect(container.querySelector(".cv-dot.is-approval")).toBeTruthy();
  });

  it("clickRow_setsActiveConv_andOpensChatPane", async () => {
    renderItem({ id: "cv_a", title: "Hi" });
    await userEvent.click(screen.getByText("Hi"));
    expect(usePaneStore.getState().activeConv).toBe("cv_a");
    expect(usePaneStore.getState().openPanes).toContain("chat");
  });

  it("clickRow_whenChatPaneAlreadyOpen_doesNotPushDuplicate", async () => {
    usePaneStore.setState({ openPanes: ["chat"], activeConv: "cv_other" });
    renderItem({ id: "cv_a", title: "Hi" });
    await userEvent.click(screen.getByText("Hi"));
    expect(usePaneStore.getState().openPanes.filter((p) => p === "chat").length).toBe(1);
    expect(usePaneStore.getState().activeConv).toBe("cv_a");
  });

  it("activeConvAndChatOpen_rendersIsActiveClass", () => {
    usePaneStore.setState({ openPanes: ["chat"], activeConv: "cv_a" });
    const { container } = renderItem({ id: "cv_a", title: "Hi" });
    expect(container.querySelector(".cv.is-active")).toBeTruthy();
  });

  it("activeConvButChatClosed_skipsIsActive", () => {
    usePaneStore.setState({ openPanes: ["forge"], activeConv: "cv_a" });
    const { container } = renderItem({ id: "cv_a", title: "Hi" });
    expect(container.querySelector(".cv.is-active")).toBeNull();
  });

  it("menuPinAction_callsUpdateWithPinnedToggled", async () => {
    renderItem({ id: "cv_a", title: "Hi", pinned: false });
    await userEvent.click(screen.getByTitle("对话操作"));
    await userEvent.click(screen.getByText("置顶"));
    expect(updateMutate).toHaveBeenCalledWith({ pinned: true }, expect.any(Object));
  });

  it("menuPinAction_whenPinned_labelSaysCancel", async () => {
    renderItem({ id: "cv_a", title: "Hi", pinned: true });
    await userEvent.click(screen.getByTitle("对话操作"));
    expect(screen.getByText("取消置顶")).toBeInTheDocument();
  });

  it("menuArchiveAction_callsUpdate_pushesToast", async () => {
    renderItem({ id: "cv_a", title: "Hi", archived: false });
    await userEvent.click(screen.getByTitle("对话操作"));
    await userEvent.click(screen.getByText("归档"));
    expect(updateMutate).toHaveBeenCalledWith({ archived: true }, expect.any(Object));
    updateMutate.mock.calls[0][1].onSuccess?.();
    expect(useToastStore.getState().toasts[0]?.title).toBe("已归档");
  });

  it("menuRename_promptCancel_skipsUpdate", async () => {
    const promptSpy = vi.spyOn(window, "prompt").mockReturnValue(null);
    renderItem({ id: "cv_a", title: "Hi" });
    await userEvent.click(screen.getByTitle("对话操作"));
    await userEvent.click(screen.getByText("重命名"));
    expect(updateMutate).not.toHaveBeenCalled();
    promptSpy.mockRestore();
  });

  it("menuRename_promptNewTitle_callsUpdate", async () => {
    const promptSpy = vi.spyOn(window, "prompt").mockReturnValue("新名");
    renderItem({ id: "cv_a", title: "Hi" });
    await userEvent.click(screen.getByTitle("对话操作"));
    await userEvent.click(screen.getByText("重命名"));
    expect(updateMutate).toHaveBeenCalledWith({ title: "新名" }, expect.any(Object));
    promptSpy.mockRestore();
  });

  it("menuDelete_confirmed_callsDelete_clearsActiveConvIfSelf_pushesToast", async () => {
    usePaneStore.setState({ activeConv: "cv_a", openPanes: ["chat"] });
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderItem({ id: "cv_a", title: "Hi" });
    await userEvent.click(screen.getByTitle("对话操作"));
    await userEvent.click(screen.getByText("删除"));
    expect(delMutate).toHaveBeenCalledWith("cv_a", expect.any(Object));
    expect(usePaneStore.getState().activeConv).toBeNull();
    expect(useToastStore.getState().toasts[0]?.title).toBe("已删除");
    confirmSpy.mockRestore();
  });

  it("menuDelete_declined_skipsDelete", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderItem({ id: "cv_a", title: "Hi" });
    await userEvent.click(screen.getByTitle("对话操作"));
    await userEvent.click(screen.getByText("删除"));
    expect(delMutate).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });
});
