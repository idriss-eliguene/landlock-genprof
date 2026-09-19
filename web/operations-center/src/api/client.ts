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
} from "../types";

export class ApiError extends Error {
  constructor(public status: number, public body: unknown) {
    super(typeof body === "object" && body && "reason" in body ? String(body.reason) : `HTTP ${status}`);
  }
}

function normalizeObservation<T extends { sources?: unknown }>(observation: T) {
  return { ...observation, sources: Array.isArray(observation.sources) ? observation.sources : [] } as T & { sources: NonNullable<T["sources"]> };
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
};
