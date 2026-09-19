const { chromium } = require("playwright");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:8090/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const namespace = process.env.UI_MIGRATION_NAMESPACE || "payments";

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    const startedAt = Date.now();
    const timings = {};
    const failedResponses = [];
    const mark = name => { timings[name] = Date.now() - startedAt; };
    page.on("request", request => {
      const path = new URL(request.url()).pathname;
      if (path === "/api/workloads" && timings.workloadRequestStart === undefined) timings.workloadRequestStart = Date.now() - startedAt;
    });
    page.on("response", response => {
      const path = new URL(response.url()).pathname;
      if (path === "/api/workloads" && timings.workloadResponse === undefined) timings.workloadResponse = Date.now() - startedAt;
      if (response.status() >= 400 && response.url().includes("/api/")) failedResponses.push(`${response.status()} ${response.url()}`);
    });
    const unexpected = [];
    page.on("console", message => { if (message.type() === "error") unexpected.push(`console: ${message.text()}`); });
    page.on("pageerror", error => unexpected.push(`pageerror: ${error.message}`));
    page.on("requestfailed", request => { if (request.url().includes("/api/")) unexpected.push(`request: ${request.url()}`); });
    await page.goto(url, { waitUntil: "domcontentloaded" });
    await page.getByTestId("migration-app").waitFor({ state: "visible" });
    mark("pageReady");
    await page.getByTestId("context-identity").locator("option").nth(1).waitFor({ state: "attached" });
    await page.getByTestId("context-identity").selectOption(identity);
    await page.getByTestId("context-namespace").locator(`option[value="${namespace}"]`).waitFor({ state: "attached" });
    await page.getByTestId("context-namespace").selectOption(namespace);
    mark("environmentReady");
    await page.getByTestId("workload-row").first().waitFor({ state: "visible" });
    mark("workloadRendered");
    const workloadText = await page.getByTestId("workload-row").first().innerText();
    const workloadUID = workloadText.match(/UID\s+(\S+)/)?.[1];
    if (!workloadUID) throw new Error("selected workload UID was not rendered");
    await page.getByTestId("workload-row").first().getByRole("button", { name: "Open observations" }).click();
    await page.getByTestId("observations-heading").waitFor({ state: "visible" });
    await page.getByTestId("start-observation").waitFor({ state: "visible" });
    const startResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/start") && response.request().method() === "POST");
    mark("startClicked");
    await page.getByTestId("start-observation").click();
    const started = await startResponse;
    mark("startResponse");
    if (started.status() !== 200) throw new Error(`start returned ${started.status()}`);
    try { await page.getByTestId("observation-detail").waitFor({ state: "visible" }); } catch (error) { console.error(JSON.stringify({ bodyTail: (await page.locator("body").innerText()).slice(-1000), unexpected })); throw error; }
    const startBody = await started.json();
    const id = startBody.observationID;
    if (!id) throw new Error("exact Observation ID was not rendered");
    if ((await page.getByTestId("observation-detail").locator(".technical-id code").textContent()) !== id) throw new Error("returned Observation ID was not rendered as the selected detail identity");
    mark("activeDetail");
    const second = await browser.newPage({ viewport: { width: 1024, height: 900 } });
    const secondErrors = [];
    second.on("console", message => { if (message.type() === "error") secondErrors.push(`console: ${message.text()}`); });
    second.on("pageerror", error => secondErrors.push(`pageerror: ${error.message}`));
    second.on("requestfailed", request => { if (request.url().includes("/api/")) secondErrors.push(`request: ${request.url()}`); });
    second.on("response", response => { if (response.status() >= 400 && response.url().includes("/api/")) secondErrors.push(`${response.status()} ${response.url()}`); });
    await second.goto(url, { waitUntil: "domcontentloaded" });
    await second.getByTestId("migration-app").waitFor({ state: "visible" });
    await second.getByTestId("context-identity").locator("option").nth(1).waitFor({ state: "attached" });
    await second.getByTestId("context-identity").selectOption(identity);
    await second.getByTestId("context-namespace").locator(`option[value="${namespace}"]`).waitFor({ state: "attached" });
    await second.getByTestId("context-namespace").selectOption(namespace);
    const secondWorkload = second.getByTestId("workload-row").filter({ hasText: `UID ${workloadUID}` });
    await secondWorkload.waitFor({ state: "visible" });
    await secondWorkload.getByRole("button", { name: "Open observations" }).click();
    await second.getByTestId("observations-heading").waitFor({ state: "visible" });
    const secondStartResponse = second.waitForResponse(response => response.url().endsWith("/api/observations/start") && response.request().method() === "POST");
    await second.getByTestId("start-observation").click();
    const secondStarted = await secondStartResponse;
    if (secondStarted.status() !== 200) throw new Error(`second tab start returned ${secondStarted.status()}`);
    const secondID = (await secondStarted.json()).observationID;
    if (!secondID || secondID === id) throw new Error("second tab did not receive an independent Observation identity");
    await second.getByTestId("observation-detail").waitFor({ state: "visible" });
    if ((await second.getByTestId("observation-detail").locator(".technical-id code").textContent()) !== secondID) throw new Error("second tab did not render its exact Observation identity");
    const stop = page.getByTestId("stop-observation");
    await stop.waitFor({ state: "visible" });
    await page.screenshot({ path: "/tmp/operations-center-migration-observation-active-1280.png", fullPage: true });
    const stopResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/stop") && response.request().method() === "POST");
    mark("stopClicked");
    await stop.click();
    const stopped = await stopResponse;
    mark("stopResponse");
    if (stopped.status() !== 200) throw new Error(`stop returned ${stopped.status()}`);
    await page.waitForFunction(() => {
      const detail = document.querySelector('[data-testid="observation-detail"]');
      const state = detail?.querySelector(".detail-heading .status-pill")?.textContent || "";
      return /Completed|Failed/.test(state) && /Frozen\s+Yes/i.test(detail?.innerText || "");
    }, undefined, { timeout: 90_000 });
    mark("terminal");
    const finalText = await page.getByTestId("observation-detail").innerText();
    const evidenceBeforeRefresh = await page.getByTestId("evidence-summary").innerText();
    mark("evidenceDetail");
    await page.getByRole("button", { name: "Refresh" }).click();
    await page.getByTestId("observation-detail").locator(".technical-id code").waitFor({ state: "visible" });
    if ((await page.getByTestId("observation-detail").locator(".technical-id code").textContent()) !== id) throw new Error("collection refresh changed exact Observation selection");
    if ((await page.getByTestId("evidence-summary").innerText()) !== evidenceBeforeRefresh) throw new Error("collection refresh changed exact Evidence detail");
    if ((await second.getByTestId("observation-detail").locator(".technical-id code").textContent()) !== secondID) throw new Error("first tab Stop changed the second tab Observation selection");
    const secondStopResponse = second.waitForResponse(response => response.url().endsWith("/api/observations/stop") && response.request().method() === "POST");
    await second.getByTestId("stop-observation").click();
    const secondStopped = await secondStopResponse;
    if (secondStopped.status() !== 200) throw new Error(`second tab stop returned ${secondStopped.status()}`);
    await second.waitForFunction(() => {
      const detail = document.querySelector('[data-testid="observation-detail"]');
      const state = detail?.querySelector(".detail-heading .status-pill")?.textContent || "";
      return /Completed|Failed/.test(state) && /Frozen\s+Yes/i.test(detail?.innerText || "");
    }, undefined, { timeout: 90_000 });
    await page.screenshot({ path: "/tmp/operations-center-migration-observation-1280.png", fullPage: true });
    for (const width of [1440, 1024, 680]) {
      await page.setViewportSize({ width, height: 900 });
      if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)) throw new Error(`horizontal overflow at ${width}px`);
      await page.screenshot({ path: `/tmp/operations-center-migration-observation-${width}.png`, fullPage: true });
    }
    await page.keyboard.press("Tab");
    if (!(await page.evaluate(() => document.activeElement instanceof HTMLElement && Boolean(document.activeElement))) ) throw new Error("keyboard focus was not visible on a primary control");
    await second.close();
    if (secondErrors.length) throw new Error(secondErrors.join("\n"));
    if ((await page.getByTestId("observation-detail").locator(".technical-id code").textContent()) !== id) throw new Error("second tab changed the first tab Observation selection");
    if (unexpected.length) throw new Error(unexpected.join("\n"));
    if (failedResponses.length) throw new Error(`unexpected failed API responses:\n${failedResponses.join("\n")}`);
    console.log(JSON.stringify({ observationID: id, startHTTP: started.status(), stopHTTP: stopped.status(), lifecycle: await page.getByTestId("observation-lifecycle").innerText(), finalText, evidence: evidenceBeforeRefresh, timings: { pageReadyMs: timings.pageReady, environmentReadyMs: timings.environmentReady, workloadRequestStartMs: timings.workloadRequestStart, workloadResponseMs: timings.workloadResponse, workloadReadMs: timings.workloadResponse - timings.workloadRequestStart, workloadRenderedMs: timings.workloadRendered, startResponseMs: timings.startResponse - timings.startClicked, observationActiveMs: timings.activeDetail - timings.startResponse, stopResponseMs: timings.stopResponse - timings.stopClicked, terminalConvergenceMs: timings.terminal - timings.stopResponse, evidenceReadMs: timings.evidenceDetail - timings.terminal }, browserErrors: 0 }));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.stack || error); process.exitCode = 1; });
