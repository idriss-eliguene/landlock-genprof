const stateLabel: Record<string, string> = { REQUESTED: "Requested", STARTING: "Starting", RUNNING: "Observing", COMPLETING: "Finalizing", COMPLETED: "Completed", FAILED: "Failed" };
const evidenceLabel: Record<string, string> = { AVAILABLE: "Available", UNKNOWN: "Unknown", EMPTY: "No attributable evidence", NOT_ESTABLISHED: "Not established", NOT_APPLICABLE: "Not applicable" };

export function Status({ value, label }: { value?: string; label?: string }) {
  const normalized = value || "UNKNOWN";
  return <span className={`status-pill status-${normalized.toLowerCase()}`}><span aria-hidden="true">{normalized === "FAILED" ? "×" : normalized === "COMPLETED" || normalized === "AVAILABLE" ? "✓" : "•"}</span> {label || stateLabel[normalized] || evidenceLabel[normalized] || normalized}</span>;
}
