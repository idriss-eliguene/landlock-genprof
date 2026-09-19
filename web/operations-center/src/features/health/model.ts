import type { SphmAttention, SphmDimension, SphmReport } from "../../types";

export const SPHM_DIMENSION_IDS = ["authority", "coverage", "evidence", "freshness", "drift", "governance", "pipeline", "enforcement"] as const;

export function nonHealthyDimensions(report: SphmReport | undefined): SphmDimension[] {
  return (report?.dimensions || []).filter(item => item.state !== "HEALTHY");
}

export function dimensionValue(item: SphmDimension): string {
  if (typeof item.value === "number") return `${item.value}${item.unit ? ` ${item.unit}` : ""}`;
  return item.state;
}

export function attentionIdentity(item: SphmAttention): string {
  return item.observationID || item.proposalName || item.workload || item.id;
}

export function isProofState(state: string): boolean {
  return state === "HEALTHY" || state === "ATTENTION" || state === "CRITICAL" || state === "UNKNOWN";
}
