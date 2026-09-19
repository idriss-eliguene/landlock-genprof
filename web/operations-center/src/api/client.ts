import type {
  AppContext,
  BoundCapabilities,
  ContextRecord,
  EnvironmentSession,
  NamespaceDiscovery,
  OperationalContext,
  WorkloadDetail,
  WorkloadResponse,
  WorkloadSelection,
  ObservationListResponse,
  ObservationRead,
  ObservationStartResponse,
  ObservationStopResponse,
  ProposalListResponse,
  ProposalRead,
  ProposalGenerationResponse,
  GovernanceResponse,
  HistoryResponse,
  EnvironmentProjectionResponse,
  OverviewProjectionResponse,
  SphmReport,
} from "../types";

export class ApiError extends Error {
  constructor(public status: number, public body: unknown) {
    super(typeof body === "object" && body && "reason" in body ? String(body.reason) : `HTTP ${status}`);
  }
}

function normalizeObservation<T extends { sources?: unknown }>(observation: T) {
  return { ...observation, sources: Array.isArray(observation.sources) ? observation.sources : [] } as T & { sources: NonNullable<T["sources"]> };
}

function value<T>(record: Record<string, unknown>, camel: string, pascal: string, fallback: T): T {
  return (record[camel] ?? record[pascal] ?? fallback) as T;
}

function optionalString(valueToRead: unknown): string | undefined {
  return typeof valueToRead === "string" && valueToRead.length > 0 ? valueToRead : undefined;
}

export function normalizeHistory(response: HistoryResponse): HistoryResponse {
  const raw = response as unknown as Record<string, unknown>;
  const rawHistory = value<Record<string, unknown>>(raw, "history", "History", {});
  const normalizeEvent = (input: unknown) => {
    const event = (input || {}) as Record<string, unknown>;
    const source = value<Record<string, unknown>>(event, "sourceRef", "SourceRef", {});
    const related = value<Record<string, unknown> | undefined>(event, "relatedRef", "RelatedRef", undefined);
    const ref = (item: Record<string, unknown>) => ({
      kind: String(value(item, "kind", "Kind", "")),
      namespace: optionalString(value(item, "namespace", "Namespace", "")),
      name: optionalString(value(item, "name", "Name", "")),
      uid: optionalString(value(item, "uid", "UID", "")),
    });
    return {
      kind: String(value(event, "kind", "Kind", "HISTORY_FACT")),
      sourceRef: ref(source),
      relatedRef: related ? ref(related) : undefined,
      timestamp: optionalString(value(event, "timestamp", "Timestamp", "")),
      temporalClass: String(value(event, "temporalClass", "TemporalClass", "UNTIMESTAMPED_UNORDERED")),
      claimTier: String(value(event, "claimTier", "ClaimTier", "BOOKKEEPING")),
      detailCode: String(value(event, "detailCode", "DetailCode", "")),
    };
  };
  const events = (value<unknown[]>(rawHistory, "timestampedEvents", "TimestampedEvents", []) || []).map(normalizeEvent);
  const facts = (value<unknown[]>(rawHistory, "untimestampedFacts", "UntimestampedFacts", []) || []).map(normalizeEvent);
  return {
    history: {
      timestampedEvents: events,
      untimestampedFacts: facts,
      limitations: value<string[]>(rawHistory, "limitations", "Limitations", []),
      totalCount: Number(value(rawHistory, "totalCount", "TotalCount", events.length + facts.length)),
      truncated: Boolean(value(rawHistory, "truncated", "Truncated", false)),
    },
    limitation: String(value(raw, "limitation", "Limitation", "BEST_EFFORT_MULTI_OBJECT_READ")),
    projectionDiagnostics: value(raw, "projectionDiagnostics", "ProjectionDiagnostics", undefined),
  };
}

