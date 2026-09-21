import { describe, expect, it } from "vitest";
import { pageForLocation, pageLabels, pathForPage } from "./router";

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
});
