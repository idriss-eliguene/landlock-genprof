import { describe, expect, it } from "vitest";
import { normalizeEnvironmentProjection, normalizeHistory } from "./client";

describe("M7 authoritative projection nullability", () => {
  it("normalizes the Go projection's exported-field JSON without inventing identity", () => {
    const result = normalizeHistory({
      History: {
        TimestampedEvents: [{ Kind: "OBSERVATION_COMPLETED", SourceRef: { Kind: "Observation", Namespace: "payments", Name: "obs-1", UID: "uid-1" }, Timestamp: "2026-01-01T00:00:00Z", TemporalClass: "TIMESTAMPED", ClaimTier: "EMPIRICALLY_OBSERVED", DetailCode: "EXECUTION_COMPLETED" }],
        UntimestampedFacts: [{ Kind: "CONTRIBUTION_PRESENT", SourceRef: { Kind: "TrainingHistory", Namespace: "payments", Name: "history" }, TemporalClass: "UNTIMESTAMPED_UNORDERED", ClaimTier: "BOOKKEEPING", DetailCode: "CONTRIBUTION_MEMBERSHIP" }],
        Limitations: ["CONTRIBUTION_TIME_NOT_RECORDED"], TotalCount: 2, Truncated: false,
      }, Limitation: "BEST_EFFORT_MULTI_OBJECT_READ",
    } as never);
    expect(result.history.timestampedEvents[0].sourceRef.uid).toBe("uid-1");
    expect(result.history.untimestampedFacts).toHaveLength(1);
    expect(result.history.limitations).toEqual(["CONTRIBUTION_TIME_NOT_RECORDED"]);
  });

  it("preserves an empty nullable Attention collection as an authoritative empty projection", () => {
    const result = normalizeEnvironmentProjection({ Items: [{ Subject: { Scope: "CONTAINER", Target: "Deployment/api", Container: "api", ImageIdentity: null }, Attention: null }], TotalCount: 1 } as never);
    expect(result.items[0].subject.imageIdentity).toBeUndefined();
    expect(result.items[0].attention).toEqual([]);
  });
});
