import { describe, expect, it } from "vitest";
import { contextQueryKey, selectionBelongsToContext } from "./context";

describe("authoritative context state", () => {
  it("includes every authority component in query identity", () => {
    expect(contextQueryKey({ cluster: "cluster-a", namespace: "payments", contextVersion: 7, sessionID: "session-a", identity: "developer" })).toEqual(["cluster-a", "payments", 7, "session-a"]);
  });

  it("does not treat an unbound selection as context-valid", () => {
    expect(selectionBelongsToContext({ group: "apps", kind: "Deployment", name: "api", container: "api", workloadUID: "uid" }, { cluster: "", namespace: "", contextVersion: 0, sessionID: "", identity: "" })).toBe(false);
  });
});
