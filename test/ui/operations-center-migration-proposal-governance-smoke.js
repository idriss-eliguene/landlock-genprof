const { chromium } = require("playwright");

const url = process.env.UI_MIGRATION_URL || "http://127.0.0.1:18093/next/";
const identity = process.env.UI_MIGRATION_IDENTITY || "developer";
const namespace = process.env.UI_MIGRATION_NAMESPACE || "payments";

async function bind(page, targetUrl = url) {
  await page.goto(targetUrl, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible" });
  await page.getByTestId("context-identity").locator("option").nth(1).waitFor({ state: "attached" });
  await page.getByTestId("context-identity").selectOption(identity);
  await page.getByTestId("context-namespace").locator(`option[value="${namespace}"]`).waitFor({ state: "attached" });
  const currentNamespace = await page.getByTestId("context-namespace").inputValue();
  const workloadsResponse = currentNamespace === namespace
    ? null
    : page.waitForResponse(response => response.url().includes("/api/workloads") && response.status() === 200);
  await page.getByTestId("context-namespace").selectOption(namespace);
  if (workloadsResponse) await workloadsResponse;
  await page.waitForFunction((expectedNamespace) => {
    const namespaceSelect = document.querySelector('[data-testid="context-namespace"]');
    const meta = document.querySelector(".context-meta")?.textContent || "";
    return (namespaceSelect instanceof HTMLSelectElement && namespaceSelect.value === expectedNamespace) && /Version\s+\d+/.test(meta);
  }, namespace, { timeout: 120000 });
  await page.getByTestId("workload-row").first().waitFor({ state: "visible", timeout: 120000 });
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const browserContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await browserContext.grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(url).origin });
  const page = await browserContext.newPage();
  const consoleErrors = [];
  const failedRequests = [];
  const httpErrors = [];
  const expectedNegative = [];
  let allowExpectedApplyFailureConsole = false;
  page.on("console", message => { if (message.type() === "error" && !(allowExpectedApplyFailureConsole && message.text().includes("Failed to load resource"))) consoleErrors.push(message.text()); });
  page.on("pageerror", error => consoleErrors.push(`pageerror: ${error.message}`));
  page.on("requestfailed", request => { if (request.url().includes("/api/")) failedRequests.push(request.url()); });
  page.on("response", async response => { if (response.url().includes("generate-proposal")) { console.error(`GENERATE_RESPONSE ${response.status()} ${response.url()} ${await response.text().catch(() => "")}`); } if (response.status() >= 400 && response.url().includes("/api/")) { if ((response.status() === 409 && response.url().includes("/api/governance/")) || (response.status() === 500 && response.url().includes("/api/governance/") && response.url().endsWith("/apply"))) expectedNegative.push(`${response.status()} ${response.url()}`); else httpErrors.push(`${response.status()} ${response.url()}`); } });
  page.on("request", request => { if (request.url().includes("generate-proposal")) console.error(`GENERATE_REQUEST ${request.method()} ${request.url()}`); });
  try {
    await bind(page);
    const workload = page.getByTestId("workload-row").first();
    const workloadText = await workload.innerText();
    const workloadUID = workloadText.match(/UID\s+(\S+)/)?.[1];
    await workload.getByRole("button", { name: "Open observations" }).click();
    await page.getByTestId("observations-heading").waitFor({ state: "visible" });

    const startResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/start") && response.request().method() === "POST");
    await page.getByTestId("start-observation").click();
    const started = await startResponse;
    const observationID = (await started.json()).observationID;
    await page.getByTestId("observation-detail").waitFor({ state: "visible" });
    await page.getByTestId("stop-observation").waitFor({ state: "visible" });
    const stopResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/stop") && response.request().method() === "POST");
    await page.getByTestId("stop-observation").click();
    const stopped = await stopResponse;
    if (stopped.status() !== 200) throw new Error(`stop failed before Proposal journey: ${stopped.status()} ${await stopped.text()}`);
    await page.waitForFunction(() => {
      const detail = document.querySelector('[data-testid="observation-detail"]');
      return detail && /Completed|Failed/.test(detail.querySelector(".detail-heading .status-pill")?.textContent || "") && /Frozen\s+Yes/i.test(detail.textContent || "");
    }, undefined, { timeout: 120000 });
    const detailText = await page.getByTestId("observation-detail").innerText();
    const evidenceText = await page.getByTestId("evidence-summary").innerText();
    await page.getByTestId("generate-proposal").waitFor({ state: "visible" });
    const generateButton = page.getByTestId("generate-proposal");
    const generateResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/generate-proposal") && response.request().method() === "POST");
    generateResponse.catch(() => undefined);
    await generateButton.click();
    await page.waitForFunction(() => document.querySelector('[data-testid="generate-proposal"]')?.hasAttribute("disabled"), undefined, { timeout: 5000 });
    const generated = await generateResponse;
    const generatedBody = await generated.json();
    const proposalName = generatedBody.proposalName;
    if (generated.status() !== 200 || !proposalName) throw new Error(`proposal generation failed: ${generated.status()} ${JSON.stringify(generatedBody)}`);
    await page.getByTestId("proposal-detail").waitFor({ state: "visible" });
    if (!(await page.getByTestId("proposal-detail").innerText()).includes(proposalName)) throw new Error("exact generated Proposal identity was not rendered");
    await page.getByRole("tab", { name: "Derived YAML" }).click();
    await page.getByTestId("proposal-yaml").waitFor({ state: "visible" });
    const yamlText = await page.getByTestId("proposal-yaml").innerText();
    await page.getByRole("button", { name: "Copy YAML" }).click();
    await page.getByRole("button", { name: "Copied" }).waitFor({ state: "visible" });
    await page.screenshot({ path: "/tmp/operations-center-migration-proposal-yaml-1440.png", fullPage: true });
    await page.getByRole("tab", { name: "Canonical JSON" }).click();
    await page.getByTestId("proposal-json").waitFor({ state: "visible" });
    const jsonText = await page.getByTestId("proposal-json").innerText();
    await page.getByRole("button", { name: "Copy JSON" }).click();
    await page.getByRole("button", { name: "Copied" }).waitFor({ state: "visible" });
    await page.screenshot({ path: "/tmp/operations-center-migration-proposal-json-1440.png", fullPage: true });
    await page.getByRole("button", { name: "Refresh" }).click();
    await page.getByTestId("proposal-detail").waitFor({ state: "visible" });
    if (!(await page.getByTestId("proposal-json").isVisible())) throw new Error("Canonical JSON representation did not survive refresh");
    if (!(await page.getByTestId("proposal-detail").innerText()).includes(proposalName)) throw new Error("Proposal selection did not survive refresh");
    const reviewResponse = page.waitForResponse(response => response.url().includes(`/api/governance/proposals/${proposalName}/review`) && response.request().method() === "POST");
    await page.getByRole("button", { name: "Review" }).click();
    const reviewed = await reviewResponse;
    if (reviewed.status() !== 200) throw new Error(`review failed: ${reviewed.status()} ${await reviewed.text()}`);
    await page.waitForFunction(() => /State:\s*Reviewed/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || ""), undefined, { timeout: 15000 });
    await page.screenshot({ path: "/tmp/operations-center-migration-proposal-governance-1440.png", fullPage: true });

    if (process.env.QUALIFY_REJECT === "1") {
      const rejectResponse = page.waitForResponse(response => response.url().includes(`/api/governance/proposals/${proposalName}/reject`) && response.request().method() === "POST");
      await page.getByRole("button", { name: "Reject" }).click();
      const rejected = await rejectResponse;
      if (rejected.status() !== 200) throw new Error(`reject failed: ${rejected.status()} ${await rejected.text()}`);
      await page.waitForFunction(() => /State:\s*Rejected/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || ""), undefined, { timeout: 15000 });
      await page.getByTestId("proposal-json").waitFor({ state: "visible" });
      await page.getByRole("button", { name: "Refresh" }).click();
      await page.waitForFunction(() => /State:\s*Rejected/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || "") && document.querySelector('[data-testid="proposal-json"]') !== null, undefined, { timeout: 30000 });
      await page.screenshot({ path: "/tmp/operations-center-migration-proposal-rejected-1440.png", fullPage: true });
      if (consoleErrors.length || failedRequests.length || httpErrors.length) throw new Error(JSON.stringify({ consoleErrors, failedRequests, httpErrors }));
      console.log(JSON.stringify({ observationID, proposalName, workloadUID, reviewHTTP: reviewed.status(), rejectHTTP: rejected.status(), expectedNegative, consoleErrors, failedRequests, httpErrors }));
      return;
    }

    const second = await browserContext.newPage({ viewport: { width: 1024, height: 900 } });
    const secondErrors = [];
    const secondExpectedNegative = [];
    second.on("console", message => { if (message.type() === "error" && !message.text().includes("Failed to load resource")) secondErrors.push(message.text()); });
    second.on("pageerror", error => secondErrors.push(`pageerror: ${error.message}`));
    second.on("response", response => { if (response.status() >= 400 && response.url().includes("/api/")) { if (response.status() === 409 && response.url().includes("/api/governance/")) secondExpectedNegative.push(`${response.status()} ${response.url()}`); else secondErrors.push(`${response.status()} ${response.url()}`); } });
    await bind(second, `${url}?proposal=${encodeURIComponent(proposalName)}`);
    await second.getByRole("navigation").getByRole("button", { name: "Proposals", exact: true }).click();
    await second.getByTestId("proposal-detail").waitFor({ state: "visible" });
    await second.waitForFunction(() => /State:\s*Reviewed/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || ""), undefined, { timeout: 30000 });
    await second.getByRole("tab", { name: "Structured" }).click();
    const approveA = page.waitForResponse(response => response.url().includes(`/api/governance/proposals/${proposalName}/approve`) && response.request().method() === "POST");
    await page.getByRole("button", { name: "Approve" }).click();
    const approved = await approveA;
    if (approved.status() !== 200) throw new Error(`approve failed: ${approved.status()}`);
    await page.waitForFunction(() => /State:\s*Approved/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || ""), undefined, { timeout: 15000 });
    const staleApprove = second.waitForResponse(response => response.url().includes(`/api/governance/proposals/${proposalName}/approve`) && response.request().method() === "POST");
    await second.getByRole("button", { name: "Approve" }).click();
    const stale = await staleApprove;
    if (stale.status() !== 409) throw new Error(`stale approve returned ${stale.status()}, expected 409`);
    await second.getByRole("alert").filter({ hasText: "Proposal changed" }).waitFor({ state: "visible" });
    await second.getByTestId("proposal-detail").waitFor({ state: "visible" });
    const applyResponse = page.waitForResponse(response => response.url().includes(`/api/governance/proposals/${proposalName}/apply`) && response.request().method() === "POST");
    allowExpectedApplyFailureConsole = true;
    await page.getByRole("button", { name: "Apply" }).click();
    const applied = await applyResponse;
    if (applied.status() === 200) {
      await page.getByRole("alert").filter({ hasText: "Apply accepted" }).waitFor({ state: "visible" });
    } else if (applied.status() === 500) {
      await page.waitForFunction(() => [...document.querySelectorAll('[role="alert"]')].some(element => (element.textContent || "").includes("canonical target binding")), undefined, { timeout: 15000 });
      const applyText = (await page.getByRole("alert").allTextContents()).find(text => text.includes("canonical target binding")) || "";
      if (!applyText.includes("canonical target binding") || applyText.includes("Apply accepted")) throw new Error(`unexpected Apply failure feedback: ${applyText}`);
    } else {
      throw new Error(`apply failed: ${applied.status()} ${await applied.text()}`);
    }
    await page.getByRole("button", { name: "Refresh" }).click();
    for (const width of [1440, 1280, 1024, 680]) {
      await page.setViewportSize({ width, height: 900 });
      if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)) throw new Error(`horizontal overflow at ${width}px`);
      await page.screenshot({ path: `/tmp/operations-center-migration-proposal-${width}.png`, fullPage: true });
    }
    await second.close();
    if (secondErrors.length) throw new Error(secondErrors.join("\n"));
    if (applied.status() === 500) expectedNegative.push(`${applied.status()} ${applied.url()}`);
    if (consoleErrors.length || failedRequests.length || httpErrors.length) throw new Error(JSON.stringify({ consoleErrors, failedRequests, httpErrors }));
    console.log(JSON.stringify({ observationID, proposalName, workloadUID, stopHTTP: stopped.status(), generateHTTP: generated.status(), yamlBytes: yamlText.length, jsonBytes: jsonText.length, reviewHTTP: reviewed.status(), approveHTTP: approved.status(), staleHTTP: stale.status(), applyHTTP: applied.status(), expectedNegative: [...expectedNegative, ...secondExpectedNegative], evidence: evidenceText, detail: detailText, consoleErrors, failedRequests, httpErrors }));
  } finally {
    await browserContext.close();
    await browser.close();
  }
}

main().catch(error => { console.error(error.stack || error); process.exitCode = 1; });
