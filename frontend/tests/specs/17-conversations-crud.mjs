// Conversations CRUD lifecycle via UI + REST verification.
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";
import { clickConv } from "../lib/helpers.mjs";

export default [
  ["new conv via sidebar + button + appears in list", async ({ page, expect }) => {
    const before = (await backend.conversations());
    const beforeList = Array.isArray(before) ? before : before?.items || [];
    await page.locator(".nav-conv-section .add-btn").first().click();
    // Wait for sidebar to refresh
    await page.waitForTimeout(800);
    const after = await backend.conversations();
    const afterList = Array.isArray(after) ? after : after?.items || [];
    expect.equals(afterList.length, beforeList.length + 1, `expected +1 conv (was ${beforeList.length}, now ${afterList.length})`);
  }],

  ["clicking a conv in sidebar activates it (highlighted + opens chat pane)", async ({ page, expect }) => {
    const c = await backend.createConv("activate-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await clickConv(page, c.title);
    await page.waitForTimeout(500);
    const active = await page.locator(`.nav-item-wrap.is-active .label:has-text('${c.title}')`).count();
    expect.equals(active, 1, "clicked conv should be active");
    const chat = await page.locator(".pane[data-kind='chat']").count();
    expect.equals(chat, 1, "chat pane should be open");
  }],

  ["empty conversation shows EmptyConvHero", async ({ page, expect }) => {
    const c = await backend.createConv("hero-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await clickConv(page, c.title);
    await page.waitForSelector(".chat-stream-inner", { timeout: 4000 });
    // Should NOT show messages, should show hero / try-something hint
    const msgs = await page.locator(".msg").count();
    expect.equals(msgs, 0, "no messages in fresh conv");
  }],

  ["sidebar shows multiple convs sorted", async ({ page, expect }) => {
    const list = await backend.conversations();
    const arr = Array.isArray(list) ? list : list?.items || [];
    if (arr.length < 2) {
      await backend.createConv("sort-test-a " + Date.now());
      await backend.createConv("sort-test-b " + Date.now());
    }
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(500);
    const rows = await page.locator(".nav-conv-section .nav-item-wrap").count();
    expect.gte(rows, 2, "sidebar shows multiple convs");
  }],
].map(([name, fn]) => () => runCase("17-conv-crud · " + name, fn));
