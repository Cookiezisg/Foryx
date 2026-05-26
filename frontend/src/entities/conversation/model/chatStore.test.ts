// entities/conversation/model/chatStore — SSE event sequence + hydrate-once
// guard + rAF coalescing + fan-out regression (selectors must return stable
// refs for unrelated blocks so memoed components don't re-render).
//
// Historical bugs covered here:
//   - hydrateConv overwriting live SSE state on conv switch
//   - selectBlock / selectChildIds returning new refs on every delta
//   - block_delta firing setState per event (no batching → render flood)
//
// Migrated from src/store/chat.test.js (4b.5 recovery).

import { beforeEach, describe, expect, it } from "vitest";
import {
  useChatStore,
  selectBlock,
  selectChildIds,
  selectTopMessageIds,
} from "./chatStore";

const CV = "cv_test";

function resetStore() {
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
}

async function nextFrame() {
  return new Promise((resolve) => setTimeout(resolve, 25));
}

beforeEach(() => resetStore());

describe("ensureConv / resetConv / resetAll", () => {
  it("ensureConv_newConvId_createsEmptyConv", () => {
    useChatStore.getState().ensureConv(CV);
    const c = useChatStore.getState().convs[CV];
    expect(c).toBeTruthy();
    expect(c.messages.size).toBe(0);
    expect(c.blocks.size).toBe(0);
    expect(c.topMsgIds).toEqual([]);
  });

  it("ensureConv_existingConvId_keepsReferenceStable", () => {
    useChatStore.getState().ensureConv(CV);
    const c1 = useChatStore.getState().convs[CV];
    useChatStore.getState().ensureConv(CV);
    const c2 = useChatStore.getState().convs[CV];
    expect(c2).toBe(c1);
  });

  it("resetConv_clearsHydrateFlag", () => {
    useChatStore.getState().hydrateConv(CV, []);
    expect(useChatStore.getState().hydratedConvs.has(CV)).toBe(true);
    useChatStore.getState().resetConv(CV);
    expect(useChatStore.getState().hydratedConvs.has(CV)).toBe(false);
  });

  it("resetAll_clearsAllConvsAndHydrateSet", () => {
    useChatStore.getState().hydrateConv(CV, []);
    useChatStore.getState().resetAll();
    expect(useChatStore.getState().convs).toEqual({});
    expect(useChatStore.getState().hydratedConvs.size).toBe(0);
  });
});

describe("hydrateConv — hydrate-once guard", () => {
  it("hydrateConv_firstCall_seedsConv", () => {
    useChatStore.getState().hydrateConv(CV, [
      { id: "msg_1", role: "user", status: "completed", createdAt: "2024-01-01T00:00:00Z", blocks: [] },
    ]);
    const c = useChatStore.getState().convs[CV];
    expect(c.messages.size).toBe(1);
    expect(c.topMsgIds).toEqual(["msg_1"]);
  });

  it("hydrateConv_secondCallSameConv_isNoop", () => {
    const seed1 = [{ id: "msg_1", role: "user" as const, createdAt: "2024-01-01T00:00:00Z", blocks: [] as any[] }];
    const seed2 = [{ id: "msg_2", role: "user" as const, createdAt: "2024-01-01T00:00:00Z", blocks: [] as any[] }];
    useChatStore.getState().hydrateConv(CV, seed1);
    useChatStore.getState().hydrateConv(CV, seed2);
    const c = useChatStore.getState().convs[CV];
    expect([...c.messages.keys()]).toEqual(["msg_1"]);
  });

  it("hydrateConv_afterResetConv_canHydrateAgain", () => {
    useChatStore.getState().hydrateConv(CV, [{ id: "msg_1", role: "user", createdAt: "2024-01-01T00:00:00Z", blocks: [] }]);
    useChatStore.getState().resetConv(CV);
    useChatStore.getState().hydrateConv(CV, [{ id: "msg_2", role: "user", createdAt: "2024-01-01T00:00:00Z", blocks: [] }]);
    const c = useChatStore.getState().convs[CV];
    expect([...c.messages.keys()]).toEqual(["msg_2"]);
  });

  it("hydrateConv_doesNotWipeSSEAccumulatedState", async () => {
    useChatStore.getState().hydrateConv(CV, []);
    useChatStore.getState().onMessageStart(CV, { id: "msg_sse", role: "assistant" });
    useChatStore.getState().onBlockStart(CV, {
      id: "blk_1", messageId: "msg_sse", blockType: "text",
    });
    useChatStore.getState().onBlockDelta(CV, { id: "blk_1", delta: "hello" });
    await nextFrame();
    useChatStore.getState().hydrateConv(CV, []);
    const c = useChatStore.getState().convs[CV];
    expect(c.messages.has("msg_sse")).toBe(true);
    expect(c.blocks.get("blk_1")!.content).toBe("hello");
  });
});

