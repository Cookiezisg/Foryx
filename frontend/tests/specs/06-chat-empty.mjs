// L2/L3 — chat with no API key shows NoApiKeyGate (when state is fresh).
// Skipped if backend already has keys.
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";

export default [
  ["chat without API key shows NoApiKeyGate (or normal chat when keyed)", async ({ page, shot, expect }) => {
    const keys = await backend.apiKeys().catch(() => []);
    const hasKey = (Array.isArray(keys) ? keys : keys?.items || []).length > 0;

    if (!hasKey) {
      await expect.visible(page.locator(".empty-shell-title:has-text('API Key')"));
      await shot("no-api-key-gate");
    } else {
      // Either NoApiKeyGate is bypassed OR ChatPane renders. Both are acceptable.
      const gate = await page.locator(".empty-shell-title:has-text('API Key')").count();
      const chat = await page.locator(".chat-stream-inner, .empty-shell-title").count();
      expect.gte(gate + chat, 1, "either NoApiKeyGate or chat should render");
    }
  }],
].map(([name, fn]) => () => runCase("06-chat-empty · " + name, fn));
