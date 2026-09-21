import type { WorkloadRecord, WorkloadResponse, WorkloadSelection } from "../../types";
import { canonicalImageIdentity } from "../../lib/identity";

export interface WorkloadLocator {
  namespace: string;
  group: string;
  kind: string;
  name: string;
  container: string;
}

export function resolutionAuthorityKey(parts: readonly unknown[]): string {
  return parts.map(String).join("\u001f");
}

export function resolvedForAuthority(resolvedKey: string | null, currentKey: string): boolean {
  return Boolean(resolvedKey && resolvedKey === currentKey);
}

export function selectionsFromResponse(response: WorkloadResponse): WorkloadSelection[] {
  return response.workloads.flatMap(workload => (workload.pods ?? []).flatMap(pod => (pod.containers ?? []).flatMap(container => {
    const target = container.target?.workload;
    if (!target || !container.supportedTarget) return [];
    return [{
      group: target.group,
      kind: target.kind,
      name: target.name,
      container: container.name,
      pod: pod.name,
      workloadUID: workload.uid ?? pod.uid ?? "",
      imageIdentity: canonicalImageIdentity(container.runtime?.imageID),
    }];
  })));
}

export function selectionForLocator(response: WorkloadResponse, locator: WorkloadLocator): WorkloadSelection | undefined {
  if (!response.namespace || response.namespace !== locator.namespace) return undefined;
  return selectionsFromResponse(response).find(item =>
    item.group === locator.group && item.kind === locator.kind && item.name === locator.name && item.container === locator.container && Boolean(item.workloadUID),
  );
}

export function pathForWorkload(locator: WorkloadLocator, base = ""): string {
  const group = locator.group || "_core";
  return `${base}/workloads/${[locator.namespace, group, locator.kind, locator.name, locator.container].map(encodeURIComponent).join("/")}`;
}

export function locatorFromWorkload(selection: WorkloadSelection, namespace: string): WorkloadLocator {
  return { namespace, group: selection.group, kind: selection.kind, name: selection.name, container: selection.container };
}

export function workloadRecordCount(response?: WorkloadResponse): number {
  return response?.workloads?.length ?? 0;
}

export function workloadHasUID(record: WorkloadRecord): boolean {
  return Boolean(record.uid);
}
