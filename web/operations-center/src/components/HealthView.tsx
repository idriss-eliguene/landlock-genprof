import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { contextQueryKey } from "../lib/context";
import type { AppContext, SphmDimension, SphmReport } from "../types";
import { attentionIdentity, dimensionValue, nonHealthyDimensions, SPHM_DIMENSION_IDS } from "../features/health/model";

const stateLabel: Record<string, string> = {
  HEALTHY: "Healthy", ATTENTION: "Attention", CRITICAL: "Critical", UNKNOWN: "Unknown",
  NOT_ESTABLISHED: "Not established", NOT_APPLICABLE: "Not applicable",
};

function StateBadge({ state }: { state?: string }) {
  const value = state || "UNKNOWN";
  return <span className={`health-state health-state-${value.toLowerCase()}`} role="status">{stateLabel[value] || value}</span>;
}

function DimensionCard({ item, onObservation, onProposal }: { item: SphmDimension; onObservation: (id: string) => void; onProposal: (name: string) => void }) {
  const drilldown = item.drilldown || "";
  return <article className="health-dimension-card" data-testid={`health-dimension-${item.id}`}>
    <div className="health-card-heading"><div><span className="eyebrow">Dimension</span><h3>{item.name}</h3></div><StateBadge state={item.state} /></div>
    <div className="health-card-value">{dimensionValue(item)}</div>
    <p className="health-reason">{item.reason || "Authoritative explanation not exposed."}</p>
    <dl className="health-definition-list"><div><dt>Source</dt><dd>{item.authoritativeSource || "NOT_EXPOSED"}</dd></div><div><dt>Projection evaluated at</dt><dd>{item.evaluatedAt || "NOT_EXPOSED"}</dd></div></dl>
    {item.state === "NOT_ESTABLISHED" ? <p className="health-missing-proof"><strong>What is missing:</strong> The authoritative model cannot establish this dimension from the current product state.</p> : null}
    {drilldown === "observations" ? <button type="button" className="link-button" onClick={() => onObservation("")}>Inspect Observations</button> : null}
    {drilldown === "proposals" ? <button type="button" className="link-button" onClick={() => onProposal("")}>Inspect Proposals</button> : null}
    {drilldown.startsWith("/api/") ? <span className="health-technical-drilldown">Read model: <code>{drilldown}</code></span> : null}
  </article>;
}

function TemporalSemantics({ report }: { report: SphmReport }) {
  return <section className="health-panel health-temporal" data-testid="health-temporal"><div className="health-panel-heading"><div><span className="eyebrow">Temporal semantics</span><h3>Projection time is not evidence freshness</h3></div></div><p>A successful health read reports when SPHM evaluated its current projection. It does not move the underlying evidence timestamps forward.</p><div className="health-time-grid"><div><strong>ObservedAt</strong><span>NOT_EXPOSED by current SPHM projection</span></div><div><strong>CollectedAt</strong><span>NOT_EXPOSED by current SPHM projection</span></div><div><strong>QualifiedAt</strong><span>NOT_EXPOSED by current SPHM projection</span></div><div><strong>ProjectedAt</strong><span>{report.overall.evaluatedAt || "NOT_EXPOSED"}</span></div></div><p className="muted">Freshness remains <strong>NOT_ESTABLISHED</strong> because no authoritative freshness policy and evidence-age contract currently exists.</p></section>;
}

function HealthError() {
  return <div className="health-error" role="alert">The authoritative SPHM projection could not be read for this context. No healthy, empty, or current state is inferred.</div>;
}

