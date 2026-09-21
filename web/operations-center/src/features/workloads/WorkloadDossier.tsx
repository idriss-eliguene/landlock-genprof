import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { AppContext, ObservationRead, ObservationSource, WorkloadSelection } from "../../types";
import { contextQueryKey } from "../../lib/context";
import { CopyButton } from "../../components/CopyButton";
import { Status } from "../../shared/Status";
import { resolutionAuthorityKey, resolvedForAuthority, selectionForLocator, type WorkloadLocator } from "./model";

const executionLabels: Record<string, string> = {
  REQUESTED: "Requested", STARTING: "Starting", RUNNING: "Running", COMPLETING: "Completing", COMPLETED: "Completed", FAILED: "Failed",
};
const completionReasons = new Set(["COMPLETED", "STOPPED_BY_REQUEST", "TARGET_REVISION_CHANGED", "TARGET_UNAVAILABLE", "EXECUTOR_LOST", "BACKEND_FAILURE"]);
const factCategories: Array<[string, string]> = [["Filesystem", "Filesystem"], ["Exec", "Exec"], ["NetworkConnect", "Network connect"], ["NetworkBind", "Network bind"], ["Capabilities", "Capabilities"]];

function field(record: Record<string, unknown>, lower: string, upper: string): unknown { return record[lower] ?? record[upper]; }

function factText(category: string, value: unknown): string {
  const record = value && typeof value === "object" ? value as Record<string, unknown> : {};
  if (category === "Filesystem") {
    const permissions = field(record, "permissions", "Permissions");
    const suffix = Array.isArray(permissions) && permissions.length ? ` (${permissions.join(", ")})` : "";
    return `${String(field(record, "path", "Path") ?? "")}${suffix}`;
  }
  if (category === "Exec") return String(field(record, "path", "Path") ?? "");
  if (category === "NetworkConnect" || category === "NetworkBind") return `${String(field(record, "direction", "Direction") ?? "")} port ${String(field(record, "port", "Port") ?? "?")}`.trim();
  return String(field(record, "name", "Name") ?? "");
}

function EvidenceMatrix({ sources }: { sources: ObservationSource[] }) {
  if (!sources.length) return <div className="empty-state" data-testid="evidence-empty">No evidence sources are recorded for this Observation.</div>;
  return <div className="table-scroll"><table className="technical-table" data-testid="evidence-matrix"><thead><tr><th>Source</th><th>State</th><th>Attribution</th><th>Attached</th><th>Flush confirmed</th><th>Excluded</th><th>References</th></tr></thead><tbody>{sources.map(source => <tr key={source.name}><td>{source.name}</td><td><Status value={source.evidenceState} label={source.evidenceState || "UNKNOWN"} /></td><td>{source.attributionState || "UNKNOWN"}</td><td>{source.sourceAttachedForBoundWindow ? "Confirmed" : "Not confirmed"}</td><td>{source.flushConfirmed ? "Confirmed" : "Not confirmed"}</td><td>{source.excludedCount}</td><td>{source.references?.length ? source.references.map(reference => <code key={reference} className="stacked-code">{reference}</code>) : "Not exposed"}</td></tr>)}</tbody></table></div>;
}

function FactsView({ sources }: { sources: ObservationSource[] }) {
  const categories = useMemo(() => factCategories.map(([key, label]) => ({ key, label, values: sources.flatMap(source => {
    const facts = source.facts && typeof source.facts === "object" ? source.facts as Record<string, unknown> : {};
    const raw = field(facts, key.charAt(0).toLowerCase() + key.slice(1), key);
    return Array.isArray(raw) ? raw.map(value => factText(key, value)).filter(Boolean) : [];
  }) })), [sources]);
  return <div className="facts-grid" data-testid="facts-view">{categories.map(category => <section className="fact-category" key={category.key}><h4>{category.label}</h4>{category.values.length ? <ul>{category.values.map((value, index) => <li key={`${value}-${index}`}><code>{value}</code></li>)}</ul> : <p className="muted">No normalized facts available for this category.</p>}</section>)}</div>;
}