describe("hydrateConv — tree rebuild", () => {
  it("hydrateConv_flatBlocks_buildsParentChildLinks", () => {
    useChatStore.getState().hydrateConv(CV, [{
      id: "msg_1",
      role: "assistant",
      createdAt: "2024-01-01T00:00:00Z",
      blocks: [
        { id: "blk_tool", type: "tool_call", parentBlockId: null },
        { id: "blk_progress", type: "progress", parentBlockId: "blk_tool" },
        { id: "blk_result", type: "tool_result", parentBlockId: "blk_tool" },
      ],
    }]);
    const c = useChatStore.getState().convs[CV];
    const tool = c.blocks.get("blk_tool")!;
    expect(tool.children).toEqual(["blk_progress", "blk_result"]);
  });

  it("hydrateConv_nestedSubagentMessage_recurses", () => {
    useChatStore.getState().hydrateConv(CV, [{
      id: "msg_outer",
      role: "assistant",
      createdAt: "2024-01-01T00:00:00Z",
      blocks: [{
        id: "blk_sub", type: "message", parentBlockId: null,
        innerMessage: { id: "msg_inner", role: "assistant", createdAt: "2024-01-01T00:00:00Z", blocks: [] },
      }],
    }]);
    const c = useChatStore.getState().convs[CV];
    expect(c.messages.has("msg_inner")).toBe(true);
    expect(c.messages.get("msg_inner")!.parentBlockId).toBe("blk_sub");
  });
});

describe("SSE event sequence — full lifecycle", () => {
  it("messageStart_blockStart_blockDelta_blockStop_messageStop_endToEnd", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });
    s.onBlockDelta(CV, { id: "blk_1", delta: "hello " });
    s.onBlockDelta(CV, { id: "blk_1", delta: "world" });
    await nextFrame();
    s.onBlockStop(CV, { id: "blk_1", status: "completed", durationMs: 123 });
    s.onMessageStop(CV, { id: "msg_1", status: "completed", inputTokens: 10, outputTokens: 20 });

    const conv = useChatStore.getState().convs[CV];
    const msg = conv.messages.get("msg_1")!;
    const blk = conv.blocks.get("blk_1")!;
    expect(msg.status).toBe("completed");
    expect(msg.inputTokens).toBe(10);
    expect(msg.outputTokens).toBe(20);
    expect(blk.content).toBe("hello world");
    expect(blk.status).toBe("completed");
    expect(blk.durationMs).toBe(123);
  });

  it("messageStart_dedupes_sameIdTwice", () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    expect(useChatStore.getState().convs[CV].topMsgIds).toEqual(["msg_1"]);
  });

  it("blockStart_dedupes_sameIdTwice", () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });
    expect(useChatStore.getState().convs[CV].messages.get("msg_1")!.blocks).toEqual(["blk_1"]);
  });

  it("blockDelta_unknownBlockId_silentlyDropped", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockDelta(CV, { id: "blk_ghost", delta: "x" });
    await nextFrame();
    expect(useChatStore.getState().convs[CV].blocks.size).toBe(0);
  });

  it("blockStop_unknownBlockId_silentlyDropped", () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    expect(() => s.onBlockStop(CV, { id: "blk_ghost" })).not.toThrow();
  });
});

