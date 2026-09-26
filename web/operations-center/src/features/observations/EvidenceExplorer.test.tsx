import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { ObservationRead, ProposalRead } from "../../types";
import { EvidenceExplorer, evidenceRows } from "./EvidenceExplorer";

const historical: ObservationRead = {
  observationID: "old-observation",
  identity: { clusterIdentity: "cluster", namespace: "team-a", group: "apps", kind: "Deployment", workloadName: "api", workloadUID: "", container: "server" },
  execution: { state: "COMPLETED" }, sources: [{ name: "capabilities", attributionState: "COMPLETED", evidenceState: "UNKNOWN", attributedCount: 1, excludedCount: 1, backendHealthConfirmed: true, sourceAttachedForBoundWindow: true, flushConfirmed: true, facts: { Capabilities: [{ Name: "CAP_NET_RAW" }] } }], frozen: true, stopEligible: false,
};

const proposal: ProposalRead = {
  name: "api-candidate", uid: "proposal-uid", candidateVersion: "candidate-v2", status: {},
  artifact: { containerCapabilities: { add: ["CAP_NET_RAW"] } },
  provenance: { observationIDs: ["old-observation"], capabilityAttribution: [{ capability: "CAP_NET_RAW", state: "UNKNOWN", observationIDs: ["old-observation"] }] },
};

describe("Evidence Explorer", () => {
  it("browses source-tagged normalized facts without inventing event records", () => {
    expect(evidenceRows(historical)).toEqual([{ source: "capabilities", label: "Capability: CAP_NET_RAW" }]);
    const html = renderToStaticMarkup(createElement(EvidenceExplorer, {
      observations: [historical], selectedID: historical.observationID, selected: historical, loading: false,
      proposals: [], proposalsLoading: false, onSelectObservation: vi.fn(), onOpenProposal: vi.fn(),
    }));
    expect(html).toContain("normalized facts");
    expect(html).toContain("not a retained raw event stream");
    expect(html).toContain("UNKNOWN — historical record");
    expect(html).toContain("Per-event exclusion reasons were not retained");
    expect(html).toContain("no resolved Pod/container identity was persisted");
  });

  it("shows persisted exclusion reason counts and capability provenance links, preserving UNKNOWN", () => {
    const observed: ObservationRead = { ...historical, sources: [{ ...historical.sources[0]!, exclusionReasons: [{ reason: "runtime identity unavailable", count: 1 }] }] };
    const html = renderToStaticMarkup(createElement(EvidenceExplorer, {
      observations: [observed], selectedID: observed.observationID, selected: observed, loading: false,
      proposals: [proposal], proposalsLoading: false, onSelectObservation: vi.fn(), onOpenProposal: vi.fn(),
    }));
    expect(html).toContain("runtime identity unavailable");
    expect(html).toContain("UNKNOWN · this Observation is recorded as a match");
    expect(html).toContain("Open associated Proposal");
    expect(html).toContain("not proof of causality");
  });
});
