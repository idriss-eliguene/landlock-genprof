const { chromium } = require("playwright");
const { bindNamespace, openContextControls } = require("./namespace-binding");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:18183/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const namespace = process.env.UI_MIGRATION_NAMESPACE || "payments";
const requiredDimensions = ["authority", "coverage", "evidence", "freshness", "drift", "governance", "pipeline", "enforcement"];
const errors = [];
const responses = [];
let browser;

async function bind(page) {
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible" });
  await openContextControls(page);
  await bindNamespace(page, namespace, identity);
  const healthNav = page.getByRole("navigation").getByRole("button", { name: /^Health/ });
  await healthNav.click();
  await page.getByTestId("health-view").waitFor({ state: "visible", timeout: 120000 });
  await page.getByTestId("health-dimensions").waitFor({ state: "visible", timeout: 120000 });
}

function attachDiagnostics(page, label) {
  page.on("console", message => { if (message.type() === "error") errors.push(`${label}: console: ${message.text()}`); });
  page.on("pageerror", error => errors.push(`${label}: pageerror: ${error.message}`));
  page.on("requestfailed", request => { if (request.url().includes("/api/")) errors.push(`${label}: requestfailed: ${request.url()}`); });
  page.on("response", response => {
    if (!response.url().includes("/api/")) return;
    responses.push({ path: new URL(response.url()).pathname, status: response.status(), at: Date.now() });
  });
}

(async () => {
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } });
  attachDiagnostics(page, "A");
  await bind(page);
  const healthNav = page.getByRole("navigation").getByRole("button", { name: /^Health/ });
  for (const id of requiredDimensions) await page.getByTestId(`health-dimension-${id}`).waitFor({ state: "visible" });
  const healthText = await page.getByTestId("health-view").textContent();
  if (healthText?.match(/Security (Health|Profile Health):\s*\d+%|\d+\/100/)) throw new Error("Health rendered a magic score");
  if (!healthText?.includes("NOT_ESTABLISHED")) throw new Error("Health did not visibly distinguish unavailable proof");
  if (!healthText?.includes("Projection time is not evidence freshness")) throw new Error("Health omitted the refresh/freshness boundary");
  if (await page.getByTestId("health-issue").count()) {
    const issue = page.getByTestId("health-issue").first();
    const inspect = issue.getByRole("button").first();
    if (await inspect.count()) { await inspect.click(); await page.getByTestId("observations-heading").waitFor({ state: "visible", timeout: 120000 }); await healthNav.click(); await page.getByTestId("health-view").waitFor({ state: "visible" }); }
  }
  await page.screenshot({ path: "/tmp/operations-center-migration-health-1440.png", fullPage: true });
  for (const width of [1280, 1024, 680]) {
    await page.setViewportSize({ width, height: 1100 });
    await page.getByTestId("health-view").waitFor({ state: "visible" });
    if (await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) throw new Error(`horizontal overflow at ${width}px`);
    await page.screenshot({ path: `/tmp/operations-center-migration-health-${width}.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 1440, height: 1100 });
  await healthNav.focus();
  if (!await page.evaluate(() => document.activeElement?.textContent?.includes("Health") === true)) throw new Error("Health navigation focus is not semantic");
  const second = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  attachDiagnostics(second, "B");
  await bind(second);
  if (await page.locator('[data-testid="context-namespace"]').inputValue() !== namespace || await second.locator('[data-testid="context-namespace"]').inputValue() !== namespace) throw new Error("Health tabs lost namespace identity");
  if (errors.length) throw new Error(errors.join("\n"));
  const healthResponses = responses.filter(item => item.path === "/api/health").length;
  const overviewResponses = responses.filter(item => item.path === "/api/v08/overview").length;
  console.log(JSON.stringify({ health: "ready", dimensions: requiredDimensions, responsive: [1440, 1280, 1024, 680], multitab: "isolated", healthResponses, overviewResponses, unexpectedBrowserErrors: 0 }));
  await second.close();
  await browser.close();
})().catch(async error => { console.error(error.stack || error); if (browser) await browser.close(); process.exitCode = 1; });
