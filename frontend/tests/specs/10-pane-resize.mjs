// PaneResize: drag clamping at 20-80%, sidebar collapse, narrow mode trigger.
import { runCase } from "../lib/harness.mjs";
import { openPaneByLabel, dragBy } from "../lib/helpers.mjs";

export default [
  ["opening 2nd pane shows .pane-resize divider", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    await page.waitForTimeout(400);
    const dividerCount = await page.locator(".pane-resize").count();
    expect.gte(dividerCount, 1, "two-pane mode should render a resize handle");
  }],

  ["resize drag changes pane width", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    await page.waitForTimeout(500);
    const leftBefore = await page.locator(".pane[data-kind='chat']").evaluate((el) => el.parentElement.clientWidth);
    await dragBy(page, page.locator(".pane-resize").first(), 200, 0, 12);
    await page.waitForTimeout(300);
    const leftAfter = await page.locator(".pane[data-kind='chat']").evaluate((el) => el.parentElement.clientWidth);
    expect.truthy(Math.abs(leftAfter - leftBefore) > 80, `expected meaningful width shift (was ${leftBefore}, now ${leftAfter})`);
  }],

  ["resize clamps to 20% minimum", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    await page.waitForTimeout(500);
    // Try to drag way too far left
    await dragBy(page, page.locator(".pane-resize").first(), -2000, 0, 20);
    await page.waitForTimeout(300);
    const leftPct = await page.evaluate(() => {
      const main = document.querySelector(".main");
      const left = document.querySelector(".pane-wrap");
      if (!main || !left) return null;
      return Math.round((left.clientWidth / main.clientWidth) * 100);
    });
    expect.truthy(leftPct >= 19, `leftPct should clamp >=20 (got ${leftPct})`);
  }],

  ["resize clamps to 80% maximum", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    await page.waitForTimeout(500);
    await dragBy(page, page.locator(".pane-resize").first(), 4000, 0, 20);
    await page.waitForTimeout(300);
    const leftPct = await page.evaluate(() => {
      const main = document.querySelector(".main");
      const left = document.querySelector(".pane-wrap");
      if (!main || !left) return null;
      return Math.round((left.clientWidth / main.clientWidth) * 100);
    });
    expect.truthy(leftPct <= 81, `leftPct should clamp <=80 (got ${leftPct})`);
  }],

  ["sidebar collapse toggle reduces width", async ({ page, expect }) => {
    const before = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    await page.keyboard.press("Meta+b");
    await page.waitForTimeout(600);
    const after = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    expect.truthy(after < before * 0.5, `sidebar should shrink dramatically (${before} → ${after})`);
  }],

  ["narrow viewport triggers narrow mode (NarrowSwitch visible)", async ({ page, expect, ctx }) => {
    // Resize browser to trigger narrow mode (main < 1000px → +sidebar width)
    await page.setViewportSize({ width: 1100, height: 800 });
    await openPaneByLabel(page, "锻造");
    await page.waitForTimeout(600);
    const narrowSwitch = await page.locator(".narrow-switch").count();
    expect.equals(narrowSwitch, 1, "NarrowSwitch should appear when 2 panes open + main < 1000px");
  }],
].map(([name, fn]) => () => runCase("10-pane-resize · " + name, fn));
