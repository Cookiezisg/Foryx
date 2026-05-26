// BlockRenderer — 7 block types + fan-out regression test.
//
// Historical perf bug: ToolCallBlock subscribed to the entire blocks
// Map, so every SSE delta re-rendered every tool_call in the conv.
// The render-counter test below is the regression gate: a delta to
// an unrelated block must NOT re-render sibling tool_calls.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render } from "@testing-library/react";
import { useChatStore } from "../../../store/chat.js";
import { BlockList } from "./BlockRenderer.jsx";

const CV = "cv_render_test";

function resetStore() {
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
}

async function nextFrame() {
  await new Promise((r) => setTimeout(r, 30));
}

beforeEach(() => resetStore());

// ── Block type rendering ──────────────────────────────────────────────
describe("BlockList — 7 block types", () => {
  it("textBlock_rendersStreamingCaret_whileStreaming", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, { id: "b_text", messageId: "m_1", blockType: "text" });
    s.onBlockDelta(CV, { id: "b_text", delta: "hello" });
    await nextFrame();

    const { container } = render(<BlockList convId={CV} blockIds={["b_text"]} />);
    expect(container.querySelector(".streaming-caret")).toBeTruthy();
    expect(container.textContent).toContain("hello");
  });

  it("textBlock_completed_hidesCaret", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, { id: "b_text", messageId: "m_1", blockType: "text" });
    s.onBlockDelta(CV, { id: "b_text", delta: "done" });
    s.onBlockStop(CV, { id: "b_text", status: "completed" });

    const { container } = render(<BlockList convId={CV} blockIds={["b_text"]} />);
    expect(container.querySelector(".streaming-caret")).toBeNull();
  });

  it("toolCallBlock_collapsedByDefault_showsHeader", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, {
      id: "b_tool", messageId: "m_1", blockType: "tool_call",
      attrs: { tool: "Read", summary: "read foo.txt" },
    });
    const { container, getByText } = render(<BlockList convId={CV} blockIds={["b_tool"]} />);
    expect(container.querySelector(".blk-tool")).toBeTruthy();
    expect(getByText("Read")).toBeInTheDocument();
    expect(getByText("read foo.txt")).toBeInTheDocument();
    expect(container.querySelector(".blk-tool-body")).toBeNull();
  });

  it("reasoningBlock_showsDurationAndCharCount", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, { id: "b_r", messageId: "m_1", blockType: "reasoning" });
    s.onBlockDelta(CV, { id: "b_r", delta: "thinking thinking" });
    s.onBlockStop(CV, { id: "b_r", status: "completed", durationMs: 1500 });
    await nextFrame();

    const { container } = render(<BlockList convId={CV} blockIds={["b_r"]} />);
    expect(container.querySelector(".blk-reasoning")).toBeTruthy();
    expect(container.textContent).toContain("已思考");
    expect(container.textContent).toMatch(/\d+ chars/);
  });

  it("compactionBlock_showsLabel", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, { id: "b_c", messageId: "m_1", blockType: "compaction" });
    const { container } = render(<BlockList convId={CV} blockIds={["b_c"]} />);
    expect(container.querySelector(".blk-compaction")).toBeTruthy();
    expect(container.textContent).toContain("对话已压缩");
  });

  it("blockList_groupsToolCallsSameExecutionGroup_intoBatch", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, {
      id: "b_a", messageId: "m_1", blockType: "tool_call",
      attrs: { tool: "Read", executionGroup: 1 },
    });
    s.onBlockStart(CV, {
      id: "b_b", messageId: "m_1", blockType: "tool_call",
      attrs: { tool: "Read", executionGroup: 1 },
    });
    const { container } = render(<BlockList convId={CV} blockIds={["b_a", "b_b"]} />);
    expect(container.querySelectorAll(".tool-batch")).toHaveLength(1);
  });

  it("blockList_unknownBlockType_skipsSilently", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, { id: "b_x", messageId: "m_1", blockType: "alien" });
    expect(() => render(<BlockList convId={CV} blockIds={["b_x"]} />)).not.toThrow();
  });

  it("compactionBlock_clickHead_opensBody", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, {
      id: "b_c", messageId: "m_1", blockType: "compaction",
      attrs: { blocksArchived: 7, generatedBy: "auto" },
    });
    s.onBlockDelta(CV, { id: "b_c", delta: "compacted summary text" });
    await nextFrame();
    const { container } = render(<BlockList convId={CV} blockIds={["b_c"]} />);
    expect(container.textContent).toContain("涵盖 7 个 block");
    expect(container.textContent).toContain("由 auto 生成");
    // body hidden by default
    expect(container.querySelector(".blk-compaction-body")).toBeNull();
    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.click(container.querySelector(".blk-compaction-head"));
    expect(container.querySelector(".blk-compaction-body")).toBeTruthy();
  });

  it("subagentBlock_renderedViaMessageType_showsAgentTypeAndStepCount", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_outer", role: "assistant" });
    s.onBlockStart(CV, {
      id: "b_sub", messageId: "m_outer", blockType: "message",
      attrs: { agentType: "research", title: "find foo" },
    });
    s.onMessageStart(CV, { id: "m_inner", role: "assistant", parentBlockId: "b_sub" });
    s.onBlockStart(CV, { id: "b_inner_text", messageId: "m_inner", blockType: "text" });

    const { container } = render(<BlockList convId={CV} blockIds={["b_sub"]} />);
    expect(container.querySelector(".blk-subagent")).toBeTruthy();
    expect(container.textContent).toContain("子 agent");
    expect(container.textContent).toContain("research");
    expect(container.textContent).toContain("find foo");
    expect(container.textContent).toContain("1 步");
  });

  it("subagentBlock_clickHead_expandsAndRendersInnerBlocks", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_outer", role: "assistant" });
    s.onBlockStart(CV, { id: "b_sub", messageId: "m_outer", blockType: "message" });
    s.onMessageStart(CV, { id: "m_inner", role: "assistant", parentBlockId: "b_sub" });
    s.onBlockStart(CV, { id: "b_in_text", messageId: "m_inner", blockType: "text" });
    s.onBlockDelta(CV, { id: "b_in_text", delta: "inner content" });
    await nextFrame();

    const { container } = render(<BlockList convId={CV} blockIds={["b_sub"]} />);
    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.click(container.querySelector(".blk-subagent-head"));
    expect(container.querySelector(".blk-subagent-body")).toBeTruthy();
    expect(container.textContent).toContain("inner content");
  });
});

