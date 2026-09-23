import { describe, expect, it } from "vitest";
import { discardRestorationHint, readRestorationHint, RESTORATION_HINT_KEY, writeRestorationHint } from "./restoration";

function storage(): Storage {
  const values = new Map<string, string>();
  return {
    getItem: key => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
    clear: () => values.clear(),
    key: index => [...values.keys()][index] ?? null,
    get length() { return values.size; },
  };
}

describe("operational context restoration hints", () => {
  it("stores only a candidate locator, never authoritative session fields", () => {
    const target = storage();
    writeRestorationHint({ contextName: "security-reviewer", namespace: "payments" }, target);
    expect(readRestorationHint(target)).toEqual({ contextName: "security-reviewer", namespace: "payments" });
    expect(target.getItem(RESTORATION_HINT_KEY)).not.toContain("sessionID");
  });

  it("rejects malformed candidates and supports discard", () => {
    const target = storage();
    target.setItem(RESTORATION_HINT_KEY, JSON.stringify({ sessionID: "trusted", contextName: "" }));
    expect(readRestorationHint(target)).toBeNull();
    writeRestorationHint({ contextName: "developer" }, target);
    discardRestorationHint(target);
    expect(readRestorationHint(target)).toBeNull();
  });
});
