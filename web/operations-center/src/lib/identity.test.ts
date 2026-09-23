import { describe, expect, it } from "vitest";
import { canonicalImageIdentity } from "./identity";

describe("canonical image identity", () => {
  it("normalizes a runtime image reference to the Observation digest identity", () => {
    expect(canonicalImageIdentity("docker.io/library/nginx@sha256:" + "a".repeat(64))).toBe("sha256:" + "a".repeat(64));
  });

  it("does not invent an identity for an unqualified runtime value", () => {
    expect(canonicalImageIdentity("docker.io/library/nginx:latest")).toBeUndefined();
  });
});
