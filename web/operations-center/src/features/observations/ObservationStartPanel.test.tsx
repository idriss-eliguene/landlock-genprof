import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { WorkloadSelection } from "../../types";
import { ObservationStartPanel, observationStartBlockReason } from "./ObservationStartPanel";

const selection: WorkloadSelection = {
  group: "apps", kind: "Deployment", name: "api", container: "api", pod: "api-0",
  podUID: "pod-uid-1", workloadUID: "deployment-uid-1", imageIdentity: `sha256:${"a".repeat(64)}`,
};

describe("observation start controls", () => {
  it("requires authorization, current immutable identities, selected evidence, and no active run", () => {
    const base = { history: "complete" as const, selectionCurrent: true, hasIdentity: true, hasSources: true, active: false };
    expect(observationStartBlockReason({ ...base, permission: "checking" })).toMatch(/Checking/);
    expect(observationStartBlockReason({ ...base, permission: "denied" })).toMatch(/observation.operate/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", selectionCurrent: false })).toMatch(/changed/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", hasIdentity: false })).toMatch(/UIDs/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", hasSources: false })).toMatch(/source/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", active: true })).toMatch(/already active/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", history: "checking" })).toMatch(/Loading existing/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", history: "partial" })).toMatch(/incomplete/);
    expect(observationStartBlockReason({ ...base, permission: "allowed", history: "error" })).toMatch(/could not be checked/);
    expect(observationStartBlockReason({ ...base, permission: "allowed" })).toBe("");
  });

  it("presents workload/Pod binding and explains that executor attachment is authoritative", () => {
    const html = renderToStaticMarkup(createElement(ObservationStartPanel, {
      selection, namespace: "payments", permission: "allowed", history: "complete", selectionCurrent: true,
      active: false, pending: false, onStart: vi.fn(),
    }));
    expect(html).toContain("payments");
    expect(html).toContain("pod-uid-1");
    expect(html).toContain("deployment-uid-1");
    expect(html).toContain("executor confirms source attachment");
    expect(html).toContain("name=\"durationSeconds\"");
    expect(html).toContain("filesystem");
    expect(html).toContain("capabilities");
    expect(html).not.toContain("disabled=\"\"");
  });

  it("disables start when permission is denied or the workload selection is stale", () => {
    for (const [permission, selectionCurrent] of [["denied", true], ["allowed", false]] as const) {
      const html = renderToStaticMarkup(createElement(ObservationStartPanel, {
        selection, namespace: "payments", permission, history: "complete", selectionCurrent,
        active: false, pending: false, onStart: vi.fn(),
      }));
      expect(html).toContain("disabled=\"\"");
      expect(html).toContain("observation-start-blocked");
    }
  });

  it("does not start another capture before complete observation history is available", () => {
    for (const history of ["checking", "partial", "error"] as const) {
      const html = renderToStaticMarkup(createElement(ObservationStartPanel, {
        selection, namespace: "payments", permission: "allowed", history,
        selectionCurrent: true, active: false, pending: false, onStart: vi.fn(),
      }));
      expect(html).toContain("disabled=\"\"");
      expect(html).toContain("Existing captures: " + history);
    }
  });
});
