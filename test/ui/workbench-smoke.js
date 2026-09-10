const { chromium } = require("playwright");

const url = process.env.UI_URL || "http://127.0.0.1:8090";
const expectedWorkload = process.env.UI_EXPECTED_WORKLOAD || "";
const errors = [];

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  page.on("console", message => {
    if (message.type() === "error") errors.push(`console: ${message.text()}`);
  });
  page.on("pageerror", error => errors.push(`pageerror: ${error.message}`));
  page.on("requestfailed", request => {
    if (request.url().includes("/api/")) errors.push(`request: ${request.url()} ${request.failure()?.errorText || "failed"}`);
  });

  await page.goto(url, { waitUntil: "networkidle" });
  const surfaces = ["overview", "workloads", "observations", "proposals", "history", "attention"];
  for (const surface of surfaces) {
    await page.locator(`[data-view="${surface}"]`).click();
    await page.locator(`#${surface}-view`).waitFor({ state: "visible" });
  }

  await page.locator('[data-view="workloads"]').click();
  if (!(await page.locator(".workload-row").count())) {
    const response = await page.request.get(`${url}/api/workloads`);
    throw new Error(`UI rendered no selectable workload rows; API=${await response.text()}`);
  }
  if (expectedWorkload && !(await page.locator(".workload-row").first().innerText()).includes(expectedWorkload)) {
    throw new Error(`discovered workload does not contain ${expectedWorkload}`);
  }
  await page.locator(".workload-row").first().getByRole("button", { name: "Inspect" }).click();
  await page.locator("#observations-view").waitFor({ state: "visible" });
  if (!(await page.locator(".workload-row.selected").count())) throw new Error("UI did not retain a selected canonical container");
  const selected = await page.locator("#workload-picker option").nth(1).getAttribute("value");
  if (!selected) throw new Error("UI did not expose a canonical container in the picker");

  for (const width of [1280, 1024, 680]) {
    await page.setViewportSize({ width, height: 900 });
    await page.locator('[data-view="attention"]').click();
    if (!(await page.locator('[data-view="attention"]').isVisible())) throw new Error(`navigation unavailable at width ${width}`);
  }
  if (errors.length) throw new Error(errors.join("\n"));
  console.log(JSON.stringify({ surfaces, selectedWorkload: JSON.parse(selected), consoleErrors: 0, failedApiRequests: 0, uncaughtExceptions: 0 }));
  await browser.close();
})().catch(error => {
  console.error(error.stack || error.message);
  process.exitCode = 1;
});
