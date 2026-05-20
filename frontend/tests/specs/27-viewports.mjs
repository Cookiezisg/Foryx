// Viewport sizes: shell behaves across screen widths.
import { runCase } from "../lib/harness.mjs";
import { openPaneByLabel } from "../lib/helpers.mjs";

export default [
  ["1920×1080: 2 panes side-by-side, no narrow mode", async ({ page, expect }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await openPaneByLabel(page, "锻造");
    await page.waitForTimeout(500);
    const narrowSwitch = await page.locator(".narrow-switch").count();
    expect.equals(narrowSwitch, 0, "wide viewport should not trigger narrow mode");
  }],

  ["1280×800: 2 panes still side-by-side", async ({ page, expect }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openPaneByLabel(page, "锻造");
    await page.waitForTimeout(500);
    const narrowSwitch = await page.locator(".narrow-switch").count();
    expect.equals(narrowSwitch, 0, "1280 main >1000 → no narrow");
  }],

  ["1024×768: NarrowSwitch tab bar appears", async ({ page, expect }) => {
    await page.setViewportSize({ width: 1024, height: 768 });
    await openPaneByLabel(page, "锻造");
    await page.waitForTimeout(700);
    const narrowSwitch = await page.locator(".narrow-switch").count();
    expect.equals(narrowSwitch, 1, "1024 viewport - sidebar (~248) = ~776 < 1000 → narrow");
  }],

  ["very tall narrow viewport (800×1200): app still renders", async ({ page, expect }) => {
    await page.setViewportSize({ width: 800, height: 1200 });
    await page.waitForTimeout(500);
    await expect.visible(page.locator(".sidebar"));
  }],

  ["sidebar collapse exposes more main width at narrow viewport", async ({ page, expect }) => {
    await page.setViewportSize({ width: 1100, height: 800 });
    const mainBefore = await page.locator(".main").evaluate((el) => el.clientWidth);
    await page.keyboard.press("Meta+b");
    await page.waitForTimeout(600);
    const mainAfter = await page.locator(".main").evaluate((el) => el.clientWidth);
    expect.truthy(mainAfter > mainBefore, `main should widen when sidebar collapses (${mainBefore} → ${mainAfter})`);
  }],
].map(([name, fn]) => () => runCase("27-viewports · " + name, fn));
