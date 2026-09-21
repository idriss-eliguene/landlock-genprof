import { describe, expect, it } from "vitest";
import { attentionIdentity, dimensionValue, isProofState, nonHealthyDimensions, SPHM_DIMENSION_IDS } from "./model";
import type { SphmReport } from "../../types";

const report: SphmReport = {
  modelVersion: "SPHM-v1",
  overall: { id: "overall", name: "Security Profile Health", state: "UNKNOWN", reason: "proof missing" },
  dimensions: SPHM_DIMENSION_IDS.map(id => ({ id, name: id, state: id === "evidence" ? "UNKNOWN" : id === "coverage" ? "NOT_ESTABLISHED" : "HEALTHY", reason: id })),
  attention: [{ id: "evidence-obs-1", kind: "EVIDENCE_UNKNOWN", title: "Evidence", reason: "flush proof missing", observationID: "obs-1" }],
  context: { clusterIdentity: "cluster", namespace: "payments", contextVersion: "7" },
};

describe("M9 SPHM presentation semantics", () => {
  it("preserves every canonical server state as a distinct value", () => {
    const states = ["HEALTHY", "ATTENTION", "CRITICAL", "UNKNOWN", "NOT_ESTABLISHED", "NOT_APPLICABLE"];
    expect(states.map(state => ({ state, proof: isProofState(state) }))).toEqual([
      { state: "HEALTHY", proof: true }, { state: "ATTENTION", proof: true }, { state: "CRITICAL", proof: true },
      { state: "UNKNOWN", proof: true }, { state: "NOT_ESTABLISHED", proof: false }, { state: "NOT_APPLICABLE", proof: false },
    ]);
  });

  it("renders every canonical dimension without collapsing missing proof", () => {
    expect(report.dimensions.map(item => item.id)).toEqual([...SPHM_DIMENSION_IDS]);
    expect(nonHealthyDimensions(report).map(item => item.state)).toEqual(["NOT_ESTABLISHED", "UNKNOWN"]);
    expect(dimensionValue(report.dimensions[1])).toBe("NOT_ESTABLISHED");
    expect(dimensionValue(report.dimensions[2])).toBe("UNKNOWN");
  });

  it("keeps exact attention identity for drilldown", () => {
    expect(attentionIdentity(report.attention[0])).toBe("obs-1");
  });

  it("distinguishes evaluated proof states from unavailable model states", () => {
    expect(isProofState("UNKNOWN")).toBe(true);
    expect(isProofState("NOT_ESTABLISHED")).toBe(false);
    expect(isProofState("NOT_APPLICABLE")).toBe(false);
  });
});
