// AskUserModal — opens on pendingAsk, submit POSTs answer, keyboard nav.
// Props-based: pending/askOpen/onClose passed directly (no store read).

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { setupFetchSpy } from "@/api/_testHarness.js";
import { useToastStore } from "@shared/ui/toastStore";
import { AskUserModal } from "./AskUserModal.jsx";

let calls;
beforeEach(async () => {
  calls = setupFetchSpy();
  useToastStore.setState({ toasts: [] });
  const bridge = await import("@/bridge/wails.js");
  await bridge.initBaseUrl();
});

const PENDING = (id = "ask_1") => ({
  id, conversationId: "cv_x", toolCallId: "tc_x",
  question: "选哪一个?",
  options: [{ id: "a", text: "选 A" }, { id: "b", text: "选 B" }],
});

describe("AskUserModal", () => {
  it("noPendingAsk_andAskOpenFalse_rendersNothing", () => {
    const { container } = render(<AskUserModal pending={null} askOpen={false} onClose={() => {}} />);
    expect(container.querySelector(".ask-card")).toBeNull();
  });

  it("askOpenWithoutPending_showsEmptyState", () => {
    render(<AskUserModal pending={null} askOpen={true} onClose={() => {}} />);
    expect(screen.getByText("agent 现在没在等你")).toBeInTheDocument();
  });

  it("pendingAsk_rendersQuestionAndOptions", () => {
    render(<AskUserModal pending={PENDING()} askOpen={false} onClose={() => {}} />);
    expect(screen.getByText("选哪一个?")).toBeInTheDocument();
    expect(screen.getByText("选 A")).toBeInTheDocument();
    expect(screen.getByText("选 B")).toBeInTheDocument();
  });

  it("clickOption_selectsIt_visualHighlight", async () => {
    const { container } = render(
      <AskUserModal pending={PENDING("ask_2")} askOpen={false} onClose={() => {}} />
    );
    await userEvent.click(screen.getByText("选 A"));
    expect(container.querySelector(".ask-option.is-selected")).toBeTruthy();
  });

  it("submitButton_disabled_untilOptionSelected", () => {
    render(<AskUserModal pending={PENDING("ask_3")} askOpen={false} onClose={() => {}} />);
    const submit = screen.getByRole("button", { name: /提交/ });
    expect(submit.disabled).toBe(true);
  });

  it("submit_postsToResolveEndpoint", async () => {
    const pending = {
      id: "ask_4", conversationId: "cv_p", toolCallId: "tc_x",
      question: "?", options: [{ id: "ok", text: "OK" }],
    };
    render(<AskUserModal pending={pending} askOpen={false} onClose={() => {}} />);
    await userEvent.click(screen.getByText("OK"));
    await userEvent.click(screen.getByRole("button", { name: /提交/ }));
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls[0].url).toContain("/conversations/cv_p/pending-questions/tc_x:resolve");
    expect(JSON.parse(calls[0].body)).toEqual({ answer: "ok" });
  });

  it("escapeKey_callsOnClose", async () => {
    const onClose = vi.fn();
    render(<AskUserModal pending={PENDING("ask_5")} askOpen={false} onClose={onClose} />);
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });

  it("numericKey_selectsOptionByIndex", async () => {
    const pending = {
      id: "ask_6", conversationId: "cv_x", toolCallId: "tc_x", question: "?",
      options: [{ id: "first", text: "First" }, { id: "second", text: "Second" }],
    };
    const { container } = render(<AskUserModal pending={pending} askOpen={false} onClose={() => {}} />);
    await userEvent.keyboard("2");
    expect(container.querySelectorAll(".ask-option.is-selected")).toHaveLength(1);
  });
});
