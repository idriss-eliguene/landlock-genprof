export const RESTORATION_HINT_KEY = "landlock-genprof.operations-context.restore-candidate";

export interface RestorationHint {
  contextName: string;
  namespace?: string;
}

export function readRestorationHint(storage: Storage | undefined = typeof window === "undefined" ? undefined : window.sessionStorage): RestorationHint | null {
  if (!storage) return null;
  try {
    const raw = storage.getItem(RESTORATION_HINT_KEY);
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<RestorationHint>;
    if (!value || typeof value.contextName !== "string" || !value.contextName.trim()) return null;
    return { contextName: value.contextName, namespace: typeof value.namespace === "string" ? value.namespace : undefined };
  } catch {
    return null;
  }
}

/** A locator hint only. It deliberately excludes session, version, actor, capabilities and cluster identity. */
export function writeRestorationHint(hint: RestorationHint, storage: Storage | undefined = typeof window === "undefined" ? undefined : window.sessionStorage): void {
  if (!storage) return;
  storage.setItem(RESTORATION_HINT_KEY, JSON.stringify(hint));
}

export function discardRestorationHint(storage: Storage | undefined = typeof window === "undefined" ? undefined : window.sessionStorage): void {
  storage?.removeItem(RESTORATION_HINT_KEY);
}
