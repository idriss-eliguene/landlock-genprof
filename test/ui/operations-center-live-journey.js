const { chromium } = require("playwright");
const { bindNamespace, openContextControls } = require("./namespace-binding");
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const operatorURL = process.env.OC_LIVE_OPERATOR_URL;
const reviewerURL = process.env.OC_LIVE_REVIEWER_URL;
const approverURL = process.env.OC_LIVE_APPROVER_URL;
const deniedURL = process.env.OC_LIVE_DENIED_URL;
const namespace = process.env.OC_LIVE_NAMESPACE;
const privateNamespace = process.env.OC_LIVE_PRIVATE_NAMESPACE;
const contextName = process.env.OC_LIVE_CONTEXT_NAME || "default";
const artifacts = process.env.OC_LIVE_ARTIFACTS;
const workloadName = process.env.OC_LIVE_EXPECTED_WORKLOAD || "live-probe";
const stages = [];
const liveContexts = [];
const livePages = [];
let browser;

function record(stage, details = {}) {
  const item = { stage, ...details, at: new Date().toISOString() };
  stages.push(item);
  console.log(JSON.stringify(item));
}
function assert(value, message) { if (!value) throw new Error(message); }
async function bind(page, url, target = url) {
  await page.goto(target, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible", timeout: 120000 });
  await openContextControls(page);
  await bindNamespace(page, namespace, contextName);
}
async function shot(page, name) {
  await page.screenshot({ path: path.join(artifacts, name), fullPage: true });
}
async function startStop(page, label) {
  const startResponse = page.waitForResponse(r => new URL(r.url()).pathname === "/api/observations/start" && r.request().method() === "POST");
  await page.getByTestId("start-observation").click();
  const started = await startResponse;
  assert(started.status() === 200, label + " start HTTP " + started.status());
  const id = (await started.json()).observationID;
  assert(id, "Observation ID missing");
  await page.waitForFunction(() => /Observing/.test(document.querySelector('[data-testid="observation-detail"] .detail-heading .status-pill')?.textContent || ""), undefined, { timeout: 120000 });
  await page.waitForTimeout(12000);
  const stopResponse = page.waitForResponse(r => new URL(r.url()).pathname === "/api/observations/stop" && r.request().method() === "POST");
  await page.getByTestId("stop-observation").click();
  const stopped = await stopResponse;
  assert(stopped.status() === 200, label + " stop HTTP " + stopped.status());
  await page.waitForFunction(() => {
    const detail = document.querySelector('[data-testid="observation-detail"]');
    return detail && /Completed/.test(detail.querySelector(".detail-heading .status-pill")?.textContent || "") && /Frozen\s+Yes/i.test(detail.innerText || "");
  }, undefined, { timeout: 120000 });
  const facts = await page.locator('[data-testid="evidence-summary"] .fact-list li').count();
  assert(facts > 0, label + " completed with no normalized persisted facts");
  return { id, facts };
}
async function contextHeaders(page) {
  const values = await page.locator(".context-meta code").allTextContents();
  assert(values.length >= 4 && values[2] && values[2] !== "NOT_BOUND" && values[3] !== "NOT_BOUND", "environment binding unavailable");
  return { "X-Environment-Session": values[2], "X-Environment-Context-Version": values[3], "X-Environment-Namespace": namespace };
}
async function getCurrentPod() {
  for (let i = 0; i < 60; i++) {
    const raw = execFileSync("kubectl", ["-n", namespace, "get", "pods", "-l", "app=live-probe", "-o", "json"], { encoding: "utf8" });
    const pods = JSON.parse(raw).items.filter(p => p.status.phase === "Running" && p.status.containerStatuses?.[0]?.ready);
    if (pods.length) return pods[0];
    await new Promise(resolve => setTimeout(resolve, 2000));
  }
  throw new Error("replacement workload Pod did not become Ready");
}
async function openProposal(url, name) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  liveContexts.push(context);
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true });
  const page = await context.newPage();
  livePages.push(page);
  await bind(page, url, url + "?proposal=" + encodeURIComponent(name));
  await page.getByRole("navigation").getByRole("button", { name: "Proposals & Governance", exact: true }).click();
  await page.getByTestId("proposal-detail").waitFor({ state: "visible", timeout: 60000 });
  return { context, page };
}

