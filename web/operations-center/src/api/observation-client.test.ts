import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./client";
import type { AppContext, WorkloadSelection } from "../types";

const context: AppContext = { cluster: "cluster-uid", namespace: "payments", sessionID: "session-1", contextVersion: 2, identity: "developer" };
const selection: WorkloadSelection = { group: "apps", kind: "Deployment", name: "api", container: "api", pod: "api-0", podUID: "pod-uid", workloadUID: "deployment-uid", imageIdentity: `sha256:${"a".repeat(64)}` };

afterEach(() => vi.unstubAllGlobals());

describe("authorized observation request client", () => {
  it("sends explicit scope and the immutable workload/Pod snapshot", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ observationID: "obs-1", executionState: "REQUESTED" }), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    await api.startObservation(selection, context, ["filesystem", "capabilities"], 120);
    const body = JSON.parse(fetch.mock.calls[0][1].body as string);
    expect(body).toMatchObject({
      namespace: "payments", pod: "api-0", container: "api",
      sources: ["filesystem", "capabilities"], duration: 120_000_000_000,
      expectedTarget: { group: "apps", kind: "Deployment", name: "api", workloadUID: "deployment-uid", podUID: "pod-uid", imageDigest: selection.imageIdentity },
    });
    expect(fetch.mock.calls[0][1].headers.get("X-Environment-Namespace")).toBe("payments");
  });
});