function ObservationExecution({ observation }: { observation: ObservationRead }) {
  const execution = observation.execution;
  const reason = execution.completion && completionReasons.has(execution.completion) ? execution.completion : execution.completion || "NOT_RECORDED";
  return <section className="dossier-section" data-testid="observation-execution"><div className="section-heading"><div><span className="eyebrow">Authoritative lifecycle</span><h3>Execution</h3></div><Status value={execution.state} label={executionLabels[execution.state || ""] || execution.state || "UNKNOWN"} /></div><div className="execution-grid"><span><b>Created</b>{observation.createdAt || "Not exposed"}</span><span><b>Started</b>{execution.startedAt || "Not recorded"}</span><span><b>Current state</b>{executionLabels[execution.state || ""] || execution.state || "UNKNOWN"}</span><span><b>Stop requested</b>{execution.stopRequestedAt || "Not recorded"}</span><span><b>Completed</b>{execution.completedAt || "Not recorded"}</span><span><b>Completion reason</b>{reason}</span><span><b>Frozen</b>{observation.frozen ? "Yes" : "No"}</span></div>{execution.failure ? <div className="failure-panel"><h4>Failure details</h4><p>{execution.failure.code || "BACKEND_FAILURE"}: {execution.failure.reason || "No failure reason exposed."}</p>{execution.failure.occurredAt ? <p>Occurred: {execution.failure.occurredAt}</p> : null}</div> : null}</section>;
}

function ObservationPanel({ observation }: { observation: ObservationRead }) {
  const [tab, setTab] = useState<"execution" | "evidence" | "facts">("execution");
  return <article className="dossier-observation" data-testid="observation-detail"><div className="detail-heading"><div><span className="eyebrow">Exact Observation</span><h3>{observation.observationID}</h3><p>{observation.identity.kind}/{observation.identity.workloadName} · {observation.identity.container}</p></div><Status value={observation.execution.state} /></div><div className="representation-tabs dossier-tabs" role="tablist"><button role="tab" aria-selected={tab === "execution"} className={tab === "execution" ? "selected" : ""} onClick={() => setTab("execution")}>Execution</button><button role="tab" aria-selected={tab === "evidence"} className={tab === "evidence" ? "selected" : ""} onClick={() => setTab("evidence")}>Evidence</button><button role="tab" aria-selected={tab === "facts"} className={tab === "facts" ? "selected" : ""} onClick={() => setTab("facts")}>Facts</button></div>{tab === "execution" ? <ObservationExecution observation={observation} /> : tab === "evidence" ? <section className="dossier-section"><h3>Evidence by source</h3><p className="muted">Each source is authoritative independently. No aggregate evidence state replaces this vector.</p><EvidenceMatrix sources={observation.sources} /></section> : <section className="dossier-section"><h3>Normalized facts</h3><FactsView sources={observation.sources} /></section>}</article>;
}