(async () => {
  fs.mkdirSync(artifacts, { recursive: true });
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  liveContexts.push(context);
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true });
  const page = await context.newPage();
  livePages.push(page);
  const errors = [];
  page.on("pageerror", e => errors.push(e.message));

  await bind(page, operatorURL);
  const workload = page.getByTestId("workload-row").filter({ hasText: workloadName }).first();
  await workload.waitFor({ state: "visible", timeout: 120000 });
  await workload.getByRole("button", { name: /Open (observations|dossier)/ }).click();
  await page.getByTestId("observations-heading").waitFor({ state: "visible" });
  await page.locator('select[name="durationSeconds"]').selectOption("30");

  const staleObservation = await startStop(page, "stale-identity");
  await shot(page, "01-observation.png");
  record("live-observation-and-persisted-evidence", { observationID: staleObservation.id, normalizedFacts: staleObservation.facts });
  await page.getByRole("navigation").getByRole("button", { name: /^Evidence$/ }).click();
  await page.getByTestId("evidence-explorer").waitFor({ state: "visible" });
  await page.getByTestId("evidence-observation-select").selectOption(staleObservation.id);
  const explorer = await page.getByTestId("evidence-explorer").innerText();
  assert(explorer.includes(staleObservation.id) && explorer.includes("Normalized observed facts"), "exact evidence was not shown in Explorer");
  await shot(page, "02-evidence-explorer.png");
  record("live-evidence-explorer", { observationID: staleObservation.id, normalizedFacts: staleObservation.facts });

  const oldPodUID = execFileSync("kubectl", ["-n", namespace, "get", "observation", staleObservation.id, "-o", "jsonpath={.spec.anchorPodUID}"], { encoding: "utf8" });
  const oldPod = await getCurrentPod();
  assert(oldPod.metadata.uid === oldPodUID, "Observation anchor is not the selected workload Pod");
  execFileSync("kubectl", ["-n", namespace, "delete", "pod", oldPod.metadata.name, "--wait=true", "--timeout=90s"], { stdio: "ignore" });
  const replacement = await getCurrentPod();
  assert(replacement.metadata.uid !== oldPodUID, "controller did not replace the Pod");
  const workloadUID = execFileSync("kubectl", ["-n", namespace, "get", "deployment", workloadName, "-o", "jsonpath={.metadata.uid}"], { encoding: "utf8" });
  const imageID = replacement.status.containerStatuses[0].imageID;
  const imageDigest = imageID.match(/sha256:[a-f0-9]{64}/)?.[0] || "";
  assert(imageDigest, "replacement image digest missing");
  const stale = await page.evaluate(async input => {
    const response = await fetch("/api/observations/generate-proposal", {
      method: "POST",
      headers: { "Content-Type": "application/json", ...input.headers },
      body: JSON.stringify({
        namespace: input.namespace, observationID: input.id, proposalName: "stale-pod-must-not-generate",
        pod: input.pod, container: "live-probe",
        expectedTarget: { group: "apps", kind: "Deployment", name: "live-probe", workloadUID: input.workloadUID, podUID: input.podUID, imageDigest: input.imageDigest },
      }),
    });
    return { status: response.status, body: await response.text() };
  }, { headers: await contextHeaders(page), namespace, id: staleObservation.id, pod: replacement.metadata.name, workloadUID, podUID: replacement.metadata.uid, imageDigest });
  assert(stale.status === 409 && /stale target identity/i.test(stale.body), "stale Pod was not rejected: " + stale.status + " " + stale.body);
  record("negative-replacement-pod-rejected", { http: stale.status, originalPodUID: oldPodUID, replacementPodUID: replacement.metadata.uid });

  await page.goto(operatorURL, { waitUntil: "domcontentloaded" });
  await page.getByTestId("migration-app").waitFor({ state: "visible" });
  await bindNamespace(page, namespace, contextName);
  await page.getByTestId("workload-row").filter({ hasText: workloadName }).first().getByRole("button", { name: /Open (observations|dossier)/ }).click();
  await page.getByTestId("observations-heading").waitFor({ state: "visible" });
  await page.locator('select[name="durationSeconds"]').selectOption("30");
  const currentObservation = await startStop(page, "current-identity");
  record("live-observation-current-target", { observationID: currentObservation.id, normalizedFacts: currentObservation.facts });
  const workbench = page.getByTestId("proposal-workbench");
  await workbench.waitFor({ state: "visible" });
  const wbText = await workbench.innerText();
  assert(wbText.includes("Candidate derivation") && wbText.includes("human review"), "guided Workbench guidance missing");
  if (/UNKNOWN|excluded event/i.test(wbText)) record("live-coverage-explicitly-qualified", { disclosureVisible: true });
  await shot(page, "03-proposal-workbench.png");
  const genResponse = page.waitForResponse(r => new URL(r.url()).pathname === "/api/observations/generate-proposal" && r.request().method() === "POST");
  await workbench.getByTestId("generate-proposal").click();
  const gen = await genResponse;
  const genBody = await gen.json();
  assert(gen.status() === 200 && genBody.proposalName, "live proposal generation failed: " + gen.status() + " " + JSON.stringify(genBody));
  const proposalName = genBody.proposalName;
  await page.getByTestId("proposal-detail").waitFor({ state: "visible", timeout: 60000 });
  const draftText = await page.getByTestId("proposal-detail").innerText();
  assert(draftText.includes(proposalName) && /CandidateDigestV2/i.test(draftText) && /sha256:[a-f0-9]{64}/i.test(draftText), "draft identity/digest missing");
  assert(/CAP_NET_RAW/.test(draftText), "real capability behavior was not attributed into the generated candidate");
  await shot(page, "04-proposal-draft.png");
  record("live-proposal-generated", { proposalName, observationID: currentObservation.id, candidateDigestVisible: true });

  const denied = await browser.newContext();
  liveContexts.push(denied);
  const deniedPage = await denied.newPage();
  livePages.push(deniedPage);
  await bind(deniedPage, deniedURL);
  const deniedHeaders = await contextHeaders(deniedPage);
  const deniedWorkloads = await deniedPage.evaluate(async headers => {
    const response = await fetch("/api/workloads", { headers });
    return { status: response.status };
  }, deniedHeaders);
  assert(deniedWorkloads.status >= 400, "unauthorized workload read unexpectedly succeeded");
  record("negative-unauthorized-workload-read-denied", { http: deniedWorkloads.status });
  const deniedMutation = await deniedPage.evaluate(async input => {
    const response = await fetch("/api/observations/generate-proposal", {
      method: "POST",
      headers: { "Content-Type": "application/json", ...input.headers },
      body: JSON.stringify({ namespace: input.namespace, observationID: input.id, proposalName: "unauthorized-must-not-generate", pod: input.pod, container: "live-probe", expectedTarget: input.target }),
    });
    return { status: response.status };
  }, { headers: deniedHeaders, namespace, id: staleObservation.id, pod: replacement.metadata.name, target: { group: "apps", kind: "Deployment", name: workloadName, workloadUID, podUID: replacement.metadata.uid, imageDigest } });
  assert(deniedMutation.status === 403, "unauthorized proposal generation HTTP " + deniedMutation.status + ", expected 403");
  record("negative-unauthorized-generation-denied", { http: deniedMutation.status });
  await denied.close();

  const bound = await contextHeaders(page);
  const cross = await page.evaluate(async input => {
    const response = await fetch("/api/v09/environments/" + encodeURIComponent(input.session) + "/capabilities?namespace=" + encodeURIComponent(input.other), { headers: input.headers });
    return { status: response.status };
  }, { session: bound["X-Environment-Session"], other: privateNamespace, headers: { ...bound, "X-Environment-Namespace": namespace } });
  assert(cross.status >= 400, "cross-namespace bind unexpectedly succeeded");
  record("negative-cross-namespace-denied", { http: cross.status });

  const reviewer = await openProposal(reviewerURL, proposalName);
  const reviewResponse = reviewer.page.waitForResponse(r => r.url().includes("/api/governance/proposals/" + proposalName + "/review") && r.request().method() === "POST");
  await reviewer.page.getByRole("button", { name: "Review", exact: true }).click();
  const reviewed = await reviewResponse;
  assert(reviewed.status() === 200, "review HTTP " + reviewed.status());
  await reviewer.page.waitForFunction(() => /State:\s*Reviewed/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || ""), undefined, { timeout: 30000 });
  await shot(reviewer.page, "05-proposal-reviewed.png");
  record("live-human-review", { actor: "ci-reviewer", proposalName, http: reviewed.status() });
  await reviewer.context.tracing.stop({ path: path.join(artifacts, "reviewer-trace.zip") });
  await reviewer.context.close();

  const approver = await openProposal(approverURL, proposalName);
  const approveResponse = approver.page.waitForResponse(r => r.url().includes("/api/governance/proposals/" + proposalName + "/approve") && r.request().method() === "POST");
  await approver.page.getByRole("button", { name: "Approve", exact: true }).click();
  const approved = await approveResponse;
  assert(approved.status() === 200, "approval HTTP " + approved.status() + ": " + await approved.text());
  await approver.page.waitForFunction(() => /State:\s*Approved/.test(document.querySelector('[data-testid="proposal-detail"]')?.textContent || ""), undefined, { timeout: 30000 });
  const approvedText = await approver.page.locator("body").innerText();
  assert(/twin-Pod experiment|Bounded behavioral evidence/i.test(approvedText), "bounded verification proof boundary was not shown");
  await shot(approver.page, "06-proposal-approved.png");
  record("live-human-approval", { actor: "ci-approver", proposalName, http: approved.status(), state: "Approved" });
  await approver.context.tracing.stop({ path: path.join(artifacts, "approver-trace.zip") });
  await approver.context.close();

  await context.tracing.stop({ path: path.join(artifacts, "operator-trace.zip") });
  assert(errors.length === 0, "browser errors: " + errors.join("; "));
  record("NOT_RUN:apply-and-runtime-proof", { reason: "candidate-v2 derives container capabilities and marks Seccomp NOT_AVAILABLE; this candidate cannot yield an approved SPO SeccompProfile" });
  fs.writeFileSync(path.join(artifacts, "qualification-results.json"), JSON.stringify({ stages, final: "PARTIAL_LIVE_SLICE", proofBoundary: "Live Gadget observation through separate human approval only; no Seccomp enforcement claim." }, null, 2));
  await browser.close();
})().catch(async error => {
  try { fs.writeFileSync(path.join(artifacts, "qualification-failure.txt"), error.stack || String(error)); } catch {}
  for (let i = 0; i < livePages.length; i++) {
    try { await livePages[i].screenshot({ path: path.join(artifacts, "failure-page-" + i + ".png"), fullPage: true }); } catch {}
  }
  for (let i = 0; i < liveContexts.length; i++) {
    try { await liveContexts[i].tracing.stop({ path: path.join(artifacts, "failure-trace-" + i + ".zip") }); } catch {}
    try { await liveContexts[i].close(); } catch {}
  }
  if (browser) await browser.close().catch(() => {});
  console.error(error.stack || error);
  process.exitCode = 1;
});
