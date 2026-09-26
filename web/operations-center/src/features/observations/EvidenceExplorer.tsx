import { useMemo, useState } from "react";
import type { ObservationRead, ProposalRead } from "../../types";
import { Status } from "../../shared/Status";

type EvidenceRow = { source: string; label: string };
function field(record: Record<string, unknown>, lower: string, upper: string): unknown {
  return record[lower] ?? record[upper];
}
export function evidenceRows(observation: ObservationRead): EvidenceRow[] {
  const rows: EvidenceRow[] = [];
  for (const source of observation.sources) {
    if (!source.facts || typeof source.facts !== "object" || Array.isArray(source.facts)) continue;
    const facts = source.facts as Record<string, unknown>;
    const add = (key: string, title: string, format: (value: Record<string, unknown>) => string) => {
      const entries = field(facts, key, key[0].toUpperCase() + key.slice(1));
      if (!Array.isArray(entries)) return;
      for (const entry of entries) {
        if (!entry || typeof entry !== "object" || Array.isArray(entry)) continue;
        const label = format(entry as Record<string, unknown>);
        if (label) rows.push({ source: source.name, label: `${title}: ${label}` });
      }
    };
    add("filesystem", "Filesystem", item => {
      const raw = field(item, "permissions", "Permissions");
      const permissions = Array.isArray(raw) ? raw.map(String) : [];
      return `${String(field(item, "path", "Path") || "")} (${permissions.join(", ")})`;
    });
    add("exec", "Executed binary", item => String(field(item, "path", "Path") || ""));
    add("networkConnect", "Network connection", item => `${String(field(item, "direction", "Direction") || "unknown")} port ${String(field(item, "port", "Port") ?? "unknown")}`);
    add("networkBind", "Network bind", item => `${String(field(item, "direction", "Direction") || "unknown")} port ${String(field(item, "port", "Port") ?? "unknown")}`);
    add("capabilities", "Capability", item => String(field(item, "name", "Name") || ""));
  }
  return rows;
}

