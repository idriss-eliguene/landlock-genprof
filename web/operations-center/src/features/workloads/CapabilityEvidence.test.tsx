import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CapabilityEvidence } from "./CapabilityEvidence";

describe("capability evidence reviewer presentation", () => {
  it("renders only recorded per-capability Observation links", () => {
    const html = renderToStaticMarkup(<CapabilityEvidence capabilities={["CAP_CHOWN", "CAP_NET_ADMIN"]} attribution={[
      { capability: "CAP_CHOWN", state: "ATTRIBUTED", observationIDs: ["obs-a"] },
      { capability: "CAP_NET_ADMIN", state: "ATTRIBUTED", observationIDs: ["obs-b", "obs-c"] },
    ]} onObservation={vi.fn()} />);
    expect(html).toContain("CAP_CHOWN");
    expect(html).toContain("CAP_NET_ADMIN");
    expect(html).toContain("Recorded in");
    expect(html).toContain("obs-a");
    expect(html).toContain("obs-b");
    expect(html).toContain("obs-c");
    expect(html).toContain("do not establish causality");
  });

  it("marks historical or unavailable attribution UNKNOWN instead of linking aggregate provenance", () => {
    const html = renderToStaticMarkup(<CapabilityEvidence capabilities={["CAP_SYS_ADMIN"]} onObservation={vi.fn()} />);
    expect(html).toContain("Evidence attribution: UNKNOWN");
    expect(html).not.toContain("obs-");
  });
});
