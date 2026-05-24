// Walk through the key surfaces on main after style refresh.
import { chromium } from 'playwright';

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const page = await ctx.newPage();
page.on('pageerror', (err) => console.log('PAGEERR:', err.message));

await page.goto('http://localhost:5173/', { waitUntil: 'domcontentloaded' });
await page.evaluate(() => {
  localStorage.setItem('forgify-settings', JSON.stringify({
    state: { activeUserId: 'u_mockuser1234567', onboarded: true, theme: 'light', accent: 'claude', density: 'cozy', lang: 'zh' },
    version: 1,
  }));
});
await page.reload({ waitUntil: 'networkidle', timeout: 10000 }).catch(() => {});
await page.waitForTimeout(1500);

async function snap(sel, name) {
  if (sel) {
    try { await page.locator(sel).first().click({ force: true, timeout: 3000 }); }
    catch (e) { console.log('click', sel, 'failed:', e.message.split('\n')[0]); }
    await page.waitForTimeout(800);
  }
  await page.screenshot({ path: `/tmp/flow-${name}.png` });
  console.log(name, 'saved');
}

await snap(null, 'home');
await snap('aside.sidebar button:has-text("锻造")', 'forge');
await snap('aside.sidebar button:has-text("执行")', 'execute');
await snap('aside.sidebar button:has-text("文档")', 'docs');
await snap('aside.sidebar button:has-text("洞察")', 'observe');
await snap('aside.sidebar button:has-text("Skills")', 'skills');
await snap('aside.sidebar button:has-text("MCP")', 'mcp');
await snap('aside.sidebar button:has-text("Memory")', 'memory');

// Open cmdk
await page.keyboard.down('Meta');
await page.keyboard.press('K');
await page.keyboard.up('Meta');
await page.waitForTimeout(400);
await snap(null, 'cmdk');
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// Click settings cog button
const settingsBtn = await page.$$('aside.sidebar .icon-btn');
if (settingsBtn.length >= 3) {
  await settingsBtn[settingsBtn.length - 1].click({ force: true });
  await page.waitForTimeout(400);
  await snap(null, 'settings-pop');
}

await browser.close();
console.log('done');
