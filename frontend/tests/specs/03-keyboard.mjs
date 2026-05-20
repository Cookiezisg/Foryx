// L2 — keyboard shortcuts.
import { runCase } from "../lib/harness.mjs";

export default [
  ["⌘K opens command palette", async ({ page, shot, expect }) => {
    await page.keyboard.press("Meta+k");
    await page.waitForSelector(".cmdk", { timeout: 4000 });
    await expect.visible(page.locator(".cmdk"));
    await shot("cmdk-open");
  }],

  ["Esc closes command palette", async ({ page, expect }) => {
    await page.keyboard.press("Meta+k");
    await page.waitForSelector(".cmdk", { timeout: 4000 });
    await page.keyboard.press("Escape");
    await page.waitForTimeout(400);
    const count = await page.locator(".cmdk").count();
    expect.equals(count, 0, "cmdk should be gone after Esc");
  }],

  ["⌘B collapses sidebar", async ({ page, expect }) => {
    const initial = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    await page.keyboard.press("Meta+b");
    await page.waitForTimeout(600);
    const after = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    expect.truthy(after < initial, `sidebar should shrink (was ${initial}, now ${after})`);
  }],

  ["⌘B again restores sidebar", async ({ page, expect }) => {
    await page.keyboard.press("Meta+b");
    await page.waitForTimeout(600);
    const collapsedW = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    await page.keyboard.press("Meta+b");
    await page.waitForTimeout(600);
    const restoredW = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    expect.truthy(restoredW > collapsedW, `sidebar should grow back (collapsed ${collapsedW}, restored ${restoredW})`);
  }],
].map(([name, fn]) => () => runCase("03-keyboard · " + name, fn));
