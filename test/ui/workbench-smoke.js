const { chromium } = require("playwright");
const { execFileSync } = require("child_process");

const url = process.env.UI_URL || "http://127.0.0.1:8090";
const expectedWorkload = process.env.UI_EXPECTED_WORKLOAD || "";
const namespace = process.env.UI_NAMESPACE || "";
const pod = process.env.UI_POD || "";
const container = process.env.UI_CONTAINER || "nginx";
const errors = [];
let browser;

(async () => {
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  page.on("console", message => {
    if (message.type() === "error") errors.push(`console: ${message.text()}`);
  });
  page.on("pageerror", error => errors.push(`pageerror: ${error.message}`));
  page.on("requestfailed", request => {
    if (request.url().includes("/api/")) errors.push(`request: ${request.url()} ${request.failure()?.errorText || "failed"}`);
  });
  page.on("response", response => {
    if (response.url().includes("/api/") && response.status() >= 400 && response.status() !== 401) {
      errors.push(`response: ${response.url()} HTTP ${response.status()}`);
    }
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

  // Exercise the real production-like Observation path. The browser issues
  // the start/generate requests through the trusted proxy; kubectl is used
  // only by this host-side test driver to create controlled workload activity.
  await page.locator('[data-view="observations"]').click();
  const startResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/start") && response.request().method() === "POST");
  await page.getByRole("button", { name: "Start observation" }).click();
  const started = await startResponse;
  if (started.status() >= 400) throw new Error(`Start Observation rejected: HTTP ${started.status()} ${await started.text()}`);
  const selectedContext = JSON.parse(selected);
  const observationQuery = new URLSearchParams({
    kind: selectedContext.kind,
    name: selectedContext.name,
    container: selectedContext.container,
    workloadUID: selectedContext.uid,
    group: selectedContext.group || "",
    imageIdentity: selectedContext.image || "",
  }).toString();
  let observation;
  for (let i = 0; i < 45; i++) {
    const body = await page.evaluate(async query => (await fetch("/api/observations?" + query)).json(), observationQuery);
    observation = (body.items || [])[0];
    if (observation && ["CLAIMED", "RUNNING", "COMPLETING", "COMPLETED"].includes(observation.execution?.state)) break;
    await page.waitForTimeout(1000);
  }
  if (!observation) throw new Error("Observation was created but was not visible through the canonical workload query");
  const observationID = observation.observationID;
  if (["REQUESTED", "PENDING"].includes(observation.execution?.state)) throw new Error(`Observation was not claimed: ${observation.execution.state}`);
  try {
    execFileSync("kubectl", ["-n", namespace, "exec", pod, "-c", container, "--", "sh", "-c", "cat /etc/hostname >/dev/null; printf qualification > /tmp/landlock-genprof-ui-flow; cat /tmp/landlock-genprof-ui-flow >/dev/null; rm -f /tmp/landlock-genprof-ui-flow"], { stdio: "pipe" });
  } catch (error) {
    throw new Error(`controlled filesystem activity failed: ${error.stderr?.toString() || error.message}`);
  }
  for (let i = 0; i < 90; i++) {
    const body = await page.evaluate(async query => (await fetch("/api/observations?" + query)).json(), observationQuery);
    observation = (body.items || []).find(item => item.observationID === observationID) || observation;
    if (observation.execution?.state === "COMPLETED") break;
    await page.waitForTimeout(1000);
  }
  if (observation.execution?.state !== "COMPLETED") throw new Error(`Observation did not naturally complete: ${observation.execution?.state || "missing"}`);
  await page.locator("#observation-list button").filter({ hasText: observationID }).click();
  await page.getByRole("button", { name: "Generate proposal" }).click();
  let proposal;
  for (let i = 0; i < 20; i++) {
    const body = await page.evaluate(async query => (await fetch("/api/proposals?" + query)).json(), observationQuery);
    proposal = (body.items || [])[0];
    if (proposal) break;
    await page.waitForTimeout(500);
  }
  if (!proposal) throw new Error(`Proposal was not generated for Observation ${observationID}`);
  await page.locator('[data-view="proposals"]').click();
  if (!(await page.locator(".proposal-row").count())) throw new Error("Proposal surface did not render the generated proposal");
  const historyResponse = page.waitForResponse(response => response.url().includes("/api/v08/history?") && response.request().method() === "GET");
  await page.locator('[data-view="history"]').click();
  const historyResult = await historyResponse;
  if (historyResult.status() !== 200) throw new Error(`History request was not accepted: HTTP ${historyResult.status()} ${await historyResult.text()}`);
  await page.waitForTimeout(100);
  const historyText = await page.locator("#history-detail").innerText();
  if (historyText.includes("UNAVAILABLE") || historyText.includes("EMPTY")) {
    throw new Error(`History rejected the canonical discovered image identity; selected=${selected}`);
  }

  for (const width of [1280, 1024, 680]) {
    await page.setViewportSize({ width, height: 900 });
    await page.locator('[data-view="attention"]').click();
    if (!(await page.locator('[data-view="attention"]').isVisible())) throw new Error(`navigation unavailable at width ${width}`);
  }
  if (errors.length) throw new Error(errors.join("\n"));
  console.log(JSON.stringify({ surfaces, selectedWorkload: JSON.parse(selected), consoleErrors: 0, failedApiRequests: 0, uncaughtExceptions: 0 }));
  await browser.close();
})().catch(async error => {
  console.error(error.stack || error.message);
  // Closing the browser on every failure is part of the harness contract:
  // otherwise Playwright's child process keeps the parent shell alive and
  // prevents the shell-level cleanup trap from reclaiming its fixtures.
  try { await browser?.close(); } catch (_) { /* best effort after failure */ }
  process.exitCode = 1;
});
