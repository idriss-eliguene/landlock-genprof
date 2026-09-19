import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "../api/client";
import type { AppContext, WorkloadSelection } from "../types";
import { CopyButton } from "../components/CopyButton";
import { contextQueryKey } from "../lib/context";
import "../styles.css";

const emptyContext: AppContext = { cluster: "", namespace: "", sessionID: "", contextVersion: 0, identity: "" };

function selectionsFromResponse(response: Awaited<ReturnType<typeof api.workloads>>): WorkloadSelection[] {
  return response.workloads.flatMap(workload => (workload.pods ?? []).flatMap(pod => (pod.containers ?? []).flatMap(container => {
    const target = container.target?.workload;
    if (!target || !container.supportedTarget) return [];
    return [{ group: target.group, kind: target.kind, name: target.name, container: container.name, workloadUID: workload.uid ?? pod.uid ?? "", imageIdentity: container.runtime?.imageID }];
  })));
}

export function App() {
  const queryClient = useQueryClient();
  const [page, setPage] = useState("overview");
  const [context, setContext] = useState<AppContext>(emptyContext);
  const [selected, setSelected] = useState<WorkloadSelection | null>(null);
  const [namespace, setNamespace] = useState("");
  const [notice, setNotice] = useState<{ tone: "info" | "error"; text: string } | null>(null);

  const contexts = useQuery({ queryKey: ["contexts"], queryFn: api.contexts, staleTime: 30_000 });
  const session = useMutation({ mutationFn: api.openContext });
  const namespaces = useQuery({
    queryKey: ["namespaces", context.sessionID],
    queryFn: () => api.namespaces(context.sessionID),
    enabled: Boolean(context.sessionID),
  });
  const workloads = useQuery({
    queryKey: ["workloads", ...contextQueryKey(context)],
    queryFn: () => api.workloads(context),
    enabled: Boolean(context.namespace && context.contextVersion),
    refetchInterval: 15_000,
  });
  const operational = useQuery({
    queryKey: ["operational-context", ...contextQueryKey(context)],
    queryFn: () => api.operationalContext(context),
    enabled: Boolean(context.namespace && context.contextVersion),
    refetchInterval: 15_000,
  });
  const detail = useQuery({
    queryKey: ["workload-detail", ...contextQueryKey(context), selected],
    queryFn: () => api.workloadDetail(selected!, context),
    enabled: Boolean(selected && context.namespace && context.contextVersion),
  });
  const selections = useMemo(() => workloads.data ? selectionsFromResponse(workloads.data) : [], [workloads.data]);

  useEffect(() => {
    if (!selected) return;
    const stillDiscovered = selections.some(item => item.name === selected.name && item.container === selected.container && item.workloadUID === selected.workloadUID);
    if (!stillDiscovered && workloads.isSuccess) setSelected(null);
  }, [selected, selections, workloads.isSuccess]);

  const changeIdentity = async (identity: string) => {
    const previousSelection = selected;
    setNotice(null); setSelected(null); setNamespace("");
    try {
      const opened = await session.mutateAsync(identity);
      const discovered = await api.namespaces(opened.sessionID);
      const initialNamespace = opened.defaultNamespace || discovered.namespaces?.[0] || "";
      if (!initialNamespace) { setContext({ cluster: "", namespace: "", sessionID: opened.sessionID, contextVersion: 0, identity }); return; }
      const provisional: AppContext = { cluster: contexts.data?.contexts.find(item => item.contextName === identity)?.clusterIdentity ?? "", namespace: initialNamespace, sessionID: opened.sessionID, contextVersion: 0, identity };
      const bound = await api.bindNamespace(opened.sessionID, initialNamespace, provisional);
      setNamespace(bound.namespace);
      setContext({ ...provisional, namespace: bound.namespace, contextVersion: bound.contextVersion });
    } catch (error) {
      setSelected(previousSelection);
      setNamespace(context.namespace);
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "Environment context could not be opened." });
    }
  };

  const changeNamespace = async (value: string) => {
    if (!value || !context.sessionID) return;
    const previousSelection = selected;
    setNotice(null); setSelected(null);
    try {
      const bound = await api.bindNamespace(context.sessionID, value, { ...context, namespace: value });
      const next = { ...context, namespace: bound.namespace, contextVersion: bound.contextVersion };
      setNamespace(bound.namespace); setContext(next);
      await queryClient.invalidateQueries({ queryKey: ["operations-center"] });
    } catch (error) {
      setSelected(previousSelection);
      setNamespace(context.namespace);
      setNotice({ tone: "error", text: error instanceof ApiError && error.status === 409 ? "This environment context is stale or unauthorized. No resources were changed." : error instanceof Error ? error.message : "Namespace binding failed." });
    }
  };

  const selectedIdentity = contexts.data?.contexts.find(item => item.contextName === context.identity);
  const ready = Boolean(context.namespace && context.contextVersion);

  return <div className="app-shell" data-testid="migration-app">
    <aside className="sidebar">
      <div className="brand"><span className="eyebrow">landlock-genprof</span><strong>Operations Center</strong><span>Migration foundation · v1</span></div>
      <nav aria-label="Primary navigation" className="primary-nav">
        {["overview", "workloads", "observations", "evidence", "proposals", "history", "attention", "health"].map(item => <button key={item} type="button" className={page === item ? "nav-item active" : "nav-item"} aria-current={page === item ? "page" : undefined} onClick={() => setPage(item)}>{item[0].toUpperCase() + item.slice(1)}{item === "health" && operational.data ? <span className="nav-state">● {operational.data.platform.status}</span> : null}</button>)}
      </nav>
    </aside>
    <header className="topbar"><div><span className="eyebrow">Bound operational context</span><h1>{page[0].toUpperCase() + page.slice(1)}</h1></div><span className={ready ? "status-pill healthy" : "status-pill unknown"} role="status">{ready ? "Context ready" : "Binding required"}</span></header>
    <main className="main-content">
      <section className="context-panel" aria-labelledby="context-heading">
        <div className="section-heading"><div><span className="eyebrow">Authority boundary</span><h2 id="context-heading">Environment</h2><p>Choose the server-authorized cluster, identity, and namespace for this tab.</p></div><button type="button" className="secondary-button" onClick={() => { void queryClient.invalidateQueries(); }}>Refresh</button></div>
        <div className="context-controls">
          <label>Cluster<select data-testid="context-cluster" value={selectedIdentity?.clusterIdentity ?? ""} onChange={event => { const next = contexts.data?.contexts.find(item => item.clusterIdentity === event.target.value); if (next) void changeIdentity(next.contextName); }} disabled={contexts.isLoading}><option value="">{contexts.isLoading ? "Loading clusters…" : "Select cluster"}</option>{Array.from(new Map((contexts.data?.contexts ?? []).map(item => [item.clusterIdentity, item])).values()).map(item => <option key={item.clusterIdentity} value={item.clusterIdentity}>{item.clusterDisplayName || item.clusterName || item.clusterIdentity}</option>)}</select></label>
          <label>Identity / context<select data-testid="context-identity" value={context.identity} onChange={event => void changeIdentity(event.target.value)} disabled={!contexts.data?.contexts.length}><option value="">Select identity</option>{(contexts.data?.contexts ?? []).filter(item => !selectedIdentity || item.clusterIdentity === selectedIdentity.clusterIdentity).map(item => <option key={item.contextName} value={item.contextName}>{item.contextName}</option>)}</select></label>
          <label>Namespace<select data-testid="context-namespace" value={namespace} onChange={event => void changeNamespace(event.target.value)} disabled={!namespaces.data?.namespaces?.length}><option value="">{namespaces.isLoading ? "Loading namespaces…" : "Select namespace"}</option>{(namespaces.data?.namespaces ?? []).map(item => <option key={item} value={item}>{item}</option>)}</select></label>
        </div>
        <div className="context-meta"><span><b>Cluster</b> <code>{context.cluster || "NOT_BOUND"}</code></span><span><b>Namespace</b> <code>{context.namespace || "NOT_BOUND"}</code></span><span><b>Session</b> <code>{context.sessionID || "NOT_BOUND"}</code></span><span><b>Version</b> <code>{context.contextVersion || "NOT_BOUND"}</code></span><span><b>Authenticated actor</b> <code>{operational.data?.context.actor.username || "NOT_BOUND"}</code></span></div>
      </section>
      {notice ? <div className={`notice ${notice.tone}`} role="alert">{notice.text}</div> : null}
      {page === "workloads" || page === "overview" ? <section aria-labelledby="workloads-heading"><div className="section-heading"><div><span className="eyebrow">Authoritative discovery</span><h2 id="workloads-heading">{page === "overview" ? "Operational overview" : "Workloads"}</h2><p>{ready ? "Resources are projected from the bound namespace." : "Bind an environment context to load namespace-scoped resources."}</p></div><span className="status-pill">{workloads.isFetching ? "Refreshing…" : `${workloads.data?.workloads.length ?? 0} workloads`}</span></div>{workloads.isError ? <div className="empty-state error">Workload discovery failed. No empty state is substituted for an authoritative read failure.</div> : !ready ? <div className="empty-state">No context selected.</div> : !selections.length ? <div className="empty-state">No supported workloads are visible in this authorized namespace.</div> : <div className="workload-grid">{selections.map(item => <article key={`${item.workloadUID}/${item.container}`} className={selected?.workloadUID === item.workloadUID && selected.container === item.container ? "workload-card selected" : "workload-card"} data-testid="workload-row"><div className="workload-card-top"><div><span className="eyebrow">{item.kind}</span><h3>{item.name}</h3><p>{context.namespace} · container <code>{item.container}</code></p></div><span className="status-pill healthy">Discovered</span></div><p className="technical">UID {item.workloadUID || "NOT_EXPOSED"}</p><button type="button" className="primary-button" onClick={() => { setSelected(item); setPage("workloads"); }}>Inspect workload</button></article>)}</div>}</section> : null}
      {page === "workloads" && selected ? <section className="detail-panel" aria-labelledby="detail-heading"><div className="section-heading"><div><span className="eyebrow">Exact selected resource</span><h2 id="detail-heading">{selected.kind} / {selected.name}</h2><p>{context.namespace} · {selected.container}</p></div><span className="status-pill">UID-bound read</span></div>{detail.isLoading ? <div className="loading-state" role="status">Loading authoritative workload object…</div> : detail.isError ? <div className="empty-state error">The authoritative workload detail could not be read. This view is not reconstructed locally.</div> : detail.data ? <div className="yaml-card"><div className="code-heading"><div><strong>Kubernetes YAML</strong><span>Safe backend projection · server metadata omitted by design</span></div><CopyButton value={detail.data.yaml} /></div><pre data-testid="workload-yaml" tabIndex={0}>{detail.data.yaml}</pre></div> : null}</section> : null}
      {page !== "workloads" && page !== "overview" ? <section className="empty-state" aria-labelledby="future-heading"><span className="eyebrow">Migration slice</span><h2 id="future-heading">{page[0].toUpperCase() + page.slice(1)} remains on the reference UI</h2><p>This route is intentionally preserved for the next vertical parity slice. No existing operational capability has been removed.</p></section> : null}
    </main>
  </div>;
}
