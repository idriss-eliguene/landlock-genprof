import { describe, expect, it } from "vitest";
import { attentionReasons, dimension, displayMetric, recentEvents } from "./model";
import type { SphmReport } from "../../types";

const report: SphmReport = {
  modelVersion: "SPHM-v1",
  overall: { id: "overall", name: "Security Profile Health", state: "UNKNOWN", reason: "proof missing" },
  dimensions: [
    { id: "evidence", name: "Evidence", state: "UNKNOWN", value: 2, unit: "observations", reason: "flush proof missing" },
    { id: "coverage", name: "Coverage", state: "NOT_ESTABLISHED", reason: "no denominator" },
  ], attention: [], context: { clusterIdentity: "cluster", namespace: "payments", contextVersion: "7" },
};

describe("M8 overview projection semantics", () => {
  it("preserves UNKNOWN and NOT_ESTABLISHED instead of turning them into zero or healthy", () => {
    expect(dimension(report, "evidence")?.state).toBe("UNKNOWN");
    expect(dimension(report, "coverage")?.state).toBe("NOT_ESTABLISHED");
    expect(displayMetric(dimension(report, "evidence"))).toBe("2 observations");
    expect(displayMetric(dimension(report, "coverage"))).toBe("NOT_ESTABLISHED");
  });

  it("keeps exact Attention references for drilldown", () => {
    const reasons = attentionReasons({ items: [{ subject: { target: "Deployment/api" }, attention: [{ category: "OBSERVATION_FAILED", observationRefs: ["obs-7"] }] }] });
    expect(reasons[0].observationRefs).toEqual(["obs-7"]);
  });

  it("uses authoritative timestamp order and does not synthesize events", () => {
    const result = recentEvents({ history: { timestampedEvents: [{ kind: "OBSERVATION_COMPLETED", sourceRef: { kind: "Observation", name: "obs-7" }, temporalClass: "TIMESTAMPED", claimTier: "EMPIRICALLY_OBSERVED", detailCode: "EXECUTION_COMPLETED" }], untimestampedFacts: [], limitations: [], totalCount: 1, truncated: false }, limitation: "BEST_EFFORT_MULTI_OBJECT_READ" });
    expect(result).toHaveLength(1);
    expect(result[0].sourceRef.name).toBe("obs-7");
  });

  it("keeps untimestamped facts out of recent chronological activity", () => {
    const result = recentEvents({ history: { timestampedEvents: [], untimestampedFacts: [{ kind: "HISTORY_FACT", sourceRef: { kind: "Observation", name: "obs-8" }, temporalClass: "UNTIMESTAMPED_UNORDERED", claimTier: "BOOKKEEPING", detailCode: "CONTRIBUTION_MEMBERSHIP" }], limitations: ["CONTRIBUTION_TIME_NOT_RECORDED"], totalCount: 1, truncated: false }, limitation: "BEST_EFFORT_MULTI_OBJECT_READ" });
    expect(result).toEqual([]);
  });
});