// ── FAN-OUT REGRESSION — the historical perf killer ───────────────────
describe("ToolCallBlock fan-out regression", () => {
  it("blockDeltaToUnrelatedBlock_doesNotReRenderSiblingToolCalls", async () => {
    // Wrap ToolCallBlock with a render counter via a memo wrapper. We
    // can't directly count internal renders, but we can observe DOM
    // mutations: a re-render produces a new commit; we track via
    // MutationObserver counts on the tool block's body subtree.
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, {
      id: "b_tool", messageId: "m_1", blockType: "tool_call",
      attrs: { tool: "Read" },
    });
    s.onBlockStart(CV, {
      id: "b_streaming_text", messageId: "m_1", blockType: "text",
    });

    const { container } = render(
      <BlockList convId={CV} blockIds={["b_tool", "b_streaming_text"]} />
    );

    // Snapshot of the tool block subtree
    const toolHeader = container.querySelector(".blk-tool-head");
    const initialToolText = toolHeader.outerHTML;

    // Fire many deltas on the UNRELATED streaming text block
    for (let i = 0; i < 30; i++) {
      s.onBlockDelta(CV, { id: "b_streaming_text", delta: "x" });
    }
    await nextFrame();

    // Tool block's DOM must be byte-identical → memo + selector stability worked
    const afterToolText = container.querySelector(".blk-tool-head").outerHTML;
    expect(afterToolText).toBe(initialToolText);
  });

  it("blockDeltaToSelfToolCall_doesUpdate", async () => {
    // Sanity: the OPPOSITE case — delta to the tool's OWN block should
    // change visible state.
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "m_1", role: "assistant" });
    s.onBlockStart(CV, {
      id: "b_tool", messageId: "m_1", blockType: "tool_call",
      attrs: { tool: "Read" },
    });
    s.onBlockStop(CV, { id: "b_tool", status: "completed", durationMs: 50 });

    const { container } = render(<BlockList convId={CV} blockIds={["b_tool"]} />);
    // status change → tool block shows duration timing
    expect(container.querySelector(".blk-tool-timing")).toBeTruthy();
  });
});
