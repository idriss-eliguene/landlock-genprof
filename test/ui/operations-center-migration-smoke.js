const { chromium } = require("playwright");
const { bindNamespace, openContextControls } = require("./namespace-binding");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:8090/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const expectedNamespace = process.env.UI_MIGRATION_NAMESPACE || "payments";
const errors = [];
let browser;

(async () => {
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const expectedNegativeRequests = [];
  page.on("console", message => { if (message.type() === "error" && !message.text().includes("status of 409 (Conflict)")) errors.push(`console: ${message.text()}`); });
  page.on("pageerror", error => errors.push(`pageerror: ${error.message}`));
  page.on("requestfailed", request => { if (request.url().includes("/api/")) errors.push(`request: ${request.url()}`); });
  page.on("response", response => { if (response.status() === 409 && response.url().includes("/api/v09/environments/")) expectedNegativeRequests.push(response.url()); });

  // The Operations Center polls authoritative state. DOM and semantic resource
  // readiness, not network-idle, are the contract for this migration route.
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible" });
  await openContextControls(page);
  await bindNamespace(page, expectedNamespace, identity);
  await page.getByTestId("workload-row").first().waitFor({ state: "visible" });
  const beforeRejectedSwitch = await page.getByTestId("workload-row").allTextContents();
  await openContextControls(page);
  if (await page.getByTestId("context-namespace").locator('option[value="security"]').count()) {
    await page.getByTestId("context-namespace").selectOption("security");
    await page.getByRole("alert").waitFor({ state: "visible" });
    if (await page.getByTestId("context-namespace").inputValue() === "security") throw new Error("rejected namespace switch displayed an unauthorized namespace as bound");
    const afterRejectedSwitch = await page.getByTestId("workload-row").allTextContents();
    if (afterRejectedSwitch.length || JSON.stringify(afterRejectedSwitch) === JSON.stringify(beforeRejectedSwitch)) throw new Error("rejected namespace switch retained the prior workload projection");
    await bindNamespace(page, expectedNamespace, identity);
  }
  await page.getByTestId("workload-row").first().getByRole("button", { name: "Inspect workload" }).click();
  await page.getByTestId("workload-dossier").waitFor({ state: "visible" });
  await page.getByTestId("workload-dossier").getByText("Workload UID", { exact: true }).waitFor({ state: "visible" });
  const dossier = await page.getByTestId("workload-dossier").innerText();
  if (!dossier.toLowerCase().includes("verified workload dossier") || !dossier.includes("Workload UID")) throw new Error("selected workload dossier did not render authoritative identity");
  if (!page.url().includes("/next/workloads/")) throw new Error("workload selection did not navigate to the locator route");
  if (errors.length) throw new Error(errors.join("\n"));
  console.log(JSON.stringify({ migrationShell: "ready", workloadYAML: "visible", browserErrors: 0, expectedNegativeRequests: expectedNegativeRequests.length }));
  await browser.close();
})().catch(async error => { console.error(error.stack || error); if (browser) await browser.close(); process.exitCode = 1; });
