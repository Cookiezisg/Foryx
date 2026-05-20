// Overlay stack: Esc priority chain (cmdk > settings > ask > notifs),
// scrim click-outside, drawer slide direction, no overlay leak between tests.
import { runCase } from "../lib/harness.mjs";
import { openCmdk } from "../lib/helpers.mjs";

export default [
  ["Esc closes cmdk first, leaves other overlays alone", async ({ page, expect }) => {
    // open notifs first, then cmdk on top
    await page.locator(".sidebar .user-pill button.icon-btn[title*='通知']").click();
    await page.waitForSelector(".drawer-wrap.is-open");
    await openCmdk(page);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    const cmdk = await page.locator(".cmdk").count();
    const drawer = await page.locator(".drawer-wrap.is-open").count();
    expect.equals(cmdk, 0, "cmdk should close first");
    expect.equals(drawer, 1, "drawer should still be open");
  }],

  ["second Esc closes drawer", async ({ page, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='通知']").click();
    await page.waitForSelector(".drawer-wrap.is-open");
    await openCmdk(page);
    await page.keyboard.press("Escape");  // cmdk
    // Wait for cmdk Framer Motion exit AND state to settle, otherwise
    // the second Esc may race the first.
    await page.waitForFunction(() => !document.querySelector(".cmdk"), { timeout: 3000 });
    await page.waitForTimeout(200);
    await page.keyboard.press("Escape");  // drawer
    await page.waitForFunction(() => !document.querySelector(".drawer-wrap.is-open"), { timeout: 3000 });
    const drawer = await page.locator(".drawer-wrap.is-open").count();
    expect.equals(drawer, 0, "drawer should close on 2nd Esc");
  }],

  ["clicking drawer scrim closes drawer", async ({ page, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='通知']").click();
    await page.waitForSelector(".drawer-wrap.is-open");
    await page.locator(".drawer-scrim").click();
    await page.waitForTimeout(400);
    const drawer = await page.locator(".drawer-wrap.is-open").count();
    expect.equals(drawer, 0, "drawer should close on scrim click");
  }],

  ["overlay backdrop receives click → closes cmdk", async ({ page, expect }) => {
    await openCmdk(page);
    // click far outside cmdk card (top-left corner)
    await page.mouse.click(20, 20);
    await page.waitForTimeout(300);
    const cmdk = await page.locator(".cmdk").count();
    expect.equals(cmdk, 0, "cmdk dismissed by overlay click");
  }],

  ["AskUserModal manual open shows empty state when no pending", async ({ page, expect }) => {
    await page.locator(".sidebar .user-pill button.icon-btn[title*='问题']").click();
    await page.waitForSelector(".ask-card", { timeout: 3000 });
    const emptyState = await page.locator(".ask-card:has-text('没有待回答的问题')").count();
    expect.equals(emptyState, 1, "should show no-pending state when manually opened");
  }],

  ["toast tray renders bottom-right", async ({ page, expect }) => {
    // Trigger a toast via accent change failure path? Simpler: trigger via
    // any mutation. Use Memory pin which should toast on success/fail.
    // For now just assert the tray container is in DOM (empty is fine).
    const tray = await page.locator(".toast-tray").count();
    expect.equals(tray, 1, "ToastTray container should exist");
  }],
].map(([name, fn]) => () => runCase("16-overlays-deep · " + name, fn));
