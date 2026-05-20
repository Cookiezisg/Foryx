// CommandPalette deep: search filter, group structure, keyboard nav, mouse vs keyboard.
import { runCase } from "../lib/harness.mjs";
import { openCmdk } from "../lib/helpers.mjs";

export default [
  ["cmdk lists 导航 group with all primary nav items", async ({ page, expect }) => {
    await openCmdk(page);
    const navRowCount = await page.locator(".cmdk-row:has-text('打开')").count();
    expect.gte(navRowCount, 4, "expect at least 4 'open' nav items");
  }],

  ["typing filters rows", async ({ page, expect }) => {
    await openCmdk(page);
    await page.locator(".cmdk-input").fill("锻造");
    await page.waitForTimeout(300);
    const rowsAfterFilter = await page.locator(".cmdk-row").count();
    const matching = await page.locator(".cmdk-row:has-text('锻造')").count();
    expect.gte(matching, 1, "expect at least one row matching 锻造");
    expect.truthy(rowsAfterFilter <= 5, `rows should narrow with filter, got ${rowsAfterFilter}`);
  }],

  ["empty filter shows full list back", async ({ page, expect }) => {
    await openCmdk(page);
    const initial = await page.locator(".cmdk-row").count();
    await page.locator(".cmdk-input").fill("zzznothingmatches");
    await page.waitForTimeout(200);
    const noMatches = await page.locator(".cmdk-row").count();
    expect.equals(noMatches, 0, "no rows when nothing matches");
    await page.locator(".cmdk-input").fill("");
    await page.waitForTimeout(200);
    const restored = await page.locator(".cmdk-row").count();
    expect.equals(restored, initial, "rows restored when filter cleared");
  }],

  ["ArrowDown then Enter selects 2nd row", async ({ page, expect }) => {
    await openCmdk(page);
    await page.waitForTimeout(200);
    // first row is 打开对话; second row is 打开锻造
    await page.keyboard.press("ArrowDown");
    await page.waitForTimeout(150);
    await page.keyboard.press("Enter");
    await page.waitForTimeout(500);
    const forge = await page.locator(".pane[data-kind='forge']").count();
    expect.equals(forge, 1, "Enter on 2nd row should open 锻造");
  }],

  ["mouse hover updates active row", async ({ page, expect }) => {
    await openCmdk(page);
    await page.waitForTimeout(200);
    const rows = page.locator(".cmdk-row");
    const target = rows.nth(2);
    await target.hover();
    await page.waitForTimeout(150);
    const isActive = await target.evaluate((el) => el.classList.contains("is-active"));
    expect.truthy(isActive, "hovered row should become active");
  }],

  ["Esc closes cmdk and does NOT open any pane", async ({ page, expect }) => {
    const panesBefore = await page.locator(".pane").count();
    await openCmdk(page);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    const cmdkVisible = await page.locator(".cmdk").count();
    expect.equals(cmdkVisible, 0, "cmdk closed");
    const panesAfter = await page.locator(".pane").count();
    expect.equals(panesAfter, panesBefore, "Esc should not select anything");
  }],
].map(([name, fn]) => () => runCase("12-cmdk-deep · " + name, fn));
