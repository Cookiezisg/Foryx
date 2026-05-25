// AskUserModal — opens on pendingAsk, submit POSTs answer, keyboard nav.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { setupFetchSpy } from "../../api/_testHarness.js";
import { useUIStore } from "../../store/ui.js";
import { AskUserModal } from "./AskUserModal.jsx";

let calls;
beforeEach(async () => {
  calls = setupFetchSpy();
  useUIStore.setState({ pendingAsk: null, askOpen: false, toasts: [] });
  const bridge = await import("../../bridge/wails.js");
  await bridge.initBaseUrl();
});

describe("AskUserModal", () => {
  it("noPendingAsk_andAskOpenFalse_rendersNothing", () => {
    const { container } = render(<AskUserModal />);
    expect(container.querySelector(".ask-card")).toBeNull();
  });

  it("askOpenWithoutPending_showsEmptyState", () => {
    useUIStore.setState({ askOpen: true });
    render(<AskUserModal />);
    expect(screen.getByText("agent 现在没在等你")).toBeInTheDocument();
  });

  it("pendingAsk_rendersQuestionAndOptions", () => {
    useUIStore.setState({
      pendingAsk: {
        id: "ask_1", conversationId: "cv_x", toolCallId: "tc_x",
        question: "选哪一个?",
        options: [{ id: "a", text: "选 A" }, { id: "b", text: "选 B" }],
      },
    });
    render(<AskUserModal />);
    expect(screen.getByText("选哪一个?")).toBeInTheDocument();
    expect(screen.getByText("选 A")).toBeInTheDocument();
    expect(screen.getByText("选 B")).toBeInTheDocument();
  });

  it("clickOption_selectsIt_visualHighlight", async () => {
    useUIStore.setState({
      pendingAsk: {
        id: "ask_2", conversationId: "cv_x", toolCallId: "tc_x",
        question: "?", options: [{ id: "a", text: "A" }],
      },
    });
    const { container } = render(<AskUserModal />);
    await userEvent.click(screen.getByText("A"));
    expect(container.querySelector(".ask-option.is-selected")).toBeTruthy();
  });

  it("submitButton_disabled_untilOptionSelected", () => {
    useUIStore.setState({
      pendingAsk: {
        id: "ask_3", conversationId: "cv_x", toolCallId: "tc_x",
        question: "?", options: [{ id: "a", text: "A" }],
      },
    });
    render(<AskUserModal />);
    const submit = screen.getByRole("button", { name: /提交/ });
    expect(submit.disabled).toBe(true);
  });

  it("submit_postsToResolveEndpoint", async () => {
    useUIStore.setState({
      pendingAsk: {
        id: "ask_4", conversationId: "cv_p", toolCallId: "tc_x",
        question: "?", options: [{ id: "ok", text: "OK" }],
      },
    });
    render(<AskUserModal />);
    await userEvent.click(screen.getByText("OK"));
    await userEvent.click(screen.getByRole("button", { name: /提交/ }));
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls[0].url).toContain("/conversations/cv_p/pending-questions/tc_x:resolve");
    expect(JSON.parse(calls[0].body)).toEqual({ answer: "ok" });
  });

  it("escapeKey_closesModal", async () => {
    useUIStore.setState({
      pendingAsk: { id: "ask_5", conversationId: "cv_x", toolCallId: "tc_x", question: "?", options: [] },
    });
    render(<AskUserModal />);
    await userEvent.keyboard("{Escape}");
    expect(useUIStore.getState().pendingAsk).toBeNull();
  });

  it("numericKey_selectsOptionByIndex", async () => {
    useUIStore.setState({
      pendingAsk: {
        id: "ask_6", conversationId: "cv_x", toolCallId: "tc_x", question: "?",
        options: [{ id: "first", text: "First" }, { id: "second", text: "Second" }],
      },
    });
    const { container } = render(<AskUserModal />);
    await userEvent.keyboard("2");
    expect(container.querySelectorAll(".ask-option.is-selected")).toHaveLength(1);
  });
});
