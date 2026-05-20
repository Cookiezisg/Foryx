// Multi-conversation parallel streaming — open 2 convs, send msg in
// each one, ensure events don't cross-contaminate.
//
// This is the hardest case for SSE chat store correctness — if event
// dispatch keys by anything but conversationId, streams will leak.
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";
import { seed, clickConv } from "../lib/helpers.mjs";

export default [
  ["2 convs streaming in parallel keep their messages separate", async ({ page, expect }) => {
    if (!await seed.ensureDeepSeek()) { console.log("    (skipped: no DEEPSEEK_KEY)"); return; }

    const a = await backend.createConv("multi-A " + Date.now());
    const b = await backend.createConv("multi-B " + Date.now());

    // Wait for backend to confirm both messages received + assistant
    // messages persisted. Polling REST keeps the test deterministic.
    await Promise.all([
      backend.sendMsg(a.id, "短答 hello A"),
      backend.sendMsg(b.id, "短答 hello B"),
    ]);

    async function waitForBoth() {
      for (let i = 0; i < 60; i++) {
        const am = await fetch(`http://localhost:8742/api/v1/conversations/${a.id}/messages`).then((r) => r.json());
        const bm = await fetch(`http://localhost:8742/api/v1/conversations/${b.id}/messages`).then((r) => r.json());
        const aList = am.data || [];
        const bList = bm.data || [];
        const aDone = aList.length >= 2 && aList[aList.length - 1].status === "completed";
        const bDone = bList.length >= 2 && bList[bList.length - 1].status === "completed";
        if (aDone && bDone) return;
        await new Promise((r) => setTimeout(r, 1000));
      }
      throw new Error("timeout waiting for both convs to finish");
    }
    await waitForBoth();

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await clickConv(page, a.title);
    await page.waitForTimeout(1000);

    const aMsgs = await page.locator(".msg").count();
    expect.gte(aMsgs, 2, `conv A should have ≥2 msgs, got ${aMsgs}`);
    const hasB = await page.locator(".msg-body:has-text('hello B')").count();
    expect.equals(hasB, 0, "conv A must NOT contain conv B content");

    await clickConv(page, b.title);
    await page.waitForTimeout(1000);
    const hasA = await page.locator(".msg-body:has-text('hello A')").count();
    expect.equals(hasA, 0, "conv B must NOT contain conv A content");
  }],
].map(([name, fn]) => () => runCase("23-multi-stream · " + name, fn));
