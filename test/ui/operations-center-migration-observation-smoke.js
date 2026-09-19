const { chromium } = require("playwright");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:8090/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const namespace = process.env.UI_MIGRATION_NAMESPACE || "payments";

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    const unexpected = [];
    page.on("console", message => { if (message.type() === "error") unexpected.push(`console: ${message.text()}`); });
    page.on("pageerror", error => unexpected.push(`pageerror: ${error.message}`));
    page.on("requestfailed", request => { if (request.url().includes("/api/")) unexpected.push(`request: ${request.url()}`); });
    await page.goto(url, { waitUntil: "domcontentloaded" });
    await page.getByTestId("migration-app").waitFor({ state: "visible" });
    await page.getByTestId("context-identity").locator("option").nth(1).waitFor({ state: "attached" });
    await page.getByTestId("context-identity").selectOption(identity);
    await page.getByTestId("context-namespace").locator(`option[value="${namespace}"]`).waitFor({ state: "attached" });
    await page.getByTestId("context-namespace").selectOption(namespace);
    await page.getByTestId("workload-row").first().waitFor({ state: "visible" });
    await page.getByTestId("workload-row").first().getByRole("button", { name: "Open observations" }).click();
    await page.getByTestId("observations-heading").waitFor({ state: "visible" });
    await page.getByTestId("start-observation").waitFor({ state: "visible" });
    const startResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/start") && response.request().method() === "POST");
    await page.getByTestId("start-observation").click();
    const started = await startResponse;
    if (started.status() !== 200) throw new Error(`start returned ${started.status()}`);
    try { await page.getByTestId("observation-detail").waitFor({ state: "visible" }); } catch (error) { console.error(JSON.stringify({ bodyTail: (await page.locator("body").innerText()).slice(-1000), unexpected })); throw error; }
    const id = await page.getByTestId("observation-detail").locator("code").first().textContent();
    if (!id) throw new Error("exact Observation ID was not rendered");
    const stop = page.getByTestId("stop-observation");
    await stop.waitFor({ state: "visible" });
    const stopResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/stop") && response.request().method() === "POST");
    await stop.click();
    const stopped = await stopResponse;
    if (stopped.status() !== 200) throw new Error(`stop returned ${stopped.status()}`);
    await page.getByTestId("observation-lifecycle").getByText(/Completed|Failed|Finalizing/).first().waitFor({ state: "visible" });
    await page.screenshot({ path: "/tmp/operations-center-migration-observation-1280.png", fullPage: true });
    if (unexpected.length) throw new Error(unexpected.join("\n"));
    console.log(JSON.stringify({ observationID: id, startHTTP: started.status(), stopHTTP: stopped.status(), lifecycle: await page.getByTestId("observation-lifecycle").innerText(), browserErrors: 0 }));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.stack || error); process.exitCode = 1; });
