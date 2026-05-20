// L4 — regression test for the hydrateConv parentBlockId fix.
// Every tool_call rendered in chat must contain exactly 1 tool_result
// inside its .blk-tool DOM subtree (not as a sibling).
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";

export default [
  ["every tool_call has its tool_result nested inside", async ({ page, expect }) => {
    if (!backend.hasDeepseekKey()) {
      console.log("    (skipped: no DEEPSEEK_KEY env)");
      return;
    }
    // Find any conv that contains tool_call blocks (created by 07-chat-live).
    const convs = await backend.conversations();
    const list = Array.isArray(convs) ? convs : convs?.items || [];
    const target = list.find((c) => /live-chat-test/.test(c.title));
    if (!target) {
      console.log("    (skipped: no seeded chat conv; run 07-chat-live first)");
      return;
    }

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.locator(`.nav-item .label:has-text('${target.title}')`).click();
    await page.waitForSelector(".blk-tool", { timeout: 8000 });
    await page.waitForTimeout(500);

    const toolHeads = page.locator(".blk-tool .blk-tool-head");
    const toolCount = await toolHeads.count();
    expect.gte(toolCount, 1, "expected at least one tool_call in DOM");

    for (let i = 0; i < toolCount; i++) {
      await toolHeads.nth(i).click();
      await page.waitForTimeout(200);
      const innerResult = await page.locator(".blk-tool").nth(i).locator(".tool-result").count();
      expect.equals(innerResult, 1, `tool_call[${i}] must contain exactly 1 nested tool_result (got ${innerResult})`);
    }
  }],
].map(([name, fn]) => () => runCase("08-nesting · " + name, fn));
