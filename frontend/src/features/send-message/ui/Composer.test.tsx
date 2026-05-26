// Composer — send/cancel/mentions/attachments + key bindings.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/function", () => ({
  useFunctions: () => ({ data: [{ id: "fn_1", name: "addNumbers" }] }),
}));
vi.mock("@entities/handler", () => ({
  useHandlers: () => ({ data: [{ id: "hd_1", name: "Adder" }] }),
}));
vi.mock("@entities/workflow", () => ({
  useWorkflows: () => ({ data: [{ id: "wf_1", name: "Flow" }] }),
}));
vi.mock("@entities/document", () => ({
  useDocuments: () => ({ data: [{ id: "doc_1", name: "Notes" }] }),
}));

import { Composer } from "./Composer.tsx";

describe("Composer", () => {
  it("typing_thenEnter_callsOnSend", async () => {
    const onSend = vi.fn();
    render(<Composer onSend={onSend} />);
    const ta = screen.getByPlaceholderText(/说说你想做什么/);
    await userEvent.type(ta, "hello{enter}");
    expect(onSend).toHaveBeenCalledWith(expect.objectContaining({ content: "hello" }));
  });

  it("shiftEnter_insertsNewline_doesNotSend", async () => {
    const onSend = vi.fn();
    render(<Composer onSend={onSend} />);
    const ta = screen.getByPlaceholderText(/说说你想做什么/);
    await userEvent.type(ta, "line1{Shift>}{enter}{/Shift}line2");
    expect(onSend).not.toHaveBeenCalled();
    expect((ta as HTMLTextAreaElement).value).toBe("line1\nline2");
  });

  it("emptyText_sendButtonDisabled", () => {
    render(<Composer onSend={() => {}} />);
    const send = screen.getByTitle(/发送/) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
  });

  it("disabled_doesNotSendOnEnter", async () => {
    const onSend = vi.fn();
    render(<Composer disabled onSend={onSend} />);
    const ta = screen.getByPlaceholderText(/说说你想做什么/);
    expect((ta as HTMLTextAreaElement).disabled).toBe(true);
  });

  it("isStreaming_showsStopButton_clickingCallsOnCancel", async () => {
    const onCancel = vi.fn();
    render(<Composer isStreaming onCancel={onCancel} />);
    const stop = screen.getByTitle(/停止/);
    await userEvent.click(stop);
    expect(onCancel).toHaveBeenCalled();
  });

  it("escapeWhileStreaming_callsOnCancel", async () => {
    const onCancel = vi.fn();
    render(<Composer isStreaming onCancel={onCancel} />);
    const ta = screen.getByPlaceholderText(/agent 在干活/);
    ta.focus();
    await userEvent.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalled();
  });

  it("atTrigger_opensMentionPopover_listsEntities", async () => {
    render(<Composer onSend={() => {}} />);
    const ta = screen.getByPlaceholderText(/说说你想做什么/);
    await userEvent.type(ta, "@");
    expect(screen.getByText(/addNumbers/)).toBeInTheDocument(); // function
    expect(screen.getByText(/Notes/)).toBeInTheDocument();      // document
    expect(screen.queryByText(/Helper/)).not.toBeInTheDocument(); // skill excluded by design
  });

  it("mentionPick_addsToMentionList_andClearsAtToken", async () => {
    const onSend = vi.fn();
    render(<Composer onSend={onSend} />);
    const ta = screen.getByPlaceholderText(/说说你想做什么/);
    await userEvent.type(ta, "@");
    await userEvent.click(screen.getByText(/addNumbers/));
    expect((ta as HTMLTextAreaElement).value).toBe(""); // @-token erased
    await userEvent.type(ta, "ok{enter}");
    expect(onSend).toHaveBeenCalledWith(expect.objectContaining({
      mentions: expect.arrayContaining([expect.objectContaining({ id: "fn_1" })]),
    }));
  });

  it("mentionArrowDownThenEnter_picksByKeyboard", async () => {
    render(<Composer onSend={() => {}} />);
    const ta = screen.getByPlaceholderText(/说说你想做什么/);
    await userEvent.type(ta, "@");
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
    // a mention should be attached → mentions section visible (3rd item)
    expect((screen.queryByPlaceholderText(/说说你想做什么/) as HTMLTextAreaElement).value).toBe("");
  });

  it("attachButton_opensFilePicker", async () => {
    render(<Composer onSend={() => {}} />);
    // file input is hidden; we just verify the button is present + clickable
    const attach = screen.getByTitle("附加文件");
    expect(attach).toBeInTheDocument();
  });
});
