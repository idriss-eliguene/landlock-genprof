import type { AppContext, WorkloadSelection } from "../types";

export function contextQueryKey(context: AppContext) {
  return [context.cluster, context.namespace, context.contextVersion, context.sessionID];
}

export function selectionBelongsToContext(selection: WorkloadSelection | null, context: AppContext) {
  return Boolean(selection && context.cluster && context.namespace && context.contextVersion && context.sessionID);
}
