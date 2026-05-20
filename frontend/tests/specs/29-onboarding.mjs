// Onboarding 5-step wizard — opts:{skipOnboarding:false} so the wizard
// actually mounts on test start (default harness behaviour skips it).
import { runCase } from "../lib/harness.mjs";

const wiz = (name, fn) => () => runCase("29-onboarding · " + name, fn, { skipOnboarding: false });

export default [
  wiz("Onboarding stage renders on fresh install", async ({ page, expect, shot }) => {
    await page.waitForSelector(".onb-stage", { timeout: 5000 });
    await expect.visible(page.locator(".onb-stage"));
    await expect.visible(page.locator(".onb-rail"));
    await shot("intro");
  }),

  wiz("step 1 (intro) shows 3 bullets + accent button", async ({ page, expect }) => {
    await page.waitForSelector(".onb-stage");
    const bullets = await page.locator(".onb-bullet").count();
    expect.equals(bullets, 3, "intro should list 3 bullets");
  }),

  wiz("step 2 requires name input to enable Continue", async ({ page, expect }) => {
    await page.waitForSelector(".onb-stage");
    await page.locator(".onb-actions button:has-text('开始')").click();
    await page.waitForTimeout(300);
    // Continue (继续) button should be disabled without a name
    const btn = page.locator(".onb-actions button:has-text('继续')");
    const disabled = await btn.first().isDisabled();
    expect.truthy(disabled, "Continue should be disabled with empty name");
    await page.locator(".onb-input-lg").fill("sun");
    await page.waitForTimeout(200);
    const disabled2 = await btn.first().isDisabled();
    expect.truthy(!disabled2, "Continue should enable after typing name");
  }),

  wiz("step 3 accent swatches change preview", async ({ page, expect, shot }) => {
    await page.waitForSelector(".onb-stage");
    await page.locator(".onb-actions button:has-text('开始')").click();
    await page.waitForTimeout(200);
    await page.locator(".onb-input-lg").fill("sun");
    await page.locator(".onb-actions button:has-text('继续')").click();
    await page.waitForSelector(".onb-swatches");
    await page.locator(".onb-swatch[title='Notion 蓝']").click();
    await page.waitForTimeout(200);
    const accent = await page.evaluate(() => document.documentElement.dataset.accent);
    expect.equals(accent, "blue", "clicking blue swatch should update dataset.accent live");
    await shot("look-blue");
  }),

  wiz("step 4 provider grid + key input render", async ({ page, expect }) => {
    await page.waitForSelector(".onb-stage");
    await page.locator(".onb-actions button:has-text('开始')").click();
    await page.locator(".onb-input-lg").fill("sun");
    await page.locator(".onb-actions button:has-text('继续')").click();
    await page.waitForTimeout(200);
    await page.locator(".onb-actions button:has-text('继续')").click();
    await page.waitForSelector(".onb-provider-grid");
    const providers = await page.locator(".onb-provider").count();
    expect.gte(providers, 4, "provider grid should have multiple options");
    const keyInput = await page.locator(".onb-input[type='password']").count();
    expect.equals(keyInput, 1, "API key input should exist");
  }),

  wiz("complete flow with no key finishes + AppShell renders", async ({ page, expect, shot }) => {
    const username = "onbuser_" + Date.now();
    await page.waitForSelector(".onb-stage");
    await page.locator(".onb-actions button:has-text('开始')").click();
    await page.locator(".onb-input-lg").fill(username);
    await page.locator(".onb-actions button:has-text('继续')").click();  // → look
    await page.locator(".onb-actions button:has-text('继续')").click();  // → provider
    await page.locator(".onb-actions button:has-text('继续')").click();  // → done
    await page.waitForSelector(".onb-done-mark");
    await page.locator(".onb-actions button:has-text('进入应用')").click();
    await page.waitForSelector(".sidebar", { timeout: 8000 });
    await expect.visible(page.locator(".sidebar"));
    await shot("post-onboarding-sidebar");
  }),
];
