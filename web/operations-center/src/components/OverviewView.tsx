import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { contextQueryKey } from "../lib/context";
import type { AppContext, SphmDimension } from "../types";
import { attentionReasons, dimension, displayMetric, recentEvents } from "../features/overview/model";

const stateLabel: Record<string, string> = {
  HEALTHY: "Healthy", ATTENTION: "Attention", CRITICAL: "Critical", UNKNOWN: "Unknown",
  NOT_ESTABLISHED: "Not established", NOT_APPLICABLE: "Not applicable",
};

function StateBadge({ item, fallback = "UNKNOWN" }: { item?: SphmDimension; fallback?: string }) {
  const state = item?.state || fallback;
  return <span className={`overview-state overview-state-${state.toLowerCase()}`} role="status">{stateLabel[state] || state}</span>;
}

function SectionError({ name }: { name: string }) {
  return <div className="overview-section-error" role="alert">{name} could not be read for this bound context. No healthy state is inferred from this error.</div>;
}

export function OverviewView({ context, onObservation, onProposal, onAttention, onHistory, onWorkloads }: {
  context: AppContext;
  onObservation: (id: string) => void;
  onProposal: (name: string) => void;
  onAttention: () => void;
  onHistory: () => void;
  onWorkloads: () => void;
}) {
  const enabled = Boolean(context.namespace && context.contextVersion);
  const health = useQuery({ queryKey: ["health", ...contextQueryKey(context)], queryFn: () => api.health(context), enabled, refetchInterval: 15_000 });
  const overview = useQuery({ queryKey: ["overview", ...contextQueryKey(context)], queryFn: () => api.overview(context), enabled, refetchInterval: 15_000 });
  const evidence = dimension(health.data, "evidence");
  const pipeline = dimension(health.data, "pipeline");
  const governance = dimension(health.data, "governance");
  const attention = attentionReasons(overview.data?.environment);
  const events = recentEvents(overview.data?.history);
  const loading = health.isLoading || overview.isLoading;

  if (!enabled) return <section className="overview-view" data-testid="overview-view"><div className="overview-empty">Bind an environment context to load the authoritative operational overview.</div></section>;
  return <section className="overview-view" data-testid="overview-view">
    <div className="overview-intro">
      <div><span className="eyebrow">Operational cockpit · {context.namespace}</span><h2>Overview</h2><p>Current namespace-bound signals from the authoritative Operations Center projections.</p></div>
      <span className="overview-refresh" role="status">{loading ? "Loading authoritative projections…" : health.isFetching || overview.isFetching ? "Refreshing…" : "Projections loaded"}</span>
    </div>
    <div className="overview-posture overview-panel" data-testid="overview-posture">
      <div><span className="eyebrow">Operational posture</span><h3>{loading ? "Evaluating authoritative state…" : health.data?.overall.name || "Posture unavailable"}</h3><p>{health.data?.overall.reason || (health.isError ? "The SPHM projection is unavailable." : "Waiting for authoritative evaluation.")}</p></div>
      <StateBadge item={health.data?.overall} />
    </div>
    <div className="overview-grid">
      <section className="overview-panel overview-attention" data-testid="overview-attention"><div className="overview-panel-heading"><div><span className="eyebrow">Operator action</span><h3>Needs attention</h3></div><button type="button" className="link-button" onClick={onAttention}>View all</button></div>{overview.isError ? <SectionError name="Attention" /> : !overview.data ? <div className="overview-loading">Loading authoritative Attention…</div> : !attention.length ? <div className="overview-empty"><strong>No current Attention items</strong><p>Empty Attention does not establish security health.</p></div> : <div className="overview-signal-list">{attention.slice(0, 4).map((reason, index) => <div className="overview-signal" key={`${reason.category}-${reason.subject?.target}-${index}`}><div><strong>{reason.category}</strong><p>{reason.explanationCode || "Authoritative condition requires investigation."}</p><small>{reason.subject?.target || "Exact workload identity not exposed"} · {reason.subject?.container || "container not exposed"}</small></div><div className="action-row">{reason.observationRefs?.[0] ? <button type="button" className="link-button" onClick={() => onObservation(reason.observationRefs![0])}>Inspect Observation</button> : null}{reason.proposalRefs?.find(ref => ref.name)?.name ? <button type="button" className="link-button" onClick={() => onProposal(reason.proposalRefs!.find(ref => ref.name)!.name!)}>Inspect Proposal</button> : null}</div></div>)}</div>}</section>
      <section className="overview-panel" data-testid="overview-evidence"><div className="overview-panel-heading"><div><span className="eyebrow">Evidence posture</span><h3>Qualification</h3></div><StateBadge item={evidence} /></div>{health.isError ? <SectionError name="Evidence" /> : <><strong className="overview-metric">{displayMetric(evidence)}</strong><p>{evidence?.reason || "Evidence projection is loading."}</p><small className="overview-scope">Scope: current namespace observations · retained authoritative read-model population</small><button type="button" className="link-button" onClick={onWorkloads}>Inspect Observations</button></>}</section>
      <section className="overview-panel" data-testid="overview-pipeline"><div className="overview-panel-heading"><div><span className="eyebrow">Execution posture</span><h3>Observation pipeline</h3></div><StateBadge item={pipeline} /></div>{health.isError ? <SectionError name="Pipeline" /> : <><strong className="overview-metric">{displayMetric(pipeline)}</strong><p>{pipeline?.reason || "Pipeline projection is loading."}</p><button type="button" className="link-button" onClick={onWorkloads}>Inspect Observations</button></>}</section>
      <section className="overview-panel" data-testid="overview-governance"><div className="overview-panel-heading"><div><span className="eyebrow">Governance posture</span><h3>Proposal queue</h3></div><StateBadge item={governance} /></div>{health.isError ? <SectionError name="Governance" /> : <><strong className="overview-metric">{displayMetric(governance)}</strong><p>{governance?.reason || "Governance projection is loading."}</p><button type="button" className="link-button" onClick={() => onProposal("")}>Inspect Proposals</button></>}</section>
    </div>
    <section className="overview-panel overview-activity" data-testid="overview-recent-activity"><div className="overview-panel-heading"><div><span className="eyebrow">Authoritative event projection</span><h3>Recent activity</h3></div><button type="button" className="link-button" onClick={onHistory}>View full History</button></div>{overview.isError ? <SectionError name="Recent activity" /> : !overview.data ? <div className="overview-loading">Loading recent activity…</div> : !events.length ? <div className="overview-empty">No timestamped activity is present in the retained projection.</div> : <div className="overview-activity-list">{events.map((event, index) => <button type="button" className="overview-activity-row" key={`${event.kind}-${event.sourceRef.kind}-${event.sourceRef.name}-${index}`} onClick={() => event.sourceRef.kind === "Observation" && (event.sourceRef.name || event.sourceRef.uid) ? onObservation(event.sourceRef.name || event.sourceRef.uid!) : event.sourceRef.kind === "SecurityProfileProposal" && event.sourceRef.name ? onProposal(event.sourceRef.name) : onHistory()}><span className="overview-activity-kind">{event.kind}</span><span className="overview-activity-ref">{event.sourceRef.kind} · {event.sourceRef.name || event.sourceRef.uid || "identity not exposed"}</span><time>{event.timestamp || "Timestamp unavailable"}</time></button>)}</div>}</section>
    <section className="overview-panel overview-sphm" data-testid="overview-sphm"><div className="overview-panel-heading"><div><span className="eyebrow">Model preview</span><h3>Security Profile Health</h3><p>Existing SPHM v1 dimensions; full Health investigation is reserved for M9.</p></div><StateBadge item={health.data?.overall} /></div>{health.isError ? <SectionError name="SPHM" /> : !health.data ? <div className="overview-loading">Loading SPHM dimensions…</div> : <div className="overview-dimension-grid">{health.data.dimensions.map(item => <div className="overview-dimension" key={item.id}><div><strong>{item.name}</strong><small>{item.reason}</small></div><StateBadge item={item} /></div>)}</div>}</section>
  </section>;
}
