const { chromium } = require("playwright");
const { bindNamespace, openContextControls } = require("./namespace-binding");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:18093/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const namespace = process.env.UI_MIGRATION_NAMESPACE || "payments";

async function bind(page) {
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible" });
  await openContextControls(page);
  await bindNamespace(page, namespace, identity);
  await page.getByTestId("workload-row").first().waitFor({ state: "visible" });
}

function captureBrowserIntegrity(page) {
  const consoleErrors = [];
  const uncaught = [];
  const failedRequests = [];
  const unexpected = [];
  const expectedNegative = [];
  page.on("console", message => { if (message.type() === "error") consoleErrors.push(message.text()); });
  page.on("pageerror", error => uncaught.push(error.message));
  page.on("requestfailed", request => { if (request.url().includes("/api/")) failedRequests.push(`${request.method()} ${request.url()} ${request.failure()?.errorText || "failed"}`); });
  page.on("response", response => {
    if (response.status() >= 400 && response.url().includes("/api/")) {
      const line = `${response.status()} ${response.request().method()} ${response.url()}`;
      if (response.status() === 409 || response.status() === 403) expectedNegative.push(line); else unexpected.push(line);
    }
  });
  return { consoleErrors, uncaught, failedRequests, unexpected, expectedNegative };
}

async function selectFirstWorkload(page) {
  const row = page.getByTestId("workload-row").first();
  const identity = await row.locator("h3").innerText();
  await row.getByRole("button", { name: "Open observations" }).click();
  await page.getByTestId("observations-heading").waitFor({ state: "visible" });
  const heading = await page.locator('[aria-labelledby="observations-heading"] .section-heading p').innerText();
  if (!heading.includes(identity)) throw new Error(`Selected workload identity was not retained after navigation: expected ${identity}, got ${heading}`);
}

async function qualifyHistory(page) {
  await page.getByRole("button", { name: "History" }).click();
  await page.getByTestId("history-view").waitFor({ state: "visible" });
  await page.getByTestId("history-empty").waitFor({ state: "visible" }).catch(() => undefined);
  const events = page.getByTestId("history-event");
  const count = await events.count();
  if (count) {
    await events.first().click();
    await page.getByTestId("history-detail").waitFor({ state: "visible" });
    const selected = await page.getByTestId("history-detail").innerText();
    if (!selected) throw new Error("History detail was empty after exact event selection");
    await openContextControls(page);
    await page.getByRole("button", { name: "Refresh" }).click();
    await page.getByTestId("history-detail").waitFor({ state: "visible" });
    await page.getByRole("combobox", { name: "Event" }).selectOption({ index: 1 }).catch(() => undefined);
    if (!(await page.getByTestId("history-detail").isVisible())) throw new Error("History filter erased exact selected detail");
  }
  return count;
}

async function qualifyAttention(page) {
  await page.getByRole("button", { name: "Attention" }).click();
  await page.getByTestId("attention-view").waitFor({ state: "visible" });
  await page.locator('[data-testid="attention-empty"], [data-testid="attention-item"], [data-testid="attention-view"] .empty-state.error').first().waitFor({ state: "visible", timeout: 120000 });
  const items = page.getByTestId("attention-item");
  const count = await items.count();
  if (count) {
    await items.first().click();
    await page.getByTestId("attention-detail").waitFor({ state: "visible" });
    if (!(await page.getByTestId("attention-detail").innerText())) throw new Error("Attention detail was empty after selection");
    await openContextControls(page);
    await page.getByRole("button", { name: "Refresh" }).click();
    await page.getByTestId("attention-detail").waitFor({ state: "visible" });
  } else {
    if (await page.locator(".empty-state.error").count()) throw new Error(`Attention authoritative read failed: ${await page.locator(".empty-state.error").innerText()}`);
    await page.getByTestId("attention-empty").waitFor({ state: "visible" });
    if (!(await page.getByTestId("attention-empty").innerText()).includes("does not establish security health")) throw new Error("Attention empty state implied health");
  }
  return count;
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  const integrity = captureBrowserIntegrity(page);
  try {
    await bind(page);
    await selectFirstWorkload(page);
    const historyCount = await qualifyHistory(page);
    await page.screenshot({ path: "/tmp/operations-center-migration-history-1440.png", fullPage: true });
    const attentionCount = await qualifyAttention(page);
    await page.screenshot({ path: "/tmp/operations-center-migration-attention-1440.png", fullPage: true });
    for (const width of [1440, 1280, 1024, 680]) {
      await page.setViewportSize({ width, height: 900 });
      if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)) {
        const offenders = await page.evaluate(() => [...document.querySelectorAll("body *")].filter(element => element.getBoundingClientRect().right > window.innerWidth + 1).slice(0, 8).map(element => ({ tag: element.tagName, className: element.className, text: (element.textContent || "").slice(0, 100), right: element.getBoundingClientRect().right })));
        throw new Error(`horizontal overflow at ${width}px ${JSON.stringify(offenders)}`);
      }
      await page.screenshot({ path: `/tmp/operations-center-migration-m7-${width}.png`, fullPage: true });
    }

    const second = await context.newPage({ viewport: { width: 1024, height: 900 } });
    const secondIntegrity = captureBrowserIntegrity(second);
    await bind(second);
    await selectFirstWorkload(second);
    await second.getByRole("button", { name: "History" }).click();
    await second.getByTestId("history-view").waitFor({ state: "visible" });
    const firstA = page.getByTestId("attention-view");
    const firstASelected = await firstA.isVisible();
    await second.getByRole("button", { name: "Attention" }).click();
    await second.getByTestId("attention-view").waitFor({ state: "visible" });
    if (!firstASelected) throw new Error("Tab A surface was lost while Tab B navigated");
    await second.close();
    const merged = {
      consoleErrors: [...integrity.consoleErrors, ...secondIntegrity.consoleErrors],
      uncaught: [...integrity.uncaught, ...secondIntegrity.uncaught],
      failedRequests: [...integrity.failedRequests, ...secondIntegrity.failedRequests],
      unexpected: [...integrity.unexpected, ...secondIntegrity.unexpected],
      expectedNegative: [...integrity.expectedNegative, ...secondIntegrity.expectedNegative],
    };
    if (merged.consoleErrors.length || merged.uncaught.length || merged.failedRequests.length || merged.unexpected.length) throw new Error(JSON.stringify(merged));
    console.log(JSON.stringify({ historyCount, attentionCount, browser: merged }));
  } finally {
    await context.close();
    await browser.close();
  }
}

main().catch(error => { console.error(error.stack || error); process.exitCode = 1; });