export function normalizeEnvironmentProjection(response: EnvironmentProjectionResponse): EnvironmentProjectionResponse {
  const raw = response as unknown as Record<string, unknown>;
  const rawItems = value<unknown[]>(raw, "items", "Items", []);
  const items = rawItems.map(input => {
    const item = (input || {}) as Record<string, unknown>;
    const rawSubject = value<Record<string, unknown>>(item, "subject", "Subject", {});
    const subject = {
      scope: optionalString(value(rawSubject, "scope", "Scope", "")),
      target: optionalString(value(rawSubject, "target", "Target", "")),
      container: optionalString(value(rawSubject, "container", "Container", "")),
      imageIdentity: optionalString(value(rawSubject, "imageIdentity", "ImageIdentity", "")),
      binaryPath: optionalString(value(rawSubject, "binaryPath", "BinaryPath", "")),
    };
    const rawAttention = value<unknown[]>(item, "attention", "Attention", []);
    const attention = rawAttention.map(inputReason => {
      const reason = (inputReason || {}) as Record<string, unknown>;
      const rawReasonSubject = value<Record<string, unknown> | undefined>(reason, "subject", "Subject", undefined);
      const refList = (camel: string, pascal: string) => value<unknown[]>(reason, camel, pascal, []).map(inputRef => {
        const ref = (inputRef || {}) as Record<string, unknown>;
        return { namespace: optionalString(value(ref, "namespace", "Namespace", "")), name: optionalString(value(ref, "name", "Name", "")), uid: optionalString(value(ref, "uid", "UID", "")) };
      });
      return {
        category: String(value(reason, "category", "Category", "ATTENTION")),
        subject: rawReasonSubject ? { scope: optionalString(value(rawReasonSubject, "scope", "Scope", "")), target: optionalString(value(rawReasonSubject, "target", "Target", "")), container: optionalString(value(rawReasonSubject, "container", "Container", "")), imageIdentity: optionalString(value(rawReasonSubject, "imageIdentity", "ImageIdentity", "")), binaryPath: optionalString(value(rawReasonSubject, "binaryPath", "BinaryPath", "")) } : undefined,
        evidenceRefs: value<string[]>(reason, "evidenceRefs", "EvidenceRefs", []),
        observationRefs: value<string[]>(reason, "observationRefs", "ObservationRefs", []),
        proposalRefs: refList("proposalRefs", "ProposalRefs"),
        governanceRefs: refList("governanceRefs", "GovernanceRefs"),
        applicationRefs: refList("applicationRefs", "ApplicationRefs"),
        explanationCode: String(value(reason, "explanationCode", "ExplanationCode", "")) || undefined,
      };
    });
    return { subject, attention };
  });
  return { items, totalCount: Number(value(raw, "totalCount", "TotalCount", items.length)), truncated: Boolean(value(raw, "truncated", "Truncated", false)), unattributedFailedObservationCount: Number(value(raw, "unattributedFailedObservationCount", "UnattributedFailedObservationCount", 0)), limitation: String(value(raw, "limitation", "Limitation", "BEST_EFFORT_MULTI_OBJECT_READ")), projectionDiagnostics: value(raw, "projectionDiagnostics", "ProjectionDiagnostics", undefined) };
}

async function request<T>(path: string, init?: RequestInit, context?: AppContext): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set("Accept", "application/json");
  if (init?.body) headers.set("Content-Type", "application/json");
  if (context && context.sessionID && context.contextVersion && context.namespace) {
    headers.set("X-Environment-Session", context.sessionID);
    headers.set("X-Environment-Context-Version", String(context.contextVersion));
    headers.set("X-Environment-Namespace", context.namespace);
  }
  const response = await fetch(path, { ...init, headers });
  const text = await response.text();
  let body: unknown = {};
  try { body = text ? JSON.parse(text) : {}; } catch { body = { reason: text }; }
  if (!response.ok) throw new ApiError(response.status, body);
  return body as T;
}