export function WorkloadDossier({ context, locator, onBack }: { context: AppContext; locator: WorkloadLocator; onBack: () => void }) {
  const [observationID, setObservationID] = useState<string | null>(null);
  const ready = Boolean(context.namespace && context.contextVersion && context.sessionID);
  const authorityKey = resolutionAuthorityKey(contextQueryKey(context));
  const [resolvedAuthorityKey, setResolvedAuthorityKey] = useState<string | null>(null);
  const workloadQuery = useQuery({ queryKey: ["dossier-workloads", ...contextQueryKey(context)], queryFn: () => api.workloads(context), enabled: ready });
  const candidateSelection = workloadQuery.data ? selectionForLocator(workloadQuery.data, locator) : undefined;
  const resolved = resolvedForAuthority(resolvedAuthorityKey, authorityKey);
  const selection = resolved ? candidateSelection : undefined;
  useEffect(() => {
    setResolvedAuthorityKey(null);
    setObservationID(null);
  }, [authorityKey]);
  useEffect(() => {
    if (ready && !workloadQuery.isFetching && !workloadQuery.isError && candidateSelection) setResolvedAuthorityKey(authorityKey);
  }, [authorityKey, candidateSelection, ready, workloadQuery.isError, workloadQuery.isFetching]);
  const detailQuery = useQuery({ queryKey: ["dossier-workload-detail", ...contextQueryKey(context), selection], queryFn: () => api.workloadDetail(selection!, context), enabled: Boolean(selection && ready) });
  const observationsQuery = useQuery({ queryKey: ["dossier-observations", ...contextQueryKey(context), selection], queryFn: () => api.observations(selection!, context), enabled: Boolean(selection && ready), refetchInterval: 5_000 });
  const selectedObservation = useQuery({ queryKey: ["dossier-observation", ...contextQueryKey(context), observationID], queryFn: () => api.observation(observationID!, context), enabled: Boolean(observationID && ready), refetchInterval: query => ["COMPLETED", "FAILED"].includes(query.state.data?.execution.state || "") ? false : 3_000 });
  const identityMismatch = context.namespace !== locator.namespace;
  return <section className="workload-dossier" data-testid="workload-dossier"><div className="dossier-breadcrumb"><button type="button" className="link-button" onClick={onBack}>← Workloads</button></div>{!ready || identityMismatch ? <div className="empty-state error" role="alert">{identityMismatch ? "This workload belongs to a different bound namespace. Rebind the authoritative context before opening it." : "Bind an authoritative Operational Context before resolving this workload."}</div> : workloadQuery.isError ? <div className="empty-state error" role="alert">The workload locator could not be resolved in the current context. No dossier was rendered.</div> : !resolved ? <div className="loading-state" role="status">Resolving workload locator authoritatively…</div> : !selection ? <div className="empty-state error" role="alert">The workload locator could not be resolved in the current context. No dossier was rendered.</div> : <><header className="dossier-header"><div><span className="eyebrow">Verified workload dossier</span><h2>{selection.kind} / {selection.name}</h2><p>{context.namespace} · container <code>{selection.container}</code></p></div><Status value={detailQuery.isError ? "ERROR" : detailQuery.isLoading ? "RESOLVING" : "AVAILABLE"} label={detailQuery.isError ? "Resolution failed" : detailQuery.isLoading ? "Resolving" : "Resolved"} /><div className="dossier-identity"><span>Workload UID</span><code>{selection.workloadUID}</code><CopyButton value={selection.workloadUID} /></div></header><section className="dossier-section dossier-overview"><div className="section-heading"><div><span className="eyebrow">Exact identity</span><h3>Overview</h3></div></div><div className="identity-grid"><span><b>Cluster identity</b><code>{context.cluster || "NOT_RESOLVED"}</code></span><span><b>Namespace</b><code>{context.namespace}</code></span><span><b>Group / Kind</b><code>{selection.group || "core"}/{selection.kind}</code></span><span><b>Name</b><code>{selection.name}</code></span><span><b>Container slot</b><code>{selection.container}</code></span><span><b>UID</b><code>{selection.workloadUID}</code></span></div>{detailQuery.isError ? <p className="notice error">The workload object changed or is no longer available. Historical data was not attached to this locator.</p> : null}{detailQuery.data ? <details><summary>Resolved technical manifest</summary><div className="yaml-card"><div className="code-heading"><strong>Authoritative backend projection</strong><CopyButton value={detailQuery.data.yaml} /></div><pre tabIndex={0}>{detailQuery.data.yaml}</pre></div></details> : null}</section><section className="dossier-section"><div className="section-heading"><div><span className="eyebrow">Exact UID and container relation</span><h3>Observations</h3></div><span className="status-pill">{observationsQuery.isFetching ? "Refreshing…" : `${observationsQuery.data?.items.length ?? 0} records`}</span></div>{observationsQuery.isLoading ? <div className="loading-state" role="status">Loading exact Observations…</div> : observationsQuery.isError ? <div className="empty-state error">Observation query failed authoritatively.</div> : observationsQuery.data?.items.length ? <div className="dossier-observation-list">{observationsQuery.data.items.map(item => <button type="button" className={`dossier-observation-row ${observationID === item.observationID ? "selected" : ""}`} key={item.observationID} onClick={() => setObservationID(item.observationID)}><span><strong>{executionLabels[item.execution.state || ""] || item.execution.state || "UNKNOWN"}</strong><code>{item.observationID}</code></span><span>{item.frozen ? "Frozen" : "Not frozen"} · {item.createdAt || "Time unavailable"}</span></button>)}</div> : <div className="empty-state">No Observations belong to this exact workload UID and container.</div>}{observationID && selectedObservation.data ? <ObservationPanel observation={selectedObservation.data} /> : observationID && selectedObservation.isLoading ? <div className="loading-state" role="status">Loading exact Observation detail…</div> : observationID ? <div className="empty-state error">Exact Observation detail is unavailable; it was not substituted from the collection row.</div> : <p className="muted">Select an Observation to inspect Execution, per-source Evidence, and category-specific Facts.</p>}</section></>}</section>;
}
