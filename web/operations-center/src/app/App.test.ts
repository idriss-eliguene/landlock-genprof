import { describe, expect, it } from "vitest";
import { evidenceVerdict, factNames } from "./App";
import type { ObservationSource } from "../types";

function source(overrides: Partial<ObservationSource>): ObservationSource {
  return {
    name: "source",
    attributionState: "COMPLETED",
    evidenceState: "AVAILABLE",
    attributedCount: 0,
    excludedCount: 0,
    backendHealthConfirmed: true,
    sourceAttachedForBoundWindow: true,
    flushConfirmed: true,
    ...overrides,
  };
}

// M10.4 Finding A: evidenceVerdict replaces sources[0], which could mask a
// genuinely UNKNOWN source behind a co-existing AVAILABLE source purely
// because of array order.
describe("evidenceVerdict", () => {
  it("returns UNKNOWN for no sources", () => {
    expect(evidenceVerdict([])).toBe("UNKNOWN");
  });

  it("lets UNKNOWN dominate regardless of source order", () => {
    const sources = [source({ name: "aardvark", evidenceState: "AVAILABLE" }), source({ name: "zzz", evidenceState: "UNKNOWN" })];
    expect(evidenceVerdict(sources)).toBe("UNKNOWN");
  });

  it("lets AVAILABLE dominate EMPTY", () => {
    const sources = [source({ evidenceState: "EMPTY" }), source({ evidenceState: "AVAILABLE" })];
    expect(evidenceVerdict(sources)).toBe("AVAILABLE");
  });

  it("is EMPTY only when every source is EMPTY", () => {
    const sources = [source({ evidenceState: "EMPTY" }), source({ evidenceState: "EMPTY" })];
    expect(evidenceVerdict(sources)).toBe("EMPTY");
  });
});

// M10.4 Finding I: factNames previously read only facts.capabilities and
// assumed a lowercase-first wire shape. The real wire shape is Go's
// internal/observation/domain.NormalizedFacts marshaled with no json tags,
// i.e. PascalCase (Filesystem/Exec/NetworkConnect/NetworkBind/Capabilities,
// and Path/Permissions/Port/Direction/Name within each entry) -- this exact
// mismatch caused a real regression during M10.4 (the equivalent Workbench
// fix initially used lowercase-only keys and broke the real browser
// qualification; see cmd/landlock-genprof/workbench_ui.go's field() dual-
// casing pattern, mirrored here by at()).
describe("factNames", () => {
  it("returns nothing when facts is absent", () => {
    expect(factNames(source({ facts: undefined }))).toEqual([]);
  });

  it("surfaces filesystem, exec, network, and capability facts together from the real PascalCase wire shape", () => {
    const facts = {
      Filesystem: [{ Path: "/etc/passwd", Permissions: ["read"] }],
      Exec: [{ Path: "/usr/bin/curl" }],
      NetworkConnect: [{ Port: 443, Direction: "egress" }],
      NetworkBind: [{ Port: 8080, Direction: "ingress" }],
      Capabilities: [{ Name: "NET_BIND_SERVICE" }],
    };
    const names = factNames(source({ facts }));
    expect(names).toEqual([
      "Filesystem: /etc/passwd (read)",
      "Exec: /usr/bin/curl",
      "Network connect: egress port 443",
      "Network bind: ingress port 8080",
      "Capabilities: NET_BIND_SERVICE",
    ]);
  });

  it("also accepts a lowercase-first shape, in case the backend ever adds explicit json tags", () => {
    const names = factNames(source({ facts: { capabilities: [{ name: "SYS_ADMIN" }] } }));
    expect(names).toEqual(["Capabilities: SYS_ADMIN"]);
  });
});
