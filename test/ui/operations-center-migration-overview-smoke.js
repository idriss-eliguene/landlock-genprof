const { chromium } = require("playwright");
const { bindNamespace } = require("./namespace-binding");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:18093/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const namespace = process.env.UI_MIGRATION_NAMESPACE || "payments";
const errors = [];
const expectedNegativeRequests = [];
let browser;

async function bind(page) {
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible" });
  await page.getByTestId("context-identity").locator("option").nth(1).waitFor({ state: "attached" });
  await page.getByTestId("context-identity").selectOption(identity);
  await bindNamespace(page, namespace, identity);
  await page.getByTestId("overview-view").waitFor({ state: "visible", timeout: 120000 });
}

function attachDiagnostics(page, label) {
  page.on("console", message => { if (message.type() === "error") errors.push(`${label}: console: ${message.text()}`); });
  page.on("pageerror", error => errors.push(`${label}: pageerror: ${error.message}`));
  page.on("requestfailed", request => { if (request.url().includes("/api/")) errors.push(`${label}: requestfailed: ${request.url()}`); });
  page.on("response", response => { if (response.status() >= 400 && response.url().includes("/api/")) expectedNegativeRequests.push(`${response.status()} ${response.url()}`); });
}

(async () => {
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  attachDiagnostics(page, "A");
  await bind(page);
  for (const testid of ["overview-posture", "overview-attention", "overview-evidence", "overview-pipeline", "overview-governance", "overview-recent-activity", "overview-sphm"]) await page.getByTestId(testid).waitFor({ state: "visible" });
  const text = await page.getByTestId("overview-view").textContent();
  if (text?.match(/Security (Health|Profile Health):\s*\d+%|\d+\/100/)) throw new Error("Overview rendered a magic health score");
  if (text?.includes("Evidence\nHealthy") && text.includes("Loading")) throw new Error("loading state was presented as healthy");
  await page.screenshot({ path: "/tmp/operations-center-migration-overview-1440.png", fullPage: true });
  const refreshed = page.waitForResponse(response => response.url().includes("/api/v08/overview") && response.status() === 200);
  await page.getByRole("button", { name: "Refresh" }).click();
  await refreshed;
  await page.getByTestId("overview-posture").waitFor({ state: "visible" });
  const contextText = await page.locator(".context-meta").textContent();
  if (!contextText?.includes(namespace)) throw new Error("Overview context is not namespace-bound");
  for (const width of [1280, 1024, 680]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.getByTestId("overview-view").waitFor({ state: "visible" });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
    if (overflow) throw new Error(`horizontal overflow at ${width}px`);
    await page.screenshot({ path: `/tmp/operations-center-migration-overview-${width}.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole("button", { name: "Attention" }).focus();
  if (await page.evaluate(() => document.activeElement?.textContent?.includes("Attention") !== true)) throw new Error("primary navigation focus is not visible/semantic");
  const second = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  attachDiagnostics(second, "B");
  await bind(second);
  const aNamespace = await page.locator('[data-testid="context-namespace"]').inputValue();
  const bNamespace = await second.locator('[data-testid="context-namespace"]').inputValue();
  if (aNamespace !== namespace || bNamespace !== namespace) throw new Error("same-context tabs did not retain exact namespace identity");
  await second.screenshot({ path: "/tmp/operations-center-migration-overview-tab-b.png", fullPage: true });
  if (errors.length) throw new Error(errors.join("\n"));
  console.log(JSON.stringify({ overview: "ready", namespace, responsive: [1440, 1280, 1024, 680], multiTab: "isolated", browserErrors: 0, expectedNegativeRequests }));
  await second.close();
  await browser.close();
})().catch(async error => { console.error(error.stack || error); if (browser) await browser.close(); process.exitCode = 1; });
