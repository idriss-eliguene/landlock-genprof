import { describe, expect, it } from "vitest";
import { contextsForCluster, namespaceControlEnabled, operationalContextState } from "./operationalContext";
import type { AppContext, ContextRecord } from "../types";

const contexts: ContextRecord[] = [
  { contextName: "core-reviewer", clusterIdentity: "cluster-core", clusterDisplayName: "Core" },
  { contextName: "core-developer", clusterIdentity: "cluster-core", clusterDisplayName: "Core" },
  { contextName: "platform-reviewer", clusterIdentity: "cluster-platform", clusterDisplayName: "Platform" },
];
const empty: AppContext = { cluster: "", namespace: "", sessionID: "", contextVersion: 0, identity: "" };

describe("global operational context cascade", () => {
  it("filters contexts by the server-derived durable cluster identity", () => {
    expect(contextsForCluster(contexts, "cluster-core").map(item => item.contextName)).toEqual(["core-reviewer", "core-developer"]);
    expect(contextsForCluster(contexts, "cluster-platform").map(item => item.contextName)).toEqual(["platform-reviewer"]);
    expect(contextsForCluster(contexts, "")).toHaveLength(3);
  });

  it("does not report Ready before the authoritative tuple is complete", () => {
    expect(operationalContextState(empty)).toBe("NOT READY");
    expect(operationalContextState({ ...empty, identity: "core-reviewer" })).toBe("SELECTING");
    expect(operationalContextState({ ...empty, identity: "core-reviewer", sessionID: "session-1" })).toBe("RESOLVING");
    expect(operationalContextState({ ...empty, identity: "core-reviewer", sessionID: "session-1", namespace: "payments", contextVersion: 3 })).toBe("READY");
  });

  it("gates namespace controls on the bound session without enabling enumeration in explicit-only mode", () => {
    expect(namespaceControlEnabled(empty, "DISCOVERED", ["payments"])).toBe(false);
    expect(namespaceControlEnabled({ ...empty, sessionID: "session-1" }, "DISCOVERED", [])).toBe(false);
    expect(namespaceControlEnabled({ ...empty, sessionID: "session-1" }, "EXPLICIT_ONLY", [])).toBe(true);
  });
});
