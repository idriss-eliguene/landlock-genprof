import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { contextQueryKey } from "../lib/context";
import type { AppContext, AttentionReason, SphmAttention } from "../types";

const categoryCopy: Record<string, string> = {
  CAPABILITY_OUTSIDE_APPROVED_POLICY: "Observed capability is outside the approved policy",
  APPROVED_NOT_APPLIED: "Approved Proposal has not been applied",
  APPLICATION_STATE_UNKNOWN: "Application state is unknown",
  NEW_CONTRIBUTION_SINCE_CANDIDATE: "New contribution exists since the candidate baseline",
  OBSERVATION_FAILED: "Observation failed",
  MULTIPLE_VALID_APPROVED_PROPOSALS: "Multiple approved Proposals match this subject",
};

function attentionKey(reason: AttentionReason, subjectIndex: number) {
  return [reason.category, reason.subject?.target, reason.subject?.container, ...(reason.observationRefs || []), ...(reason.proposalRefs || []).map(ref => ref.name || ref.uid), subjectIndex].join("/");
}

function subjectLabel(reason: AttentionReason) {
  const subject = reason.subject;
  return [subject?.scope, subject?.target, subject?.container, subject?.imageIdentity].filter(Boolean).join(" · ") || "Subject identity not exposed";
}

function sphmSubjectLabel(item: SphmAttention) {
  return item.workload || item.observationID || item.proposalName || "Exact related identity not exposed";
}

function AttentionDetail({ reason, onObservation, onProposal }: { reason: AttentionReason; onObservation: (id: string) => void; onProposal: (name: string) => void }) {
  return <aside className="m7-detail-panel" data-testid="attention-detail"><span className="eyebrow">Exact attention item</span><h3>{categoryCopy[reason.category] || reason.category}</h3><p>{subjectLabel(reason)}</p><dl className="m7-definition-list"><div><dt>Category</dt><dd><code>{reason.category}</code></dd></div><div><dt>Explanation</dt><dd><code>{reason.explanationCode || "NOT_EXPOSED"}</code></dd></div><div><dt>Evidence refs</dt><dd>{reason.evidenceRefs?.join(", ") || "NOT_APPLICABLE"}</dd></div></dl><p className="muted">Attention is an authoritative operational projection. An empty Attention view does not establish security health.</p><div className="action-row">{(reason.observationRefs || []).map(id => <button key={id} className="secondary-button" type="button" onClick={() => onObservation(id)}>Inspect Observation</button>)}{(reason.proposalRefs || []).filter(ref => ref.name).map(ref => <button key={ref.name} className="secondary-button" type="button" onClick={() => onProposal(ref.name!)}>Inspect Proposal</button>)}</div></aside>;
}

export function AttentionView({ context, onObservation, onProposal }: { context: AppContext; onObservation: (id: string) => void; onProposal: (name: string) => void }) {
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const query = useQuery({ queryKey: ["attention", ...contextQueryKey(context)], queryFn: () => api.environmentProjection(context), enabled: Boolean(context.namespace && context.contextVersion), refetchInterval: 15_000 });
  const health = useQuery({ queryKey: ["health", ...contextQueryKey(context)], queryFn: () => api.health(context), enabled: Boolean(context.namespace && context.contextVersion), refetchInterval: 15_000 });
  const items = useMemo(() => (query.data?.items || []).flatMap((item, index) => (item.attention || []).map((reason, reasonIndex) => ({ key: attentionKey(reason, index * 1000 + reasonIndex), reason }))), [query.data]);
  const selected = items.find(item => item.key === selectedKey)?.reason;
  if (query.isLoading || health.isLoading) return <section className="m7-view" data-testid="attention-view"><div className="section-heading"><div><span className="eyebrow">Operator action surface</span><h2>Attention</h2></div></div><div className="loading-state" role="status">Loading authoritative Attention projections…</div></section>;
  const sphmItems = health.data?.attention || [];
  return <section className="m7-view" data-testid="attention-view"><div className="section-heading"><div><span className="eyebrow">Operator action surface</span><h2>Attention</h2><p>Two authoritative projections are shown separately; they do not share a lifecycle or severity model.</p></div><span className="status-pill">{query.isFetching || health.isFetching ? "Refreshing…" : `${items.length + sphmItems.length} diagnostics`}</span></div><section className="attention-source" data-testid="attention-reconciliation"><div className="section-heading"><div><span className="eyebrow">Reconciliation projection</span><h3>Operational reconciliation</h3><p>Predicate categories and durable references from <code>/api/v08/environment</code>.</p></div></div>{query.isError ? <div className="empty-state error">Operational reconciliation could not be read for this bound context. No healthy state is inferred.</div> : !items.length ? <div className="empty-state" data-testid="attention-empty"><h3>No current reconciliation predicates</h3><p>This does not establish security health.</p></div> : <div className="m7-split"><div className="attention-list">{items.map(item => <button type="button" key={item.key} className={`attention-item ${item.key === selectedKey ? "selected" : ""}`} data-testid="attention-item" onClick={() => setSelectedKey(item.key)}><span className="attention-marker" aria-hidden="true">!</span><span><strong>{categoryCopy[item.reason.category] || item.reason.category}</strong><small>{subjectLabel(item.reason)}</small><em>{item.reason.explanationCode || "Authoritative reconciliation condition"}</em></span></button>)}</div>{selected ? <AttentionDetail reason={selected} onObservation={onObservation} onProposal={onProposal} /> : <aside className="m7-detail-panel empty-detail"><span className="eyebrow">Reconciliation detail</span><h3>Select an item</h3><p>Inspect the exact object references responsible for this condition.</p></aside>}</div>}</section><section className="attention-source" data-testid="attention-health-diagnostics"><div className="section-heading"><div><span className="eyebrow">SPHM projection</span><h3>Health diagnostics</h3><p>Diagnostic items emitted by the server-owned SPHM report from <code>/api/health</code>.</p></div></div>{health.isError ? <div className="empty-state error">SPHM diagnostics could not be read for this bound context.</div> : !sphmItems.length ? <div className="empty-state"><h3>No current SPHM diagnostics</h3><p>This does not establish healthy security posture.</p></div> : <div className="attention-list">{sphmItems.map(item => <div className="attention-item attention-item-static" data-testid="sphm-attention-item" key={item.id}><span className="attention-marker" aria-hidden="true">!</span><span><strong>{item.title}</strong><small>{sphmSubjectLabel(item)}</small><em>{item.kind} · {item.reason}</em></span><span className="action-row">{item.observationID ? <button type="button" className="link-button" onClick={() => onObservation(item.observationID!)}>Inspect Observation</button> : null}{item.proposalName ? <button type="button" className="link-button" onClick={() => onProposal(item.proposalName!)}>Inspect Proposal</button> : null}</span></div>)}</div>}</section></section>;
}
