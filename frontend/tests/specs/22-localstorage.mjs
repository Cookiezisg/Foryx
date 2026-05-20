// localStorage: settings shape, corruption tolerance, defaults restored.
import { runCase } from "../lib/harness.mjs";

const KEY = "forgify-settings";

export default [
  ["fresh localStorage → defaults apply (theme=system)", async ({ page, expect }) => {
    await page.evaluate((k) => localStorage.removeItem(k), KEY);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(400);
    const accent = await page.evaluate(() => document.documentElement.dataset.accent);
    const density = await page.evaluate(() => document.documentElement.dataset.density);
    expect.equals(accent, "claude", "default accent");
    expect.equals(density, "cozy", "default density");
  }],

  ["corrupted localStorage value falls back to defaults (no crash)", async ({ page, expect }) => {
    await page.evaluate((k) => localStorage.setItem(k, "{not valid json"), KEY);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar", { timeout: 5000 });
    await page.waitForTimeout(400);
    const accent = await page.evaluate(() => document.documentElement.dataset.accent);
    expect.equals(accent, "claude", "should fall back to default accent");
  }],

  ["setting accent persists across reload", async ({ page, expect }) => {
    await page.evaluate((k) => localStorage.removeItem(k), KEY);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
    await page.waitForSelector(".settings-pop");
    await page.locator(".settings-pop-swatch[title='blue']").click();
    await page.waitForTimeout(300);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(400);
    const accent = await page.evaluate(() => document.documentElement.dataset.accent);
    expect.equals(accent, "blue");
  }],
].map(([name, fn]) => () => runCase("22-localstorage · " + name, fn));
