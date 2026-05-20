// Keyboard accessibility: Tab order, focus indicators, Esc priority,
// can-everything-be-driven-without-mouse.
import { runCase } from "../lib/harness.mjs";

export default [
  ["sidebar nav items are keyboard-focusable", async ({ page, expect }) => {
    // Focus first nav item
    await page.locator("button.nav-item").first().focus();
    const focused = await page.evaluate(() => document.activeElement?.tagName);
    expect.equals(focused, "BUTTON", "nav-item button should be focusable");
  }],

  ["Tab moves between focusable elements", async ({ page, expect }) => {
    await page.locator(".cmdk-trigger").focus();
    const beforeRole = await page.evaluate(() => document.activeElement?.className);
    await page.keyboard.press("Tab");
    const afterRole = await page.evaluate(() => document.activeElement?.className);
    expect.truthy(afterRole !== beforeRole, "Tab should change focus");
  }],

  ["Escape priority: cmdk > drawer", async ({ page, expect }) => {
    // open drawer
    await page.locator(".sidebar .user-pill button.icon-btn[title*='通知']").click();
    await page.waitForSelector(".drawer-wrap.is-open");
    // open cmdk on top
    await page.keyboard.press("Meta+k");
    await page.waitForSelector(".cmdk");
    // First Esc closes cmdk
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    const cmdkGone = (await page.locator(".cmdk").count()) === 0;
    const drawerStill = (await page.locator(".drawer-wrap.is-open").count()) === 1;
    expect.truthy(cmdkGone, "cmdk should close first");
    expect.truthy(drawerStill, "drawer should remain after first Esc");
  }],

  ["typing in textarea does NOT trigger global ⌘B", async ({ page, expect }) => {
    // Need an active conv to get composer. Click first conv in sidebar.
    const firstConv = page.locator(".nav-conv-section .nav-item-wrap").first();
    if (await firstConv.count() > 0) {
      await firstConv.locator(".nav-item").first().click();
    }
    await page.waitForSelector(".composer-textarea", { timeout: 4000 });
    const w0 = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    await page.locator(".composer-textarea").focus();
    await page.keyboard.press("b");
    await page.waitForTimeout(300);
    const w1 = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    expect.equals(w0, w1, "typing 'b' in textarea must not collapse sidebar");
  }],

  ["typing in textarea does NOT trigger ⌘K either (without modifier)", async ({ page, expect }) => {
    const firstConv = page.locator(".nav-conv-section .nav-item-wrap").first();
    if (await firstConv.count() > 0) {
      await firstConv.locator(".nav-item").first().click();
    }
    await page.waitForSelector(".composer-textarea", { timeout: 4000 });
    await page.locator(".composer-textarea").focus();
    await page.keyboard.press("k");
    await page.waitForTimeout(200);
    const cmdk = await page.locator(".cmdk").count();
    expect.equals(cmdk, 0, "typing 'k' must not open cmdk");
  }],
].map(([name, fn]) => () => runCase("26-a11y · " + name, fn));