export function HealthView({ context, onObservation, onProposal, onAttention, onWorkloads }: { context: AppContext; onObservation: (id: string) => void; onProposal: (name: string) => void; onAttention: () => void; onWorkloads: () => void }) {
  const enabled = Boolean(context.namespace && context.contextVersion);
  const health = useQuery({ queryKey: ["health", ...contextQueryKey(context)], queryFn: () => api.health(context), enabled, refetchInterval: 15_000 });
  const overview = useQuery({ queryKey: ["overview", ...contextQueryKey(context)], queryFn: () => api.overview(context), enabled, refetchInterval: 15_000 });
  if (!enabled) return <section className="health-view" data-testid="health-view"><div className="empty-state">Bind an environment context to inspect authoritative SPHM state.</div></section>;
  if (health.isLoading) return <section className="health-view" data-testid="health-view"><div className="health-loading" role="status">Loading authoritative SPHM projection…</div></section>;
  if (health.isError || !health.data) return <section className="health-view" data-testid="health-view"><HealthError /></section>;
  const report = health.data;
  const issues = nonHealthyDimensions(report);
  const attention = report.attention || [];
  return <section className="health-view" data-testid="health-view">
    <div className="health-intro"><div><span className="eyebrow">Security Profile Health Model · {report.modelVersion}</span><h2>Health</h2><p>Authoritative security-profile posture for <code>{context.cluster}</code> / <code>{context.namespace}</code>.</p></div><div className="health-intro-state"><span className="eyebrow">Overall posture</span><StateBadge state={report.overall.state} /></div></div>
    <section className="health-panel health-posture" data-testid="health-posture"><div><span className="eyebrow">Why this state</span><h3>{report.overall.name}</h3><p>{report.overall.reason}</p></div><div className="health-posture-summary">{issues.length ? <><strong>{issues.length} dimension{issues.length === 1 ? "" : "s"} require interpretation</strong><span>Review the authoritative reasons below; unavailable proof is not treated as healthy.</span></> : <span>No non-healthy dimension was reported by SPHM.</span>}</div></section>
    <section className="health-panel" data-testid="health-dimensions"><div className="health-panel-heading"><div><span className="eyebrow">Authoritative dimension ledger</span><h3>SPHM v1</h3><p>Each dimension remains independent. States, values, sources and reasons are server-owned; no score is calculated in React.</p></div><span className="health-refresh" role="status">{health.isFetching ? "Refreshing…" : "Projection loaded"}</span></div><div className="health-dimension-grid">{SPHM_DIMENSION_IDS.map(id => { const item = report.dimensions.find(dimension => dimension.id === id); return item ? <DimensionCard key={id} item={item} onObservation={idValue => idValue ? onObservation(idValue) : onWorkloads()} onProposal={name => name ? onProposal(name) : onProposal("")} /> : <div className="health-dimension-missing" key={id}><strong>{id}</strong><StateBadge state="NOT_ESTABLISHED" /><p>Dimension is not present in the authoritative report.</p></div>; })}</div></section>
    <div className="health-two-column"><section className="health-panel" data-testid="health-attention"><div className="health-panel-heading"><div><span className="eyebrow">Operator action</span><h3>Active issues{attention.length ? ` · ${attention.length}` : ""}</h3></div><button type="button" className="link-button" onClick={onAttention}>View Attention</button></div>{!attention.length ? <div className="health-empty"><strong>No current SPHM attention items</strong><p>This does not establish that the security profile is healthy.</p></div> : <><div className="health-issue-list">{attention.slice(0, 8).map(item => <div className="health-issue" data-testid="health-issue" key={item.id}><div><strong>{item.title}</strong><p>{item.reason}</p><small>{item.workload || "Workload not exposed"} · {attentionIdentity(item)}</small></div><div className="action-row">{item.observationID ? <button type="button" className="link-button" onClick={() => onObservation(item.observationID!)}>Inspect Observation</button> : null}{item.proposalName ? <button type="button" className="link-button" onClick={() => onProposal(item.proposalName!)}>Inspect Proposal</button> : null}</div></div>)}</div>{attention.length > 8 ? <p className="muted">Showing 8 of {attention.length} authoritative issues. Open Attention for the complete exact-resource list.</p> : null}</>}</section><section className="health-panel" data-testid="health-proof"><div className="health-panel-heading"><div><span className="eyebrow">Evidence and pipeline</span><h3>Proof posture</h3></div></div><p>Dimension state is separate from quantity. Capability facts, observations, or fetched projections do not establish qualification by themselves.</p><div className="health-proof-list">{["evidence", "pipeline", "governance"].map(id => { const item = report.dimensions.find(dimension => dimension.id === id); return item ? <div key={id}><strong>{item.name}</strong><StateBadge state={item.state} /><span>{item.reason}</span></div> : null; })}</div><button type="button" className="link-button" onClick={onWorkloads}>Inspect authoritative observations</button></section></div>
    <TemporalSemantics report={report} />
    <section className="health-panel health-context" data-testid="health-context"><span className="eyebrow">Authority binding</span><p>Cluster <code>{report.context.clusterIdentity || context.cluster}</code> · Namespace <code>{report.context.namespace || context.namespace}</code> · Context version <code>{report.context.contextVersion || context.contextVersion}</code></p><p className="muted">This tab is bound to its authenticated EnvironmentSession. Health is not aggregated across namespaces.</p></section>
    {overview.isError ? <div className="health-secondary-error" role="alert">Attention and History drilldown projection is unavailable. SPHM dimensions above remain authoritative; no empty drilldown is substituted.</div> : null}
  </section>;
}
