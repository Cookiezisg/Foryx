// L3 — settings persistence + live theme apply.
import { runCase } from "../lib/harness.mjs";

export default [
  ["clicking accent swatch updates documentElement.dataset.accent", async ({ page, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
    await page.waitForSelector(".settings-pop-swatches");
    const before = await page.evaluate(() => document.documentElement.dataset.accent);
    // Pick a swatch different from current
    const swatches = page.locator(".settings-pop-swatch");
    const count = await swatches.count();
    let targetKey = null;
    for (let i = 0; i < count; i++) {
      const title = await swatches.nth(i).getAttribute("title");
      if (title && title !== before) { targetKey = title; await swatches.nth(i).click(); break; }
    }
    expect.truthy(targetKey, "found a non-current accent to click");
    await page.waitForTimeout(300);
    const after = await page.evaluate(() => document.documentElement.dataset.accent);
    expect.equals(after, targetKey, "accent dataset should reflect click");
  }],

  ["theme/accent persist across reload (localStorage)", async ({ page, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
    await page.waitForSelector(".settings-pop");
    await page.locator(".settings-pop-swatch[title='green']").click();
    await page.waitForTimeout(300);
    const accent = await page.evaluate(() => document.documentElement.dataset.accent);
    expect.equals(accent, "green");

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(500);
    const accentAfter = await page.evaluate(() => document.documentElement.dataset.accent);
    expect.equals(accentAfter, "green", "accent should persist across reload");
  }],
].map(([name, fn]) => () => runCase("05-settings · " + name, fn));
