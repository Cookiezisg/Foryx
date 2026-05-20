// L4 — end-to-end chat with real DeepSeek. Seeds key + model + conv
// via REST before the browser test runs.
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";

async function ensureSetup() {
  if (!backend.hasDeepseekKey()) return null;
  // Idempotent: add key + set model only if not already present.
  const keys = await backend.apiKeys();
  const list = Array.isArray(keys) ? keys : keys?.items || [];
  let key = list.find((k) => k.provider === "deepseek");
  if (!key) {
    key = await backend.addKey("deepseek", backend.deepseekKey(), "DeepSeek (test)");
    await backend.testKey(key.id);
    await backend.setModel("chat", "deepseek", "deepseek-v4-flash");
  }
  return key;
}

export default [
  ["seed conv + send message + stream + assert block tree", async ({ page, shot, expect }) => {
    const key = await ensureSetup();
    if (!key) {
      console.log("    (skipped: no DEEPSEEK_KEY env)");
      return;
    }

    const conv = await backend.createConv("live-chat-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(800);

    // Click the freshly created conv in sidebar
    await page.locator(`.nav-item .label:has-text('${conv.title}')`).click();
    await page.waitForSelector(".composer-textarea", { timeout: 5000 });

    // Type and send
    const ta = page.locator(".composer-textarea");
    await ta.fill("用 search_documents 查 forgify 这个词");
    await page.locator(".send-btn:not(.is-stop):not(.is-disabled)").click();

    // Wait for streaming badge to appear (proves SSE event arrived).
    await page.waitForSelector(".badge.streaming", { timeout: 15000 });
    await shot("streaming");

    // Wait for completion (streaming badge gone).
    await page.waitForFunction(
      () => !document.querySelector(".badge.streaming"),
      { timeout: 90_000 }
    );
    await page.waitForTimeout(500);

    // At least 2 messages (user + assistant)
    const msgCount = await page.locator(".msg").count();
    expect.gte(msgCount, 2, `expected >=2 messages, got ${msgCount}`);

    // Expect at least one tool_call rendered (because we asked for search_documents).
    const toolCount = await page.locator(".blk-tool").count();
    expect.gte(toolCount, 1, "expected at least one tool_call block");

    await shot("done");
  }],
].map(([name, fn]) => () => runCase("07-chat-live · " + name, fn));
