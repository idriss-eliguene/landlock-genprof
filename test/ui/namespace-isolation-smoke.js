const { chromium } = require("playwright");

const url = process.env.UI_URL || "http://127.0.0.1:18384";
const errors = [];
let browser;
async function workloadNames(page) {
  return page.locator(".workload-row td:first-child strong").allTextContents();
}

async function selectNamespace(page, namespace, expectedAfterSelection = namespace) {
  const requestHeaders = [];
  page.on("request", request => {
    if (request.url().endsWith("/api/workloads")) requestHeaders.push(request.headers());
  });
  const capabilityResponse = page.waitForResponse(response =>
    response.url().includes("/api/v09/environments/") && response.url().includes("/capabilities?namespace=" + encodeURIComponent(namespace)),
  );
  await page.locator("#namespace-selector").selectOption(namespace);
  const capability = await capabilityResponse;
  if (capability.status() !== 200 && capability.status() !== 409) {
    throw new Error(`namespace ${namespace} capability binding returned HTTP ${capability.status()}`);
  }
  await page.locator("#namespace-selector").waitFor({ state: "visible" });
  await page.waitForFunction(expected => document.querySelector("#namespace-selector").value === expected, expectedAfterSelection);
  const result = capability.status() === 200 ? await page.waitForResponse(response =>
    response.url().endsWith("/api/workloads") && response.request().method() === "GET",
  ) : null;
  await page.locator("#workload-list").waitFor({ state: "visible" });
  return { status: capability.status(), body: result ? await result.text() : await capability.text(), headers: requestHeaders.at(-1) };
}

(async () => {
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const expectedNegativeRequests = [];
  page.on("console", message => {
    if (message.type() === "error" && !message.text().includes("409 (Conflict)")) errors.push(`console: ${message.text()}`);
  });
  page.on("pageerror", error => errors.push(`pageerror: ${error.message}`));
  page.on("response", response => {
    if (response.status() === 409 && response.url().includes("/api/v09/environments/") && response.url().includes("/capabilities?namespace=security")) expectedNegativeRequests.push(response.url());
  });
  page.on("requestfailed", request => {
    if (request.url().includes("/api/")) errors.push(`request: ${request.url()}`);
  });

  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.locator("#namespace-selector option").nth(1).waitFor({ state: "attached" });
  await page.locator('[data-view="workloads"]').click();
  await page.locator(".workload-row").first().waitFor({ state: "visible" });

  const payments = await workloadNames(page);
  if (!payments.includes("api") || !payments.includes("frontend")) {
    throw new Error(`payments workload set was not visible: ${JSON.stringify(payments)}`);
  }

  const securityResult = await selectNamespace(page, "security", "payments");
  const securityStatus = securityResult.status;
  const security = await workloadNames(page);
  if (securityStatus === 200 && (!security.includes("auditor") || security.includes("api") || security.includes("frontend"))) {
    throw new Error(`security workload set crossed namespace boundary: ${JSON.stringify(security)}`);
  }
  if (securityStatus === 409 && (!security.includes("api") || !security.includes("frontend") || security.includes("auditor"))) {
    throw new Error(`rejected security context lost the last authoritative workload projection: ${JSON.stringify(security)}`);
  }

  const paymentsAgainResult = await selectNamespace(page, "payments");
  const paymentsAgainStatus = paymentsAgainResult.status;
  const paymentsAgain = await workloadNames(page);
  if (paymentsAgainStatus !== 200 || !paymentsAgain.includes("api") || !paymentsAgain.includes("frontend") || paymentsAgain.includes("auditor")) {
    throw new Error(`payments reverse transition failed: status=${paymentsAgainStatus} body=${paymentsAgainResult.body} headers=${JSON.stringify(paymentsAgainResult.headers)} rows=${JSON.stringify(paymentsAgain)}`);
  }

  if (errors.length) throw new Error(errors.join("\n"));
  console.log(JSON.stringify({ payments, security, securityStatus, securityBody: securityResult.body, paymentsAgain, consoleErrors: 0, expectedNegativeRequests: expectedNegativeRequests.length }));
})().catch(error => {
  console.error(error.stack || error);
  process.exitCode = 1;
}).finally(async () => {
  await browser?.close();
});
