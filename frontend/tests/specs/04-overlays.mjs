// L2 — overlays: notifications drawer, settings popover.
import { runCase } from "../lib/harness.mjs";

export default [
  ["notifications drawer opens on bell click", async ({ page, shot, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='通知']").click();
    await page.waitForTimeout(500);
    await expect.visible(page.locator(".drawer-wrap.is-open"));
    await shot("notifs");
  }],

  ["settings popover opens + theme/accent/density swatches visible", async ({ page, shot, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
    await page.waitForTimeout(400);
    await expect.visible(page.locator(".settings-pop"));
    await expect.visible(page.locator(".settings-pop-swatches"));
    const swatchCount = await page.locator(".settings-pop-swatch").count();
    expect.gte(swatchCount, 5, "expected 5 accent swatches");
    await shot("settings");
  }],

  ["clicking outside closes settings popover", async ({ page, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
    await page.waitForSelector(".settings-pop");
    await page.mouse.click(800, 400);
    await page.waitForTimeout(400);
    const count = await page.locator(".settings-pop").count();
    expect.equals(count, 0, "popover should dismiss on outside click");
  }],
].map(([name, fn]) => () => runCase("04-overlays · " + name, fn));
