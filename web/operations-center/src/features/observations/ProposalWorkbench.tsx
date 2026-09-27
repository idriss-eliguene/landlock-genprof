import type { ObservationRead, WorkloadSelection } from "../../types";

export type ProposalWorkbenchPermission = "checking" | "allowed" | "denied" | "unavailable";

export function proposalGenerationBlockReason(input: {
  observation: ObservationRead;
  namespace: string;
  selection?: WorkloadSelection | null;
  selectionCurrent: boolean;
  permission: ProposalWorkbenchPermission;
}): string {
  const { observation, selection } = input;
  if (observation.execution.state !== "COMPLETED" || !observation.frozen) return "Wait until this Observation is completed and frozen before generating a Proposal.";
  if (!selection) return "Select and resolve the current workload and concrete Pod before continuing.";
  if (!input.selectionCurrent) return "The workload inventory is stale or the selected Pod was replaced. Refresh and select the current Pod again.";
  if (!selection.pod || !selection.podUID || !selection.workloadUID || !selection.imageIdentity) return "The current Pod, workload UID, and immutable image digest must all be known.";
  const identity = observation.identity;
  if (identity.namespace !== input.namespace || identity.group !== selection.group || identity.kind !== selection.kind || identity.workloadName !== selection.name || identity.workloadUID !== selection.workloadUID || identity.container !== selection.container) return "This Observation belongs to a different namespace, workload UID, or container. It cannot be used for the selected workload.";
  if (!identity.imageIdentity || identity.imageIdentity !== selection.imageIdentity) return "The Observation image digest does not match the current workload image. Select a current Observation instead.";
  if (observation.targetChanges?.some(change => change.kind === "IMAGE_CHANGED" || change.kind === "WORKLOAD_REVISION_CHANGED")) return "The workload revision or image changed during capture. This mixed-revision Observation cannot generate a Proposal.";
  if (input.permission === "checking") return "Checking observation.operate and proposal.generate permissions…";
  if (input.permission === "unavailable") return "Generation permissions could not be checked. Refresh the Operational Context.";
  if (input.permission === "denied") return "This identity lacks observation.operate or proposal.generate in the selected namespace.";
  return "";
}

export function ProposalWorkbench(props: {
  observation: ObservationRead;
  namespace: string;
  selection?: WorkloadSelection | null;
  selectionCurrent: boolean;
  permission: ProposalWorkbenchPermission;
  pending: boolean;
  error?: string;
  onInspectEvidence: () => void;
  onGenerate: () => void;
}) {
  const { observation } = props;
  const reason = proposalGenerationBlockReason(props);
  const sources = observation.sources;
  const excluded = sources.reduce((sum, source) => sum + source.excludedCount, 0);
  const unknown = sources.filter(source => source.evidenceState === "UNKNOWN").length;
  const canGenerate = !reason && !props.pending;
  const next = observation.execution.state !== "COMPLETED" || !observation.frozen
    ? "Wait for authoritative Observation completion."
    : reason
      ? reason
      : "Inspect the evidence, then generate a draft Proposal for human review.";

  return <section className="detail-panel proposal-workbench" data-testid="proposal-workbench">
    <div className="section-heading"><div><span className="eyebrow">Guided workflow · no automatic approval</span><h3>Proposal Workbench</h3><p>Review the recorded evidence, derive a draft candidate, then use the existing human governance flow.</p></div></div>
    <ol className="proposal-workbench-steps">
      <li><strong>Observation</strong><span>{observation.execution.state || "UNKNOWN"} · {observation.frozen ? "frozen" : "not frozen"}</span></li>
      <li><strong>Evidence</strong><span>{sources.length} source(s) · {unknown ? `${unknown} source(s) UNKNOWN` : "source states available"} · {excluded} excluded event(s)</span></li>
      <li><strong>Target binding</strong><span>{observation.identity.kind}/{observation.identity.workloadName} · UID {observation.identity.workloadUID || "UNKNOWN"} · image {observation.identity.imageIdentity || "UNKNOWN"}</span></li>
      <li><strong>Human review</strong><span>Generation creates a draft only. Review and approval remain separate authorized actions.</span></li>
    </ol>
    {unknown || excluded ? <p className="notice" data-testid="proposal-workbench-coverage">Evidence is incomplete or has exclusions. Inspect source qualification and reasons; generation does not turn UNKNOWN or excluded evidence into a positive fact.</p> : null}
    <p className="muted">Candidate derivation uses the existing backend contract. Incomplete evidence is retained as qualification metadata; it is not interpreted as proof of absence.</p>
    {reason ? <p className="notice error" role="status" data-testid="proposal-generation-blocked">{reason}</p> : null}
    {props.error ? <p className="notice error" role="alert" data-testid="proposal-generation-error">Proposal generation failed: {props.error}</p> : null}
    <p className="proposal-workbench-next"><strong>Next action:</strong> {props.pending ? "Generating a draft Proposal…" : next}</p>
    <div className="action-row"><button type="button" className="secondary-button" onClick={props.onInspectEvidence}>Inspect evidence</button><button type="button" className="primary-button" data-testid="generate-proposal" disabled={!canGenerate} onClick={props.onGenerate}>{props.pending ? "Generating…" : "Generate draft Proposal"}</button></div>
  </section>;
}
