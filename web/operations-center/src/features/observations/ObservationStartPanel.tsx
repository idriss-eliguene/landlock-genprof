import { useState } from "react";
import type { WorkloadSelection } from "../../types";

const sourceOptions = [
  ["filesystem", "Filesystem"],
  ["capabilities", "Capabilities"],
  ["exec", "Executed binaries"],
  ["networkConnect", "Network connections"],
  ["networkBind", "Network binds"],
] as const;

export type ObservationPermission = "checking" | "allowed" | "denied" | "unavailable";
export type ObservationHistoryState = "checking" | "complete" | "partial" | "error";

export function observationStartBlockReason(input: {
  permission: ObservationPermission;
  history: ObservationHistoryState;
  selectionCurrent: boolean;
  hasIdentity: boolean;
  hasSources: boolean;
  active: boolean;
}): string {
  if (input.permission === "checking") return "Checking observation.operate permission…";
  if (input.permission === "unavailable") return "Observation permissions could not be checked. Refresh the environment context before starting.";
  if (input.permission === "denied") return "This identity lacks observation.operate in the selected namespace.";
  if (input.history === "checking") return "Loading existing Observation history before enabling a new capture…";
  if (input.history === "error") return "Existing Observation history could not be checked. Refresh it before starting another capture.";
  if (input.history === "partial") return "Observation history is incomplete. Load all remaining pages before starting another capture.";
  if (!input.hasIdentity) return "The selected workload and Pod need verified UIDs before observation can start.";
  if (!input.selectionCurrent) return "The selected Pod changed or disappeared. Refresh the workload inventory and select it again.";
  if (!input.hasSources) return "Select at least one evidence source.";
  if (input.active) return "An Observation for this workload/container is already active.";
  return "";
}

export function ObservationStartPanel(props: {
  selection: WorkloadSelection;
  namespace: string;
  permission: ObservationPermission;
  history: ObservationHistoryState;
  selectionCurrent: boolean;
  active: boolean;
  pending: boolean;
  onStart: (sources: string[], durationSeconds: number) => void;
}) {
  const [sources, setSources] = useState<string[]>(["filesystem", "capabilities"]);
  const [durationSeconds, setDurationSeconds] = useState(60);
  const reason = observationStartBlockReason({
    permission: props.permission,
    history: props.history,
    selectionCurrent: props.selectionCurrent,
    hasIdentity: Boolean(props.selection.workloadUID && props.selection.podUID),
    hasSources: sources.length > 0,
    active: props.active,
  });
  const disabled = Boolean(reason) || props.pending;

  return <section className="detail-panel observation-scope" data-testid="observation-scope">
    <div className="section-heading"><div><span className="eyebrow">Capture prerequisites</span><h3>Observation scope</h3><p>Capture only starts after the request is authorized and the executor confirms source attachment.</p></div></div>
    <dl className="observation-scope-identity">
      <dt>Namespace</dt><dd><code>{props.namespace || "NOT_BOUND"}</code></dd>
      <dt>Workload</dt><dd>{props.selection.kind} / {props.selection.name} · {props.selection.container}</dd>
      <dt>Anchor Pod</dt><dd>{props.selection.pod || "UNKNOWN"} · UID <code>{props.selection.podUID || "UNKNOWN"}</code></dd>
      <dt>Workload UID</dt><dd><code>{props.selection.workloadUID || "UNKNOWN"}</code></dd>
      <dt>Image identity</dt><dd><code>{props.selection.imageIdentity || "UNKNOWN — immutable runtime digest unavailable"}</code></dd>
      <dt>Coverage</dt><dd>Selected workload/container; executor records the concrete running Pod identities it resolves during the observation.</dd>
    </dl>
    <fieldset className="observation-source-options">
      <legend>Evidence sources</legend>
      {sourceOptions.map(([value, label]) => <label key={value}><input type="checkbox" value={value} checked={sources.includes(value)} onChange={event => setSources(current => event.target.checked ? [...current, value] : current.filter(source => source !== value))} />{label}</label>)}
    </fieldset>
    <label className="form-field observation-duration">Duration
      <select name="durationSeconds" aria-label="Observation duration" value={durationSeconds} onChange={event => setDurationSeconds(Number(event.target.value))}>
        <option value={30}>30 seconds</option><option value={60}>1 minute</option><option value={120}>2 minutes</option><option value={300}>5 minutes</option>
      </select>
    </label>
    <div className="observation-prerequisites" aria-live="polite">
      <span>Namespace bound: {props.namespace ? "ready" : "required"}</span>
      <span>Request permission: {props.permission === "allowed" ? "allowed" : props.permission === "denied" ? "denied" : props.permission}</span>
      <span>Existing captures: {props.history}</span>
      <span>Selection: {props.selectionCurrent ? "current" : "stale or unavailable"}</span>
      <span>Capture backend: verified by executor during STARTING</span>
    </div>
    {reason ? <p className="notice error" role="status" data-testid="observation-start-blocked">{reason}</p> : null}
    <button type="button" className="primary-button" data-testid="start-observation" disabled={disabled} onClick={() => props.onStart(sources, durationSeconds)}>{props.pending ? "Starting…" : "Start observation"}</button>
  </section>;
}
