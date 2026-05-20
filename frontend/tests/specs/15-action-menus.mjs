// ActionMenu: open/close, click-outside, danger styling, item separation.
import { runCase } from "../lib/harness.mjs";
import { seed, clickConv } from "../lib/helpers.mjs";

export default [
  ["conv list item shows ActionMenu trigger on hover", async ({ page, expect }) => {
    await seed.conv("am-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(500);
    const item = page.locator(".nav-conv-section .nav-item-wrap").first();
    await item.hover();
    await page.waitForTimeout(150);
    const trigger = await item.locator("button.rel-more-btn").count();
    expect.gte(trigger, 1, "more-btn should be present on conv row");
  }],

  ["clicking trigger opens floating menu", async ({ page, expect }) => {
    await seed.conv("am-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(400);
    const item = page.locator(".nav-conv-section .nav-item-wrap").first();
    await item.hover();
    await page.waitForTimeout(100);
    await item.locator("button.rel-more-btn").click({ force: true });
    await page.waitForTimeout(300);
    const menu = await page.locator(".action-menu").count();
    expect.equals(menu, 1, "ActionMenu should render in portal");
  }],

  ["menu shows divider + danger item (delete)", async ({ page, expect }) => {
    await seed.conv("am-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(400);
    const item = page.locator(".nav-conv-section .nav-item-wrap").first();
    await item.hover();
    await item.locator("button.rel-more-btn").click({ force: true });
    await page.waitForSelector(".action-menu");
    const divider = await page.locator(".action-menu-divider").count();
    expect.gte(divider, 1, "menu should have a divider");
    const danger = await page.locator(".action-menu-item.is-danger").count();
    expect.gte(danger, 1, "menu should have a danger-styled item (删除)");
  }],

  ["clicking outside closes the menu", async ({ page, expect }) => {
    await seed.conv("am-test " + Date.now());
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(400);
    const item = page.locator(".nav-conv-section .nav-item-wrap").first();
    await item.hover();
    await item.locator("button.rel-more-btn").click({ force: true });
    await page.waitForSelector(".action-menu");
    await page.mouse.click(800, 400);
    await page.waitForTimeout(400);
    const menu = await page.locator(".action-menu").count();
    expect.equals(menu, 0, "menu should dismiss on outside click");
  }],
].map(([name, fn]) => () => runCase("15-action-menus · " + name, fn));
