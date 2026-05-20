// Test harness — fresh browser + page per case, captures console errors,
// auto-screenshots, returns a structured result.
//
// 每个 case 一个新 browser + page；监听 console + pageerror；
// 自动截图；结构化返回。

import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const FRONTEND_URL = process.env.FRONTEND_URL || "http://localhost:5173";
const SHOT_DIR = process.env.SHOT_DIR || "/tmp/forgify-tests";

mkdirSync(SHOT_DIR, { recursive: true });

export async function runCase(name, fn, opts = {}) {
  const { allowConsoleErrors = false, skipOnboarding = true } = opts;
  const start = Date.now();
  const browser = await chromium.launch();
  const ctx = await browser.newContext({
    viewport: { width: 1440, height: 900 },
  });
  const page = await ctx.newPage();

  // Pre-set localStorage BEFORE the page loads. The Onboarding wizard
  // is gated on settings.onboarded; default tests want to skip past it
  // and land in AppShell. A dedicated Onboarding spec passes
  // {skipOnboarding:false} to test the first-run flow itself.
  //
  // 测试默认设 onboarded=true 跳过首次启动向导；Onboarding 专门 spec
  // 显式传 skipOnboarding:false 来测向导本身。
  if (skipOnboarding) {
    await ctx.addInitScript(() => {
      try {
        const key = "forgify-settings";
        const raw = localStorage.getItem(key);
        const cur = raw ? JSON.parse(raw) : { state: {}, version: 1 };
        cur.state = { ...(cur.state || {}), onboarded: true };
        localStorage.setItem(key, JSON.stringify(cur));
      } catch { /* ignore */ }
    });
  }

  const consoleErrors = [];
  page.on("console", (m) => { if (m.type() === "error") consoleErrors.push(m.text()); });
  page.on("pageerror", (e) => consoleErrors.push("pageerror: " + e.message));

  const shot = async (tag) => {
    const safe = (name + "-" + tag).replace(/[^a-z0-9-]+/gi, "_");
    const path = `${SHOT_DIR}/${safe}.png`;
    await page.screenshot({ path });
    return path;
  };

  let result;
  try {
    // Onboarding tests append `?onboarding=1` so the wizard mounts
    // unconditionally regardless of backend user state (which other
    // tests mutate). Default tests use the bare URL.
    //
    // Onboarding 测试加 `?onboarding=1` 强制弹向导；其他测试用裸 URL。
    const url = skipOnboarding ? FRONTEND_URL : `${FRONTEND_URL}?onboarding=1`;
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: 15000 });
    if (skipOnboarding) {
      await page.waitForSelector(".sidebar", { timeout: 8000 });
    } else {
      await page.waitForSelector(".onb-stage", { timeout: 8000 });
    }
    await page.waitForTimeout(800);
    await fn({ page, ctx, shot, expect });
    result = { name, status: "pass", consoleErrors, durationMs: Date.now() - start };
  } catch (err) {
    const path = await shot("FAIL").catch(() => null);
    result = {
      name, status: "fail",
      error: err.message,
      stack: err.stack?.split("\n").slice(0, 5).join("\n"),
      screenshot: path,
      consoleErrors,
      durationMs: Date.now() - start,
    };
  } finally {
    await browser.close();
  }

  if (consoleErrors.length > 0 && result.status === "pass" && !allowConsoleErrors) {
    // Console errors → automatic fail. Tests intentionally triggering
    // errors (forced 500s) pass `{allowConsoleErrors:true}` as 3rd arg.
    result.status = "fail";
    result.error = "console errors leaked";
  }

  return result;
}

// Mini assertion lib — single dependency-free assert helper. Throws
// with descriptive messages so harness can surface them in results.
//
// 极简断言：失败抛 Error，runner 捕获写入结果。
export const expect = {
  truthy(actual, msg) {
    if (!actual) throw new Error(msg || `expected truthy, got ${actual}`);
  },
  equals(actual, expected, msg) {
    if (actual !== expected) {
      throw new Error(msg || `expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
    }
  },
  gte(actual, min, msg) {
    if (actual < min) throw new Error(msg || `expected >= ${min}, got ${actual}`);
  },
  visible: async (locator, msg) => {
    const v = await locator.isVisible().catch(() => false);
    if (!v) throw new Error(msg || `expected visible: ${await locator.toString?.() ?? "(locator)"}`);
  },
};

// Print a final tabular report and exit with the right code.
export function report(results) {
  console.log("\n" + "═".repeat(78));
  console.log("Forgify frontend test report");
  console.log("═".repeat(78));

  let pass = 0, fail = 0;
  for (const r of results) {
    const mark = r.status === "pass" ? "✓" : "✗";
    const time = `${r.durationMs}ms`.padStart(7);
    console.log(`  ${mark}  ${r.name.padEnd(50)} ${time}`);
    if (r.status === "fail") {
      fail++;
      console.log(`       └─ ${r.error}`);
      if (r.stack) for (const l of r.stack.split("\n")) console.log(`            ${l.trim()}`);
      if (r.screenshot) console.log(`       └─ screenshot: ${r.screenshot}`);
      if (r.consoleErrors.length > 0) {
        for (const e of r.consoleErrors.slice(0, 3)) console.log(`       └─ console: ${e.slice(0, 100)}`);
        if (r.consoleErrors.length > 3) console.log(`            ... and ${r.consoleErrors.length - 3} more`);
      }
    } else {
      pass++;
    }
  }
  console.log("─".repeat(78));
  console.log(`  ${pass} passed · ${fail} failed · screenshots in ${SHOT_DIR}/`);
  console.log("═".repeat(78));
  return fail === 0;
}
