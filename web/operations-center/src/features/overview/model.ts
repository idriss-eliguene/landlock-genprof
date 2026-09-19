import type { AttentionReason, EnvironmentProjectionResponse, HistoryEvent, HistoryResponse, SphmDimension, SphmReport } from "../../types";

export function dimension(report: SphmReport | undefined, id: string): SphmDimension | undefined {
  return report?.dimensions.find(item => item.id === id);
}

export function attentionReasons(projection: EnvironmentProjectionResponse | undefined): AttentionReason[] {
  return (projection?.items || []).flatMap(item => item.attention || []);
}

export function recentEvents(response: HistoryResponse | undefined, limit = 6): HistoryEvent[] {
  return (response?.history.timestampedEvents || []).slice(0, limit);
}

export function displayMetric(item: SphmDimension | undefined): string {
  if (!item) return "NOT_ESTABLISHED";
  if (typeof item.value === "number") return `${item.value}${item.unit ? ` ${item.unit}` : ""}`;
  return item.state;
}
