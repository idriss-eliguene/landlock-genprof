import type { AppContext, ContextRecord } from "../types";

export type OperationalContextState = "NOT READY" | "SELECTING" | "RESOLVING" | "READY";

export function contextsForCluster(contexts: ContextRecord[], clusterIdentity: string): ContextRecord[] {
  return clusterIdentity ? contexts.filter(context => context.clusterIdentity === clusterIdentity) : contexts;
}

export function operationalContextState(context: AppContext): OperationalContextState {
  if (!context.identity) return "NOT READY";
  if (!context.sessionID) return "SELECTING";
  if (!context.namespace || !context.contextVersion) return "RESOLVING";
  return "READY";
}

export function namespaceControlEnabled(context: AppContext, discoveryMode: string | undefined, namespaces: string[]): boolean {
  if (!context.sessionID) return false;
  return discoveryMode === "EXPLICIT_ONLY" || namespaces.length > 0;
}