describe("subagent nesting via message-type block", () => {
  it("messageStart_withParentBlockId_nestsUnderPlaceholderBlock", () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_outer", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_msg", messageId: "msg_outer", blockType: "message" });
    s.onMessageStart(CV, { id: "msg_inner", role: "assistant", parentBlockId: "blk_msg" });

    const conv = useChatStore.getState().convs[CV];
    expect(conv.messages.has("msg_inner")).toBe(true);
    expect(conv.topMsgIds).toEqual(["msg_outer"]);
    expect(conv.blocks.get("blk_msg")!.attrs!["messageId"]).toBe("msg_inner");
  });
});

describe("onBlockDelta — rAF coalescing", () => {
  it("multipleDeltasInSameFrame_collapseToOneSetState", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });

    const versionBefore = useChatStore.getState().convs[CV].blocks.get("blk_1")!.version;

    for (let i = 0; i < 20; i++) {
      s.onBlockDelta(CV, { id: "blk_1", delta: "x" });
    }

    expect(useChatStore.getState().convs[CV].blocks.get("blk_1")!.content).toBe("");

    await nextFrame();

    const after = useChatStore.getState().convs[CV].blocks.get("blk_1")!;
    expect(after.content).toBe("xxxxxxxxxxxxxxxxxxxx");
    expect(after.version).toBe(versionBefore + 1);
  });

  it("onBlockStop_flushesPendingDeltasFirst", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });

    s.onBlockDelta(CV, { id: "blk_1", delta: "partial" });
    s.onBlockStop(CV, { id: "blk_1", status: "completed" });

    const blk = useChatStore.getState().convs[CV].blocks.get("blk_1")!;
    expect(blk.content).toBe("partial");
    expect(blk.status).toBe("completed");
  });

  it("onMessageStop_flushesPendingDeltasFirst", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });
    s.onBlockDelta(CV, { id: "blk_1", delta: "last bit" });
    s.onMessageStop(CV, { id: "msg_1", status: "completed" });

    const c = useChatStore.getState().convs[CV];
    expect(c.blocks.get("blk_1")!.content).toBe("last bit");
    expect(c.messages.get("msg_1")!.status).toBe("completed");
  });
});

describe("selector identity — fan-out regression", () => {
  it("selectBlock_unchangedBlock_returnsSameRefAcrossDeltas", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_target", messageId: "msg_1", blockType: "text" });
    s.onBlockStart(CV, { id: "blk_other", messageId: "msg_1", blockType: "text" });

    const beforeRef = selectBlock(CV, "blk_other", useChatStore.getState());
    s.onBlockDelta(CV, { id: "blk_target", delta: "x" });
    await nextFrame();
    const afterRef = selectBlock(CV, "blk_other", useChatStore.getState());

    expect(afterRef).toBe(beforeRef);
  });

  it("selectChildIds_unchangedParent_returnsSameRefAcrossDeltas", async () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    s.onBlockStart(CV, { id: "blk_parent", messageId: "msg_1", blockType: "tool_call" });
    s.onBlockStart(CV, { id: "blk_child", messageId: "msg_1", parentId: "blk_parent", blockType: "progress" });

    const before = selectChildIds(CV, "blk_parent", useChatStore.getState());
    s.onBlockDelta(CV, { id: "blk_child", delta: "x" });
    await nextFrame();
    const after = selectChildIds(CV, "blk_parent", useChatStore.getState());
    expect(after).toBe(before);
  });

  it("selectTopMessageIds_noNewMessage_stableRef", () => {
    const s = useChatStore.getState();
    s.onMessageStart(CV, { id: "msg_1", role: "assistant" });
    const before = selectTopMessageIds(CV, useChatStore.getState());
    s.onBlockStart(CV, { id: "blk_1", messageId: "msg_1", blockType: "text" });
    const after = selectTopMessageIds(CV, useChatStore.getState());
    expect(after).toBe(before);
  });

  it("selectBlock_missingConv_returnsNull", () => {
    expect(selectBlock("cv_nope", "blk", useChatStore.getState())).toBeNull();
  });

  it("selectChildIds_missingParent_returnsFrozenEmpty", () => {
    const a = selectChildIds("cv_x", "blk_x", useChatStore.getState());
    const b = selectChildIds("cv_y", "blk_y", useChatStore.getState());
    expect(a).toBe(b);
    expect(Object.isFrozen(a)).toBe(true);
  });
});
