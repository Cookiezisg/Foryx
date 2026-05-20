// Live block-tree verification: send a message that triggers multiple
// tool calls + reasoning, then walk the rendered DOM and assert every
// invariant. Closes the loop on Phase 11's stability fixes.
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";
import { seed, clickConv } from "../lib/helpers.mjs";

export default [
  ["live message triggers reasoning + tool_call + result + final text",
   async ({ page, expect, shot }) => {
    if (!await seed.ensureDeepSeek()) { console.log("    (skipped: no DEEPSEEK_KEY)"); return; }
    const c = await backend.createConv("blocks-live " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await clickConv(page, c.title);
    await page.waitForSelector(".composer-textarea");

    await page.locator(".composer-textarea").fill("用 search_documents 查 Forgify 这个词");
    await page.locator(".send-btn:not(.is-stop):not(.is-disabled)").click();
    await page.waitForSelector(".badge.streaming", { timeout: 15000 });
    await page.waitForFunction(
      () => !document.querySelector(".badge.streaming"),
      { timeout: 90_000 }
    );
    await page.waitForTimeout(800);

    // Reasoning blocks rendered
    const reasoning = await page.locator(".blk-reasoning").count();
    expect.gte(reasoning, 1, "expected ≥1 reasoning block");
    // Tool calls rendered
    const tools = await page.locator(".blk-tool").count();
    expect.gte(tools, 1, "expected ≥1 tool_call block");
    // Each tool_call → nested tool_result on expand
    for (let i = 0; i < tools; i++) {
      await page.locator(".blk-tool .blk-tool-head").nth(i).click();
      await page.waitForTimeout(200);
      const nested = await page.locator(".blk-tool").nth(i).locator(".tool-result").count();
      expect.equals(nested, 1, `tool[${i}] must have exactly 1 nested tool-result`);
    }
    // Final text block rendered
    const text = await page.locator(".blk-text").count();
    expect.gte(text, 1, "expected final text block");

    await shot("live-tree");
  }],

  ["assistant message shows token counts in meta",
   async ({ page, expect }) => {
    if (!await seed.ensureDeepSeek()) { console.log("    (skipped)"); return; }
    const list = await backend.conversations();
    const arr = Array.isArray(list) ? list : list?.items || [];
    const conv = arr.find((c) => /blocks-live/.test(c.title));
    if (!conv) { console.log("    (skipped: no blocks-live conv)"); return; }
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await clickConv(page, conv.title);
    await page.waitForSelector(".msg-tokens", { timeout: 6000 });
    const tokensVisible = await page.locator(".msg.role-assistant .msg-tokens").count();
    expect.gte(tokensVisible, 1, "assistant message should show token counts");
  }],
].map(([name, fn]) => () => runCase("25-blocks-live · " + name, fn));
