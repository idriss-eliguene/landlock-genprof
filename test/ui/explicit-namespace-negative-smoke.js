const { chromium } = require('playwright');
const { bindNamespace } = require('./namespace-binding');

const url = process.env.UI_MIGRATION_URL || 'http://127.0.0.1:18483/next/';
const identity = process.env.UI_MIGRATION_IDENTITY || 'developer';

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    await page.getByTestId('migration-app').waitFor({ state: 'visible' });
    const bound = await bindNamespace(page, 'payments', identity);
    if (bound.mode !== 'EXPLICIT_ONLY') throw new Error(`expected EXPLICIT_ONLY, got ${bound.mode}`);
    const input = page.getByTestId('explicit-namespace');
    await input.fill('namespace-that-does-not-exist');
    const response = page.waitForResponse(item => item.url().includes('/capabilities?namespace=namespace-that-does-not-exist') && item.request().method() === 'GET');
    await page.getByTestId('open-explicit-namespace').click();
    const rejected = await response;
    if (rejected.status() !== 403 && rejected.status() !== 409) throw new Error(`unexpected unauthorized namespace status ${rejected.status()}`);
    const codes = await page.locator('.context-meta code').allTextContents();
    if (codes[1] !== 'payments') throw new Error(`unauthorized bind changed namespace to ${codes[1]}`);
    const alert = await page.getByRole('alert').innerText();
    if (/does-not-exist|not found|exists/i.test(alert)) throw new Error(`namespace existence leaked in UI: ${alert}`);
    console.log(JSON.stringify({ discoveryMode: bound.mode, unauthorizedStatus: rejected.status(), boundNamespace: codes[1], existenceLeakage: false }));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.stack || error); process.exitCode = 1; });
