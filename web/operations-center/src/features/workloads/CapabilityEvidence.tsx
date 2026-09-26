import type { ReactNode } from "react";

export interface CapabilityEvidenceEntry {
  capability: string;
  state: string;
  observationIDs?: string[];
}

export function CapabilityEvidence({ capabilities, attribution, onObservation }: {
  capabilities: string[];
  attribution?: CapabilityEvidenceEntry[];
  onObservation: (id: string) => void;
}) {
  if (capabilities.length === 0) return null;
  return <section className="detail-panel capability-evidence" data-testid="capability-evidence"><h3>Supporting Observations by capability</h3>{capabilities.map(capability => {
    const evidence = attribution?.find(item => item.capability === capability);
    const links: ReactNode = evidence?.observationIDs?.map(id => <button key={id} type="button" className="link-button" onClick={() => onObservation(id)}><code>{id}</code></button>);
    return <div className="capability-evidence-row" key={capability}><strong><code>{capability}</code></strong>{!evidence || evidence.state === "UNKNOWN" ? <span>Evidence attribution: UNKNOWN{links ? <> · known matching Observations: {links}</> : null}</span> : <span>Recorded in {links}</span>}</div>;
  })}<p className="muted">These links report recorded Observation facts; they do not establish causality or that a capability is safe.</p></section>;
}
