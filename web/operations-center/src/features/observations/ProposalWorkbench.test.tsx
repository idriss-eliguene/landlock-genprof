import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { ObservationRead, WorkloadSelection } from "../../types";
import { ProposalWorkbench, proposalGenerationBlockReason } from "./ProposalWorkbench";

const observation: ObservationRead = {
  observationID: "obs-1",
  identity: { clusterIdentity: "cluster", namespace: "team-a", group: "apps", kind: "Deployment", workloadName: "api", workloadUID: "workload-1", container: "server", imageIdentity: "sha256:abcd" },
  spec: { anchorPodUID: "pod-1" }, execution: { state: "COMPLETED" }, frozen: true, stopEligible: false,
  sources: [{ name: "capabilities", evidenceState: "UNKNOWN", attributionState: "UNKNOWN", attributedCount: 0, excludedCount: 2, backendHealthConfirmed: false, sourceAttachedForBoundWindow: true, flushConfirmed: false }],
};
const selection: WorkloadSelection = { group: "apps", kind: "Deployment", name: "api", container: "server", pod: "api-1", podUID: "pod-1", workloadUID: "workload-1", imageIdentity: "sha256:abcd" };
const guardInput = { observation, namespace: "team-a", selection, selectionCurrent: true, permission: "allowed" as const };

describe("Proposal Workbench", () => {
  it("allows the existing generator only for a current, exact, immutable target", () => {
    expect(proposalGenerationBlockReason(guardInput)).toBe("");
  });

  it("blocks replacement workloads, changed images, and denied authorization", () => {
    expect(proposalGenerationBlockReason({ ...guardInput, selection: { ...selection, workloadUID: "replacement" } })).toContain("different namespace, workload UID");
    expect(proposalGenerationBlockReason({ ...guardInput, selection: { ...selection, imageIdentity: "sha256:efgh" } })).toContain("image digest");
    expect(proposalGenerationBlockReason({ ...guardInput, selectionCurrent: false })).toContain("Pod was replaced");
    expect(proposalGenerationBlockReason({ ...guardInput, permission: "denied" })).toContain("lacks observation.operate");
  });

  it("blocks a replacement Pod from reusing the earlier Observation", () => {
    expect(proposalGenerationBlockReason({ ...guardInput, selection: { ...selection, pod: "api-2", podUID: "pod-2" } })).toContain("original anchor does not match");
    expect(proposalGenerationBlockReason({ ...guardInput, observation: { ...observation, spec: undefined } })).toContain("original anchor does not match");
  });

  it("keeps historical identity and incomplete evidence explicit", () => {
    const historical = { ...observation, identity: { ...observation.identity, workloadUID: "", imageIdentity: undefined } };
    expect(proposalGenerationBlockReason({ ...guardInput, observation: historical })).toContain("different namespace, workload UID");
    const html = renderToStaticMarkup(createElement(ProposalWorkbench, { ...guardInput, observation, pending: false, onInspectEvidence: vi.fn(), onGenerate: vi.fn() }));
    expect(html).toContain("Evidence is incomplete or has exclusions");
    expect(html).toContain("Generation creates a draft only");
    expect(html).toContain("Inspect evidence");
  });

  it("requires completed frozen results before enabling generation", () => {
    expect(proposalGenerationBlockReason({ ...guardInput, observation: { ...observation, frozen: false } })).toContain("completed and frozen");
  });

  it("shows denied permission and backend generation failures without hiding evidence context", () => {
    const denied = renderToStaticMarkup(createElement(ProposalWorkbench, { ...guardInput, permission: "denied", pending: false, error: "NO_CANDIDATE: no attributable capabilities", onInspectEvidence: vi.fn(), onGenerate: vi.fn() }));
    expect(denied).toContain("lacks observation.operate or proposal.generate");
    expect(denied).toContain("Proposal generation failed");
    expect(denied).toContain("no attributable capabilities");
    expect(denied).toContain("data-testid=\"generate-proposal\" disabled=\"\"");
  });
});
