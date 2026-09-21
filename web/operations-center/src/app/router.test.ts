import { describe, expect, it } from "vitest";
import { pageForLocation, pageLabels, pathForPage, workloadLocatorFromLocation } from "./router";

describe("Operations Center routes", () => {
  it("maps the React base and level-one routes", () => {
    expect(pageForLocation("/next/")).toBe("home");
    expect(pageForLocation("/next/proposals")).toBe("proposals");
    expect(pageForLocation("/next/health")).toBe("health");
    expect(pageForLocation("/next/unknown")).toBe("home");
    expect(pageLabels.proposals).toBe("Proposals & Governance");
  });

  it("keeps observation and evidence reachable as transitional routes", () => {
    expect(pageForLocation("/next/observations")).toBe("observations");
    expect(pageForLocation("/next/evidence")).toBe("evidence");
    expect(pathForPage("workloads")).toMatch(/\/workloads$/);
  });

  it("recognizes a workload dossier locator without treating it as identity", () => {
    expect(pageForLocation("/next/workloads/payments/apps/Deployment/api/api")).toBe("workloads");
    expect(workloadLocatorFromLocation("/next/workloads/payments/apps/Deployment/api/api")).toEqual({ namespace: "payments", group: "apps", kind: "Deployment", name: "api", container: "api" });
  });
});
