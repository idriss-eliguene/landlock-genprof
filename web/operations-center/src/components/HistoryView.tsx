import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { contextQueryKey } from "../lib/context";
import type { AppContext, HistoryEvent, WorkloadSelection } from "../types";

function refLabel(event: HistoryEvent) {
  const ref = event.sourceRef;
  return [ref.kind, ref.namespace, ref.name, ref.uid ? `uid ${ref.uid}` : ""].filter(Boolean).join(" · ");
}

function refKey(event: HistoryEvent) {
  return [event.kind, event.sourceRef.namespace, event.sourceRef.name, event.sourceRef.uid].join("/");
}

function HistoryEventDetail({ event, onObservation, onProposal }: { event: HistoryEvent; onObservation: (id: string) => void; onProposal: (name: string) => void }) {
  const ref = event.sourceRef;
  const canObservation = ref.kind === "Observation" && Boolean(ref.name || ref.uid);
  const canProposal = ref.kind === "SecurityProfileProposal" && Boolean(ref.name);
  return <aside className="m7-detail-panel" data-testid="history-detail">
    <span className="eyebrow">Exact history entry</span>
    <h3>{event.kind}</h3>
    <p className="technical">{refLabel(event)}</p>
    <dl className="m7-definition-list">
      <div><dt>When</dt><dd>{event.timestamp || "Timestamp unavailable"}</dd></div>
      <div><dt>Claim</dt><dd>{event.claimTier}</dd></div>
      <div><dt>Temporal class</dt><dd>{event.temporalClass}</dd></div>
      <div><dt>Result/detail</dt><dd>{event.detailCode || "NOT_EXPOSED"}</dd></div>
    </dl>
    {event.relatedRef ? <p className="muted">Related authoritative resource: {event.relatedRef.kind} · {event.relatedRef.name || event.relatedRef.uid || "identity unavailable"}</p> : null}
    <div className="action-row">
      {canObservation ? <button className="secondary-button" type="button" onClick={() => onObservation(ref.name || ref.uid || "")}>Inspect Observation</button> : null}
      {canProposal ? <button className="secondary-button" type="button" onClick={() => onProposal(ref.name || "")}>Inspect Proposal</button> : null}
    </div>
  </aside>;
}

export function HistoryView({ context, selection, onObservation, onProposal }: { context: AppContext; selection: WorkloadSelection | null; onObservation: (id: string) => void; onProposal: (name: string) => void }) {
  const [eventFilter, setEventFilter] = useState("ALL");
  const [resourceFilter, setResourceFilter] = useState("ALL");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const query = useQuery({ queryKey: ["history", ...contextQueryKey(context), selection], queryFn: () => api.history(selection!, context), enabled: Boolean(selection?.imageIdentity && context.namespace && context.contextVersion), refetchInterval: 15_000 });
  const projection = query.data?.history;
  const allEvents = useMemo(() => [...(projection?.timestampedEvents || []), ...(projection?.untimestampedFacts || [])], [projection]);
  const events = useMemo(() => allEvents.filter(event => (eventFilter === "ALL" || event.kind === eventFilter) && (resourceFilter === "ALL" || event.sourceRef.kind === resourceFilter)), [allEvents, eventFilter, resourceFilter]);
  const selectedEvent = allEvents.find(event => refKey(event) === selectedKey);
  const eventKinds = [...new Set(allEvents.map(event => event.kind))].sort();
  const resourceKinds = [...new Set(allEvents.map(event => event.sourceRef.kind))].sort();
  if (!selection) return <section className="m7-view" data-testid="history-view"><div className="section-heading"><div><span className="eyebrow">Authoritative custody</span><h2>History</h2></div></div><div className="empty-state">Select a workload to inspect its exact operational history.</div></section>;
  if (!selection.imageIdentity) return <section className="m7-view" data-testid="history-view"><div className="section-heading"><div><span className="eyebrow">Authoritative custody</span><h2>History</h2><p>{selection.kind} / {selection.name} · {context.namespace}</p></div></div><div className="empty-state">History is unavailable until the selected workload has a validated image identity. No relationship is inferred from incomplete identity.</div></section>;
  return <section className="m7-view" data-testid="history-view">
    <div className="section-heading"><div><span className="eyebrow">Authoritative custody</span><h2>History</h2><p>{selection.kind} / {selection.name} · {selection.container} · {context.namespace}</p></div><span className="status-pill">{query.isFetching ? "Refreshing…" : `${projection?.totalCount ?? 0} entries`}</span></div>
    {query.isLoading ? <div className="loading-state" role="status">Loading authoritative History…</div> : query.isError ? <div className="empty-state error">History could not be read for this bound context.</div> : <>
      <div className="m7-toolbar"><label>Event<select value={eventFilter} onChange={event => setEventFilter(event.target.value)}><option value="ALL">All events</option>{eventKinds.map(kind => <option key={kind} value={kind}>{kind}</option>)}</select></label><label>Resource<select value={resourceFilter} onChange={event => setResourceFilter(event.target.value)}><option value="ALL">All resources</option>{resourceKinds.map(kind => <option key={kind} value={kind}>{kind}</option>)}</select></label><span className="muted">Filters affect presentation only.</span></div>
      {!events.length ? <div className="empty-state" data-testid="history-empty">No history entries match the current authoritative projection/filter.</div> : <div className="m7-split"><div className="timeline" data-testid="history-timeline">{events.map(event => <button type="button" className={`timeline-event ${refKey(event) === selectedKey ? "selected" : ""}`} data-testid="history-event" key={`${refKey(event)}-${event.detailCode}`} onClick={() => setSelectedKey(refKey(event))}><span className="timeline-marker" aria-hidden="true" /><span><strong>{event.kind}</strong><small>{event.timestamp || "Timestamp unavailable"}</small><code>{refLabel(event)}</code><em>{event.detailCode || event.claimTier}</em></span></button>)}</div>{selectedEvent ? <HistoryEventDetail event={selectedEvent} onObservation={onObservation} onProposal={onProposal} /> : <aside className="m7-detail-panel empty-detail"><span className="eyebrow">History detail</span><h3>Select an exact event</h3><p>Timeline relationships are shown only from authoritative identities and provenance.</p></aside>}</div>}
      {(projection?.untimestampedFacts?.length || projection?.limitations?.length || query.data?.limitation) ? <details className="m7-limitations"><summary>Projection boundaries</summary><p>{query.data?.limitation}</p>{projection?.limitations.map(item => <p key={item}><code>{item}</code></p>)}</details> : null}
    </>}
  </section>;
}