export function EvidenceExplorer(props: {
  observations: ObservationRead[];
  observationsComplete?: boolean;
  loadingMoreObservations?: boolean;
  onLoadMoreObservations?: () => void;
  selectedID?: string | null;
  selected?: ObservationRead;
  loading: boolean;
  error?: string;
  proposals: ProposalRead[];
  proposalsLoading: boolean;
  proposalsError?: string;
  proposalsComplete?: boolean;
  loadingMoreProposals?: boolean;
  onLoadMoreProposals?: () => void;
  onSelectObservation: (id: string) => void;
  onOpenProposal: (name: string, uid?: string) => void;
}) {
  const [sourceFilter, setSourceFilter] = useState("all");
  const [search, setSearch] = useState("");
  const observation = props.selected;
  const rows = useMemo(() => observation ? evidenceRows(observation) : [], [observation]);
  const sources = observation?.sources ?? [];
  const filtered = rows.filter(row => (sourceFilter === "all" || row.source === sourceFilter) && row.label.toLowerCase().includes(search.trim().toLowerCase()));
  const related = observation ? props.proposals.filter(proposal => proposal.provenance?.observationIDs?.includes(observation.observationID)) : [];

  return <section className="detail-panel evidence-explorer" data-testid="evidence-explorer">
    <div className="section-heading"><div><span className="eyebrow">Persisted evidence and provenance</span><h2>Evidence Explorer</h2><p>Browse normalized facts recorded for an Observation and follow explicit proposal attribution.</p></div></div>
    {props.observations.length ? <label className="form-field">Observation<select aria-label="Select observation" data-testid="evidence-observation-select" value={props.selectedID || ""} onChange={event => props.onSelectObservation(event.target.value)}><option value="">Choose an Observation</option>{props.observations.map(item => <option key={item.observationID} value={item.observationID}>{item.identity.workloadName} / {item.identity.container} · {item.execution.state || "UNKNOWN"} · {item.observationID}</option>)}</select></label> : null}
    {props.observationsComplete === false ? <p className="notice" data-testid="evidence-observations-partial">Observation history is partial; continue loading before concluding that no record exists.</p> : null}{props.onLoadMoreObservations ? <button type="button" className="secondary-button" disabled={props.loadingMoreObservations} onClick={props.onLoadMoreObservations}>{props.loadingMoreObservations ? "Loading more Observations…" : "Load more Observations"}</button> : null}
    {props.loading ? <div className="loading-state" role="status">Loading persisted Observation evidence…</div> : props.error ? <p className="notice error" role="alert">{props.error}</p> : !observation ? <div className="empty-state">Select an Observation from the bound workload to inspect its available evidence.</div> : <>
      <section className="evidence-context"><h3>{observation.identity.kind} / {observation.identity.workloadName}</h3><p>{observation.identity.namespace} · {observation.identity.container} · Observation <code>{observation.observationID}</code></p><dl><dt>Workload UID</dt><dd><code>{observation.identity.workloadUID || "UNKNOWN — historical record"}</code></dd><dt>Selected Pod anchor</dt><dd><code>{observation.spec?.anchorPodUID || "UNKNOWN — historical record"}</code></dd><dt>Image identity</dt><dd><code>{observation.identity.imageIdentity || "UNKNOWN — not persisted"}</code></dd><dt>Resolved runtime identities</dt><dd>{observation.resolvedTargets?.length ? observation.resolvedTargets.map(target => <span key={target.podUID}><code>Pod {target.podUID}</code> · <code>{target.containerID || "container ID UNKNOWN"}</code> · <code>{target.imageDigest || "image digest UNKNOWN"}</code>; </span>) : "UNKNOWN — no resolved Pod/container identity was persisted"}</dd></dl>{observation.targetChanges?.length ? <details><summary>Pod/workload changes recorded during capture</summary><ul>{observation.targetChanges.map((change, index) => <li key={`${change.at}-${index}`}>{change.at} · {change.kind}{change.detail ? ` · ${change.detail}` : ""}</li>)}</ul></details> : null}</section>
      <section className="evidence-source-list"><h3>Source qualification</h3>{sources.length ? sources.map(source => <article className="observation-card" key={source.name}><div className="card-heading"><div><h4>{source.name}</h4><p>{source.backend || "Backend UNKNOWN"} {source.version || "version UNKNOWN"}</p></div><Status value={source.evidenceState || "UNKNOWN"} /></div><p>Attributed count: {source.attributedCount} · excluded count: {source.excludedCount} · attribution: {source.attributionState || "UNKNOWN"}</p><p>Backend health: {source.backendHealthConfirmed ? "confirmed" : "UNKNOWN"} · attached for requested window: {source.sourceAttachedForBoundWindow ? "confirmed" : "UNKNOWN"} · flush: {source.flushConfirmed ? "confirmed" : "UNKNOWN"}</p>{source.excludedCount > 0 ? <div className="exclusion-summary"><strong>{source.excludedCount} event(s) excluded from target-attributed evidence</strong>{source.exclusionReasons?.length ? <ul>{source.exclusionReasons.map(item => <li key={item.reason}>{item.count} · {item.reason}</li>)}</ul> : <p>Per-event exclusion reasons were not retained for this historical/source result. Do not infer a reason from the count.</p>}</div> : null}{source.references?.length ? <details><summary>Persisted source references</summary><ul>{source.references.map((reference, index) => <li key={`${reference}-${index}`}><code>{reference}</code></li>)}</ul></details> : null}</article>) : <p className="notice">No source results were persisted; evidence is UNKNOWN.</p>}</section>
      <section className="evidence-facts"><div className="section-heading"><div><h3>Normalized observed facts</h3><p>Facts are deduplicated source results, not a retained raw event stream or a complete account of workload behavior.</p></div></div><div className="evidence-filters"><label>Source<select aria-label="Filter evidence source" value={sourceFilter} onChange={event => setSourceFilter(event.target.value)}><option value="all">All sources</option>{sources.map(source => <option key={source.name} value={source.name}>{source.name}</option>)}</select></label><label>Filter facts<input aria-label="Search evidence facts" value={search} onChange={event => setSearch(event.target.value)} placeholder="Path, capability, port…" /></label></div>{filtered.length ? <ul className="fact-list">{filtered.map((row, index) => <li key={`${row.source}-${row.label}-${index}`}><span className="eyebrow">{row.source}</span> <code>{row.label}</code></li>)}</ul> : <p className="empty-inline">No persisted normalized facts match this filter. Missing facts are not treated as absent behavior.</p>}</section>
      <section className="evidence-proposal-links"><h3>Proposals linked by persisted Observation provenance</h3>{props.proposalsComplete === false ? <p className="notice" data-testid="evidence-lineage-partial">Proposal lineage is partial; load remaining results before concluding no associated Proposal exists.</p> : null}{props.proposalsLoading ? <p className="loading-state">Resolving namespace-authorized proposal lineage…</p> : props.proposalsError ? <p className="notice error" role="alert">{props.proposalsError}</p> : related.length ? related.map(proposal => {
        const capabilityEntries = proposal.provenance?.capabilityAttribution;
        return <article key={proposal.uid || proposal.name} className="observation-card"><div className="card-heading"><h4>{proposal.name}</h4><Status value={proposal.status?.approvalState || "UNKNOWN"} /></div><p>Candidate digest <code>{proposal.candidateDigest || "UNKNOWN"}</code></p>{(proposal.artifact?.containerCapabilities?.add || []).map(capability => {
          const attribution = capabilityEntries?.find(entry => entry.capability === capability);
          const linked = attribution?.observationIDs?.includes(observation.observationID) || false;
          return <p key={capability}><code>{capability}</code> · {attribution?.state || "UNKNOWN — historical proposal has no per-capability attribution"}{linked ? " · this Observation is recorded as a match" : " · this Observation is not proven as support"}</p>;
        })}<button type="button" className="link-button" onClick={() => props.onOpenProposal(proposal.name, proposal.uid)}>Open associated Proposal</button></article>;
      }) : <p className="empty-inline">No exact associated Proposal was returned. This does not imply that a same-named Proposal exists.</p>}{props.onLoadMoreProposals ? <button type="button" className="secondary-button" disabled={props.loadingMoreProposals} onClick={props.onLoadMoreProposals}>{props.loadingMoreProposals ? "Loading more lineage…" : "Load more proposal lineage"}</button> : null}</section>
      <p className="muted">Recorded association is provenance, not proof of causality, completeness, safety, or universal workload behavior. Candidate and approval digests are unchanged by this explorer.</p>
    </>}
  </section>;
}
