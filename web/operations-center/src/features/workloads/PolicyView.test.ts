import { describe, expect, it } from "vitest";
import { lineageCompletenessLabel, lineageDiagnosticTotal } from "./PolicyView";

describe("authoritative workload Policy presentation", () => {
  it("keeps incomplete lineage visibly distinct from a complete result", () => {
    expect(lineageCompletenessLabel(true)).toBe("Complete lineage");
    expect(lineageCompletenessLabel(false)).toBe("Partial lineage");
  });

  it("counts server-owned exclusion diagnostics without treating them as proposals", () => {
    expect(lineageDiagnosticTotal({ excludedMalformed: 1, excludedInsufficientProvenance: 2, excludedMixedIdentity: 3, excludedNotAssociated: 4 })).toBe(10);
    expect(lineageDiagnosticTotal(undefined)).toBe(0);
  });
});
