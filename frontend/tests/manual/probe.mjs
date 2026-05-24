import { chromium } from 'playwright';

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const page = await ctx.newPage();
page.on('pageerror', (err) => console.log('PAGEERR:', err.message));

const url = process.argv[2] || 'http://localhost:5173/';
const shot = process.argv[3] || '/tmp/forgify-shot.png';

await page.goto(url, { waitUntil: 'domcontentloaded' }).catch(() => {});
await page.waitForTimeout(800);

const mock = process.argv[4];
if (mock === 'main') {
  await page.evaluate(() => {
    localStorage.setItem('forgify-settings', JSON.stringify({
      state: { activeUserId: 'u_mockuser1234567', onboarded: true, theme: 'light', accent: 'claude', density: 'cozy', lang: 'zh' },
      version: 1,
    }));
  });
  await page.reload({ waitUntil: 'networkidle', timeout: 10000 }).catch(() => {});
  await page.waitForTimeout(1500);
}

await page.screenshot({ path: shot, fullPage: false });
console.log('shot saved:', shot);
await browser.close();
