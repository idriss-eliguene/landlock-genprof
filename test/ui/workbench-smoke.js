const { chromium } = require("playwright");
const { execFileSync } = require("child_process");

const url = process.env.UI_URL || "http://127.0.0.1:8090";
const expectedWorkload = process.env.UI_EXPECTED_WORKLOAD || "";
const namespace = process.env.UI_NAMESPACE || "";
const pod = process.env.UI_POD || "";
const container = process.env.UI_CONTAINER || "nginx";
const errors = [];
let browser;

async function responseJSON(response) {
  const text = await response.text();
  let body = null;
  try { body = text ? JSON.parse(text) : null; } catch (_) { /* caller reports raw text */ }
  return { status: response.status(), body, text };
}

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

  // Exercise the real production-like capability Observation path. The
  // browser issues the start/generate requests through the trusted proxy;
  // kubectl is used only by this host-side test driver to create controlled
  // workload activity. The container-capability Proposal derivation contract
  // requires the supported `capabilities` source; filesystem-only evidence
  // is intentionally not converted into capability facts.
  await page.locator('[data-view="observations"]').click();
  const startResponse = page.waitForResponse(response => response.url().endsWith("/api/observations/start") && response.request().method() === "POST");
  await page.evaluate(async request => {
    const response = await fetch("/api/observations/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    });
    if (!response.ok) throw new Error(`Start Observation rejected: HTTP ${response.status} ${await response.text()}`);
  }, { namespace, pod, container, sources: ["capabilities"], duration: 60000000000 });
  const started = await startResponse;
  if (started.status() >= 400) throw new Error(`Start Observation rejected: HTTP ${started.status()} ${await started.text()}`);
  const startedBody = await started.json();
  const observationID = startedBody.observationID;
  if (!observationID) throw new Error("Start Observation returned no authoritative observationID");
  const selectedContext = JSON.parse(selected);
  const observationQuery = new URLSearchParams({
    kind: selectedContext.kind,
    name: selectedContext.name,
    container: selectedContext.container,
    workloadUID: selectedContext.uid,
    group: selectedContext.group || "",
    imageIdentity: selectedContext.image || "",
  }).toString();
  const observationQueryWithoutImage = new URLSearchParams({
    kind: selectedContext.kind,
    name: selectedContext.name,
    container: selectedContext.container,
    workloadUID: selectedContext.uid,
    group: selectedContext.group || "",
  }).toString();
  const readObservation = async id => {
    const response = await page.evaluate(async observationID => {
      const result = await fetch("/api/observations/" + encodeURIComponent(observationID));
      return {status: result.status, body: await result.text()};
    }, id);
    if (response.status !== 200) throw new Error(`Observation detail read failed: HTTP ${response.status} ${response.body}`);
    const observation = JSON.parse(response.body);
    // observationProjection serializes the domain ObservationExecution value
    // with its exported Go field names.  Keep this parser strict: an unknown
    // response shape must fail rather than being interpreted as a terminal
    // state or fabricated evidence.
    if (!observation || typeof observation !== "object" || !observation.execution || typeof observation.execution.State !== "string" || (observation.sources !== null && !Array.isArray(observation.sources))) {
      throw new Error(`Observation detail has unsupported authoritative shape: ${response.body}`);
    }
    return observation;
  };
  const observationState = observation => observation.execution.State;
  let observation;
  for (let i = 0; i < 45; i++) {
    observation = await readObservation(observationID);
    if (observation && ["STARTING", "RUNNING", "COMPLETING", "COMPLETED"].includes(observationState(observation))) break;
    await page.waitForTimeout(1000);
  }
  if (!observation) throw new Error(`Observation ${observationID} was not readable through its authoritative detail endpoint`);
  if (["REQUESTED", "PENDING"].includes(observationState(observation))) throw new Error(`Observation was not claimed: ${observationState(observation)}`);
  try {
    execFileSync("kubectl", ["-n", namespace, "exec", pod, "-c", container, "--", "sh", "-c", "cat /etc/hostname >/dev/null; printf qualification > /tmp/landlock-genprof-ui-flow; cat /tmp/landlock-genprof-ui-flow >/dev/null; chown 65534:65534 /tmp/landlock-genprof-ui-flow 2>/dev/null || true; chown 0:0 /tmp/landlock-genprof-ui-flow 2>/dev/null || true; rm -f /tmp/landlock-genprof-ui-flow"], { stdio: "pipe" });
  } catch (error) {
    throw new Error(`controlled filesystem activity failed: ${error.stderr?.toString() || error.message}`);
  }
  for (let i = 0; i < 90; i++) {
    observation = await readObservation(observationID);
    if (observationState(observation) === "COMPLETED") break;
    await page.waitForTimeout(1000);
  }
  if (observationState(observation) !== "COMPLETED") throw new Error(`Observation did not naturally complete: ${observationState(observation) || "missing"}`);
  // The read-model selector is intentionally strict about canonical image
  // identity. Reload the actual Workloads -> Inspect path after the executor
  // has persisted its binding, then select the Observation rendered by the UI.
  // Discard any in-flight selection read from before the executor persisted
  // the binding. A page reload is the supported browser navigation boundary
  // and guarantees the next Inspect action has one authoritative read.
  await page.reload({ waitUntil: "networkidle" });
  await page.locator('[data-view="workloads"]').click();
  await page.locator(".workload-row").first().getByRole("button", { name: "Inspect" }).click();
  await page.locator("#observations-view").waitFor({ state: "visible" });
  try {
    await page.locator("#observation-list button").filter({ hasText: observationID }).waitFor({ state: "visible", timeout: 30000 });
  } catch (error) {
    const queryResponse = await page.request.get(`${url}/api/observations?${observationQuery}`);
    const queryBody = await queryResponse.text();
    const queryWithoutImageResponse = await page.request.get(`${url}/api/observations?${observationQueryWithoutImage}`);
    throw new Error(`${error.message}\nselectedContext=${JSON.stringify(selectedContext)}\ncompletedDetailIdentity=${JSON.stringify(observation.identity)}\ncanonical query=${observationQuery} HTTP ${queryResponse.status()} body=${queryBody}\nwithout-image query=${observationQueryWithoutImage} HTTP ${queryWithoutImageResponse.status()} body=${await queryWithoutImageResponse.text()}`);
  }
  await page.locator("#observation-list button").filter({ hasText: observationID }).click();
  const proposalRequest = page.waitForRequest(request =>
    request.url().includes("/api/observations/generate-proposal") && request.method() === "POST"
  );
  const proposalResponse = page.waitForResponse(response =>
    response.url().includes("/api/observations/generate-proposal") && response.request().method() === "POST"
  );
  await page.getByRole("button", { name: "Generate proposal" }).click();
  const generatedRequest = await proposalRequest;
  const generatedResponse = await proposalResponse;
  const generatedBody = await generatedResponse.text();
  if (generatedResponse.status() >= 400) {
    throw new Error(`Generate Proposal rejected: HTTP ${generatedResponse.status()} ${generatedBody}\nrequest=${generatedRequest.postData() || ""}`);
  }
  let proposal;
  for (let i = 0; i < 20; i++) {
    const body = await page.evaluate(async query => (await fetch("/api/proposals?" + query)).json(), observationQuery);
    proposal = (body.items || [])[0];
    if (proposal) break;
    await page.waitForTimeout(500);
  }
  if (!proposal) throw new Error(`Proposal was not generated for Observation ${observationID}`);
  const proposalName = proposal.name;
  const proposalInitialRV = proposal.resourceVersion;
  if (!proposalName || !proposalInitialRV) throw new Error(`Generated Proposal lacks authoritative name/resourceVersion: ${JSON.stringify(proposal)}`);

  const capabilityResponse = await page.request.get(`${url}/api/v08/capabilities`);
  const capabilityResult = await responseJSON(capabilityResponse);
  if (capabilityResult.status !== 200 || !capabilityResult.body?.capabilities) {
    throw new Error(`Capability discovery failed: HTTP ${capabilityResult.status} ${capabilityResult.text}`);
  }
  const capabilities = capabilityResult.body.capabilities;
  const canReview = capabilities["proposal.review"] === true;
  const canApprove = capabilities["proposal.approve"] === true;
  const canApply = capabilities["proposal.apply"] === true;
  if (!canReview || !canApprove) throw new Error(`Generated proposal is not governable by the authenticated qualification identity: ${JSON.stringify(capabilities)}`);

  await page.locator('[data-view="proposals"]').click();
  const proposalRow = page.locator(".proposal-row").filter({ hasText: proposalName });
  if (!(await proposalRow.count())) throw new Error(`Proposal surface did not render current-run proposal ${proposalName}`);
  const reviewResponse = page.waitForResponse(response => response.url().includes(`/api/governance/proposals/${encodeURIComponent(proposalName)}/review`) && response.request().method() === "POST");
  await proposalRow.getByRole("button", { name: "Review" }).click();
  const reviewed = await reviewResponse;
  if (reviewed.status() < 200 || reviewed.status() >= 300) throw new Error(`Review rejected: HTTP ${reviewed.status()} ${await reviewed.text()}`);
  const afterReviewResponse = await page.request.get(`${url}/api/proposals?${observationQuery}`);
  const afterReview = await responseJSON(afterReviewResponse);
  const reviewedProposal = (afterReview.body?.items || []).find(item => item.name === proposalName);
  if (!reviewedProposal || reviewedProposal.status?.approvalState !== "Reviewed" || reviewedProposal.status?.reviewedBy !== "qualification-operator") {
    throw new Error(`Review did not persist the server-derived actor/state: ${JSON.stringify(reviewedProposal)}`);
  }

  const staleResponse = await page.request.post(`${url}/api/governance/proposals/${encodeURIComponent(proposalName)}/approve`, {
    data: { expectedResourceVersion: proposalInitialRV, expectedDigest: reviewedProposal.candidateDigest },
  });
  const stale = await responseJSON(staleResponse);
  if (stale.status !== 409) throw new Error(`Stale governance request returned HTTP ${stale.status}, body=${stale.text}`);
  const afterStaleResponse = await page.request.get(`${url}/api/proposals?${observationQuery}`);
  const afterStale = await responseJSON(afterStaleResponse);
  const unchanged = (afterStale.body?.items || []).find(item => item.name === proposalName);
  if (!unchanged || unchanged.status?.approvalState !== "Reviewed") throw new Error(`Stale request changed proposal state or disappeared: ${JSON.stringify(unchanged)}`);

  const approveResponse = await page.request.post(`${url}/api/governance/proposals/${encodeURIComponent(proposalName)}/approve`, {
    data: { expectedResourceVersion: unchanged.resourceVersion, expectedDigest: unchanged.candidateDigest },
  });
  const approved = await responseJSON(approveResponse);
  if (approved.status < 200 || approved.status >= 300) throw new Error(`Approve rejected: HTTP ${approved.status} ${approved.text}`);
  const approvedReadResponse = await page.request.get(`${url}/api/proposals?${observationQuery}`);
  const approvedRead = await responseJSON(approvedReadResponse);
  const approvedProposal = (approvedRead.body?.items || []).find(item => item.name === proposalName);
  if (!approvedProposal || approvedProposal.status?.approvalState !== "Approved" || approvedProposal.status?.approvedBy !== "qualification-operator") {
    throw new Error(`Approve did not persist the server-derived actor/state: ${JSON.stringify(approvedProposal)}`);
  }
  // A second Proposal from the same real completed Observation is an independent
  // durable object, allowing the mutually exclusive Reject transition to be
  // qualified without forcing an illegal transition on the approved object.
  const rejectName = `${proposalName}-reject`;
  const rejectCreateResponse = await page.request.post(`${url}/api/observations/generate-proposal`, {
    data: { namespace, observationID, proposalName: rejectName },
  });
  const rejectCreated = await responseJSON(rejectCreateResponse);
  if (rejectCreated.status < 200 || rejectCreated.status >= 300) throw new Error(`Independent Reject Proposal generation failed: HTTP ${rejectCreated.status} ${rejectCreated.text}`);
  let rejectProposal;
  for (let i = 0; i < 20; i++) {
    const rejectReadResponse = await page.request.get(`${url}/api/proposals?${observationQuery}`);
    const rejectRead = await responseJSON(rejectReadResponse);
    rejectProposal = (rejectRead.body?.items || []).find(item => item.name === rejectName);
    if (rejectProposal) break;
    await page.waitForTimeout(500);
  }
  if (!rejectProposal) throw new Error(`Independent Reject Proposal ${rejectName} was not persisted`);
  const rejectReviewResponse = await page.request.post(`${url}/api/governance/proposals/${encodeURIComponent(rejectName)}/review`, { data: { expectedResourceVersion: rejectProposal.resourceVersion } });
  const rejectReviewed = await responseJSON(rejectReviewResponse);
  if (rejectReviewed.status < 200 || rejectReviewed.status >= 300) throw new Error(`Reject Proposal review failed: HTTP ${rejectReviewed.status} ${rejectReviewed.text}`);
  const rejectAfterReviewResponse = await page.request.get(`${url}/api/proposals?${observationQuery}`);
  const rejectAfterReview = await responseJSON(rejectAfterReviewResponse);
  rejectProposal = (rejectAfterReview.body?.items || []).find(item => item.name === rejectName);
  const rejectResponse = await page.request.post(`${url}/api/governance/proposals/${encodeURIComponent(rejectName)}/reject`, { data: { expectedResourceVersion: rejectProposal.resourceVersion } });
  const rejected = await responseJSON(rejectResponse);
  if (rejected.status < 200 || rejected.status >= 300) throw new Error(`Reject rejected: HTTP ${rejected.status} ${rejected.text}`);
  const attentionResponse = await page.request.get(`${url}/api/v08/environment`);
  const attention = await responseJSON(attentionResponse);
  const attentionDiagnostics = Array.isArray(attention.body?.diagnostics) ? attention.body.diagnostics : [];
  if (attention.status !== 200 || !Array.isArray(attention.body?.items)) {
    throw new Error(`Attention read model was not authoritative: HTTP ${attention.status} ${attention.text}`);
  }
  const attentionLoad = page.waitForResponse(response => response.url().includes("/api/v08/environment") && response.request().method() === "GET");
  await page.locator('[data-view="attention"]').click();
  const attentionLoadResult = await attentionLoad;
  if (attentionLoadResult.status() !== 200) throw new Error(`Attention UI reload failed: HTTP ${attentionLoadResult.status()} ${await attentionLoadResult.text()}`);
  await page.locator("#attention-list").waitFor({ state: "visible" });
  const attentionItems = await page.locator(".attention-item").count();
  const projectedAttentionItems = (attention.body.items || []).reduce((count, item) => count + (Array.isArray(item.Attention || item.attention) ? (item.Attention || item.attention).length : 0), 0) + attentionDiagnostics.length;
  if (attentionItems !== projectedAttentionItems) {
    throw new Error(`Attention UI/read-model mismatch: visible=${attentionItems} projected=${projectedAttentionItems} diagnostics=${attentionDiagnostics.length}`);
  }
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
  console.log(JSON.stringify({
    surfaces, selectedWorkload: JSON.parse(selected), observationID, proposalName,
    rejectProposalName: rejectName, attentionState: projectedAttentionItems ? "DIAGNOSTICS" : "HEALTHY_EMPTY",
    capabilities: { review: canReview, approve: canApprove, reject: canApprove, apply: canApply },
    staleResourceVersion409: true, consoleErrors: 0, failedApiRequests: 0, uncaughtExceptions: 0,
  }));
  await browser.close();
})().catch(async error => {
  console.error(error.stack || error.message);
  // Closing the browser on every failure is part of the harness contract:
  // otherwise Playwright's child process keeps the parent shell alive and
  // prevents the shell-level cleanup trap from reclaiming its fixtures.
  try { await browser?.close(); } catch (_) { /* best effort after failure */ }
  process.exitCode = 1;
});