export const api = {
  contexts: () => request<{ contexts: ContextRecord[] }>("/api/v09/environments"),
  openContext: (contextName: string) => request<EnvironmentSession>("/api/v09/environments", {
    method: "POST", body: JSON.stringify({ contextName }),
  }),
  namespaces: (sessionID: string) => request<NamespaceDiscovery>(`/api/v09/environments/${encodeURIComponent(sessionID)}/namespaces`),
  bindNamespace: (sessionID: string, namespace: string, context: AppContext) => request<BoundCapabilities>(
    `/api/v09/environments/${encodeURIComponent(sessionID)}/capabilities?namespace=${encodeURIComponent(namespace)}`,
    undefined, context,
  ),
  operationalContext: (context: AppContext) => request<OperationalContext>("/api/v08/operations-context", undefined, context),
  workloads: (context: AppContext) => request<WorkloadResponse>("/api/workloads", undefined, context),
  workloadDetail: (selection: WorkloadSelection, context: AppContext) => {
    const query = new URLSearchParams({
      group: selection.group, kind: selection.kind, name: selection.name,
      container: selection.container, workloadUID: selection.workloadUID,
    });
    if (selection.imageIdentity) query.set("imageIdentity", selection.imageIdentity);
    return request<WorkloadDetail>(`/api/workloads/detail?${query}`, undefined, context);
  },
  observations: async (selection: WorkloadSelection, context: AppContext) => {
    const query = new URLSearchParams({
      group: selection.group, kind: selection.kind, name: selection.name,
      container: selection.container, workloadUID: selection.workloadUID,
    });
    if (selection.imageIdentity) query.set("imageIdentity", selection.imageIdentity);
    const response = await request<ObservationListResponse>(`/api/observations?${query}`, undefined, context);
    return { ...response, items: (response.items || []).map(normalizeObservation) };
  },
  observation: async (id: string, context: AppContext) => normalizeObservation(await request<ObservationRead>(`/api/observations/${encodeURIComponent(id)}`, undefined, context)),
  startObservation: (selection: WorkloadSelection, context: AppContext) => request<ObservationStartResponse>("/api/observations/start", {
    method: "POST",
    body: JSON.stringify({ namespace: context.namespace, pod: selection.pod || selection.name, container: selection.container, sources: ["capabilities"], duration: 60_000_000_000 }),
  }, context),
  stopObservation: (id: string, context: AppContext) => request<ObservationStopResponse>("/api/observations/stop", {
    method: "POST", body: JSON.stringify({ namespace: context.namespace, observationID: id }),
  }, context),
  proposals: (selection: WorkloadSelection, context: AppContext) => {
    const query = new URLSearchParams({ group: selection.group, kind: selection.kind, name: selection.name, container: selection.container, workloadUID: selection.workloadUID });
    if (selection.imageIdentity) query.set("imageIdentity", selection.imageIdentity);
    return request<ProposalListResponse>(`/api/proposals?${query}`, undefined, context);
  },
  proposal: (name: string, context: AppContext) => request<ProposalRead>(`/api/proposals/${encodeURIComponent(name)}`, undefined, context),
  generateProposal: (observationID: string, context: AppContext) => request<ProposalGenerationResponse>("/api/observations/generate-proposal", {
    method: "POST", body: JSON.stringify({ namespace: context.namespace, observationID, proposalName: `observation-${observationID}` }),
  }, context),
  governance: (name: string, operation: "review" | "approve" | "reject" | "apply", body: { expectedResourceVersion: string; expectedDigest?: string; reason?: string }, context: AppContext) => request<GovernanceResponse>(`/api/governance/proposals/${encodeURIComponent(name)}/${operation}`, {
    method: "POST", body: JSON.stringify(body),
  }, context),
  history: (selection: WorkloadSelection, context: AppContext) => {
    const query = new URLSearchParams({ scope: "CONTAINER", target: `${selection.kind}/${selection.name}`, container: selection.container, imageIdentity: selection.imageIdentity || "", binaryPath: "", limit: "100" });
    return request<HistoryResponse>(`/api/v08/history?${query}`, undefined, context).then(normalizeHistory);
  },
  environmentProjection: (context: AppContext) => request<EnvironmentProjectionResponse>("/api/v08/environment?limit=100", undefined, context).then(normalizeEnvironmentProjection),
  health: (context: AppContext) => request<SphmReport>("/api/health", undefined, context),
  overview: async (context: AppContext) => {
    const response = await request<OverviewProjectionResponse>("/api/v08/overview?limit=100", undefined, context);
    return {
      ...response,
      environment: normalizeEnvironmentProjection(response.environment),
      history: normalizeHistory(response.history),
    };
  },
};
