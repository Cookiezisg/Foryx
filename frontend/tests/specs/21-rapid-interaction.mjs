// Rapid / repeated interactions — toggle floods, double clicks, etc.
import { runCase } from "../lib/harness.mjs";

export default [
  ["rapid ⌘K toggles 5x leaves cmdk in stable state", async ({ page, expect }) => {
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press("Meta+k");
      await page.waitForTimeout(50);
    }
    await page.waitForTimeout(300);
    // After 5 toggles (odd → open). Take final state, force-close, assert clean.
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    const cmdk = await page.locator(".cmdk").count();
    expect.equals(cmdk, 0, "Esc must close after rapid toggles");
  }],

  ["rapid ⌘B toggle 4x returns to original width", async ({ page, expect }) => {
    const w0 = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    for (let i = 0; i < 4; i++) {
      await page.keyboard.press("Meta+b");
      await page.waitForTimeout(400);
    }
    // Spring animation may still be settling; wait extra for it to finish.
    await page.waitForTimeout(800);
    const wf = await page.locator(".sidebar").evaluate((el) => el.clientWidth);
    // Tolerate ±16px for spring damping (stiffness=280, damping=28 ≈ ~6% overshoot).
    expect.truthy(Math.abs(wf - w0) < 16, `4 toggles should return to ~original (${w0} → ${wf})`);
  }],

  ["spam-clicking nav item doesn't crash", async ({ page, expect }) => {
    const btn = page.locator("button.nav-item:has-text('锻造')").first();
    for (let i = 0; i < 10; i++) {
      await btn.click({ force: true });
      await page.waitForTimeout(50);
    }
    await page.waitForTimeout(500);
    // Final state: either open or closed. Page should still render sidebar.
    await expect.visible(page.locator(".sidebar"));
  }],

  ["rapid pane open/close doesn't leak DOM nodes", async ({ page, expect }) => {
    for (let i = 0; i < 5; i++) {
      await page.locator("button.nav-item:has-text('执行')").first().click();
      await page.waitForTimeout(250);
      await page.locator("button.nav-item:has-text('执行')").first().click();
      await page.waitForTimeout(250);
    }
    // AnimatePresence exit animation takes ~220-300ms; wait for it.
    await page.waitForFunction(
      () => !document.querySelector(".pane[data-kind='execute']"),
      { timeout: 3000 }
    );
    const executePane = await page.locator(".pane[data-kind='execute']").count();
    expect.equals(executePane, 0, "no leaked execute pane");
  }],
].map(([name, fn]) => () => runCase("21-rapid · " + name, fn));
