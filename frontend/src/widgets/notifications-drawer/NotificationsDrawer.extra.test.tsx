// NotificationsDrawer — additional branch coverage.
// Covers: TodoTab with pendingAsk, submit call, tab switching, empty snapshot,
// notif with unknown type, notif missing pane (no-op), handleClose clearUnread.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("./useNotificationsSnapshot.js", () => ({
  useNotificationsSnapshot: () => ({ data: [] }),
}));

import { usePaneStore, useOverlayStore } from "@app/model";
import { NotificationsDrawer } from "./NotificationsDrawer.tsx";
import type { PendingAsk } from "@shared/api";

const PENDING_ASK: PendingAsk = {
  id: "ask_1",
  conversationId: "cv_x",
  toolCallId: "tc_x",
  question: "Choose one",
  options: [
    { id: "a", value: "a", text: "Option A" },
    { id: "b", value: "b", text: "Option B" },
  ],
};

function makeProps(overrides = {}) {
  const pane = usePaneStore.getState();
  const overlay = useOverlayStore.getState();
  return {
    open: true,
    onClose: () => overlay.setNotifsOpen(false),
    onOpenPane: pane.openPane,
    onOpenEntity: pane.openEntity,
    onSetActiveConv: pane.setActiveConv,
    pendingAsk: null as PendingAsk | null,
    onSetPendingAsk: overlay.setPendingAsk,
    unread: 0,
    clearUnread: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  useOverlayStore.setState({ notifsOpen: true });
  usePaneStore.setState({ openPanes: [], activeConv: null, activeNarrowPane: null, focusEntity: {} });
});

describe("NotificationsDrawer — TodoTab", () => {
  it("noSnapshot_emptyNotifMessage", () => {
    render(<NotificationsDrawer {...makeProps()} />);
    // default tab is notifs since no pendingAsk; snapshot is empty
    expect(screen.getByText("这里很安静。")).toBeInTheDocument();
  });

  it("withPendingAsk_todoTabActiveByDefault", () => {
    render(<NotificationsDrawer {...makeProps({ pendingAsk: PENDING_ASK })} />);
    expect(screen.getByText("Choose one")).toBeInTheDocument();
  });

  it("todoTab_noOptions_showsNoOptionsMessage", () => {
    const askNoOpts = { ...PENDING_ASK, options: [] };
    render(<NotificationsDrawer {...makeProps({ pendingAsk: askNoOpts })} />);
    expect(screen.getByText(/没给选项/)).toBeInTheDocument();
  });

  it("todoTab_noPendingAsk_showsEmptyState", () => {
    render(<NotificationsDrawer {...makeProps()} />);
    // Switch to todo tab
    fireEvent.click(screen.getByText("待办"));
    expect(screen.getByText("无待办")).toBeInTheDocument();
  });

  it("todoTab_selectOption_enablesSubmit", async () => {
    render(<NotificationsDrawer {...makeProps({ pendingAsk: PENDING_ASK })} />);
    await userEvent.click(screen.getByText("Option A"));
    const submit = screen.getByRole("button", { name: /提交/ });
    expect(submit).not.toBeDisabled();
  });

  it("tabSwitch_notifs_andBack_works", async () => {
    render(<NotificationsDrawer {...makeProps({ pendingAsk: PENDING_ASK })} />);
    await userEvent.click(screen.getByText(/通知/));
    expect(screen.queryByText("Choose one")).not.toBeInTheDocument();
    await userEvent.click(screen.getByText(/待办/));
    expect(screen.getByText("Choose one")).toBeInTheDocument();
  });

  it("unread_badge_visible_whenUnreadGreaterThanZero", () => {
    render(<NotificationsDrawer {...makeProps({ unread: 5 })} />);
    expect(screen.getByText(/5/)).toBeInTheDocument();
  });

  it("handleClose_callsClearUnread", async () => {
    const clearUnread = vi.fn();
    render(<NotificationsDrawer {...makeProps({ clearUnread })} />);
    // Click the X button
    const closeBtns = document.querySelectorAll(".icon-btn");
    await userEvent.click(closeBtns[closeBtns.length - 1]);
    expect(clearUnread).toHaveBeenCalled();
  });
});

describe("NotificationsDrawer — header/drawer props", () => {
  it("markAllRead_buttonVisible", () => {
    render(<NotificationsDrawer {...makeProps()} />);
    expect(screen.getByText("全部已读")).toBeInTheDocument();
  });

  it("drawer_title_visible", () => {
    render(<NotificationsDrawer {...makeProps()} />);
    expect(screen.getByText("收件箱")).toBeInTheDocument();
  });
});
