import { describe, expect, it } from "vitest";
import { pathForWorkload, resolutionAuthorityKey, resolvedForAuthority, selectionsFromResponse, selectionForLocator } from "./model";

describe("workload dossier identity", () => {
  it("handles a valid empty workload collection without throwing", () => {
    expect(selectionsFromResponse({ namespace: "empty", complete: true, workloads: [] })).toEqual([]);
  });

  const response = { namespace: "payments", workloads: [{ uid: "uid-a", target: { group: "apps", kind: "Deployment", name: "api" }, pods: [{ name: "api-pod", uid: "pod-a", containers: [{ name: "api", supportedTarget: true, target: { workload: { group: "apps", kind: "Deployment", name: "api" } } }] }] }] };

  it("resolves a locator to the server-discovered workload UID", () => {
    expect(selectionForLocator(response, { namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" })?.workloadUID).toBe("uid-a");
    expect(selectionForLocator(response, { namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "sidecar" })).toBeUndefined();
  });

  it("does not resolve a locator from another namespace", () => {
    expect(selectionForLocator(response, { namespace: "security", group: "apps", kind: "Deployment", name: "api", container: "api" })).toBeUndefined();
  });

  it("resolves a recreated locator to the new UID instead of carrying old identity", () => {
    const recreated = { ...response, workloads: [{ ...response.workloads[0], uid: "uid-b" }] };
    expect(selectionForLocator(response, { namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" })?.workloadUID).toBe("uid-a");
    expect(selectionForLocator(recreated, { namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" })?.workloadUID).toBe("uid-b");
    expect(selectionForLocator(recreated, { namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" })?.workloadUID).not.toBe("uid-a");
  });

  it("keeps routes as locators and does not encode UID", () => {
  expect(pathForWorkload({ namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" })).toBe("/workloads/payments/apps/Deployment/api/api");
  });

  it("invalidates a resolved identity when only the authority session changes", () => {
    const authorityA = resolutionAuthorityKey(["cluster", "payments", 1, "session-a"]);
    const authorityB = resolutionAuthorityKey(["cluster", "payments", 2, "session-b"]);
    expect(authorityA).not.toBe(authorityB);
    expect(resolvedForAuthority(authorityA, authorityB)).toBe(false);
  });

  it("does not render an identity after failed re-resolution", () => {
    const authority = resolutionAuthorityKey(["cluster", "payments", 2, "session-b"]);
    expect(resolvedForAuthority(null, authority)).toBe(false);
  });

  it("rejects a late response from the previous authority", () => {
    const authorityA = resolutionAuthorityKey(["cluster", "payments", 1, "session-a"]);
    const authorityB = resolutionAuthorityKey(["cluster", "payments", 2, "session-b"]);
    expect(resolvedForAuthority(authorityA, authorityB)).toBe(false);
  });
});
