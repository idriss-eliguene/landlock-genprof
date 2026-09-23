import { describe, expect, it } from "vitest";
import { pageForLocation, pageLabels, pathForPage, workloadLocatorFromLocation } from "./router";

describe("Operations Center routes", () => {
  it("maps the React base and level-one routes", () => {
    expect(pageForLocation("/")).toBe("home");
    expect(pageForLocation("/proposals")).toBe("proposals");
    expect(pageForLocation("/health")).toBe("health");
    expect(pageForLocation("/unknown")).toBe("home");
    expect(pageLabels.proposals).toBe("Proposals & Governance");
  });

  it("keeps observation and evidence reachable as transitional routes", () => {
    expect(pageForLocation("/observations")).toBe("observations");
    expect(pageForLocation("/evidence")).toBe("evidence");
    expect(pathForPage("workloads")).toMatch(/\/workloads$/);
  });

  it("recognizes a workload dossier locator without treating it as identity", () => {
    expect(pageForLocation("/workloads/payments/apps/Deployment/api/api")).toBe("workloads");
    expect(workloadLocatorFromLocation("/workloads/payments/apps/Deployment/api/api")).toEqual({ namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" });
  });
});
