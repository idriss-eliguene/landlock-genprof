import { useCallback, useEffect, useState } from "react";
import type { WorkloadLocator } from "../features/workloads/model";

export type AppPage = "home" | "workloads" | "observations" | "evidence" | "proposals" | "history" | "attention" | "health";

const pageByPath: Record<string, AppPage> = {
  "/": "home",
  "/home": "home",
  "/workloads": "workloads",
  "/observations": "observations",
  "/evidence": "evidence",
  "/proposals": "proposals",
  "/history": "history",
  "/attention": "attention",
  "/health": "health",
};

export const pageLabels: Record<AppPage, string> = {
  home: "Home",
  workloads: "Workloads",
  observations: "Observations",
  evidence: "Evidence",
  proposals: "Proposals & Governance",
  history: "History",
  attention: "Attention",
  health: "Health",
};

function basePath(): string {
	return "";
}

export function pathForPage(page: AppPage): string {
  const suffix = page === "home" ? "/" : `/${page}`;
  return `${basePath()}${suffix}`;
}

export function workloadLocatorFromLocation(pathname = window.location.pathname): WorkloadLocator | undefined {
  const parts = pathname.split("/").filter(Boolean).map(decodeURIComponent);
  if (parts.length !== 6 || parts[0] !== "workloads") return undefined;
  return { namespace: parts[1], group: parts[2] === "_core" ? "" : parts[2], kind: parts[3], name: parts[4], container: parts[5] };
}

export function pathForWorkload(locator: WorkloadLocator): string {
  const group = locator.group || "_core";
  return `${basePath()}/workloads/${[locator.namespace, group, locator.kind, locator.name, locator.container].map(encodeURIComponent).join("/")}`;
}

export function pageForLocation(pathname = window.location.pathname): AppPage {
  if (workloadLocatorFromLocation(pathname)) return "workloads";
  return pageByPath[pathname] || "home";
}

export function useAppRouter(): [AppPage, (page: AppPage) => void, string] {
  const [page, setPage] = useState<AppPage>(() => pageForLocation());
  const [pathname, setPathname] = useState(() => window.location.pathname);
  useEffect(() => {
    const onPopState = () => { setPage(pageForLocation()); setPathname(window.location.pathname); };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);
  const navigate = useCallback((next: AppPage) => {
    const nextPath = pathForPage(next);
    if (window.location.pathname !== nextPath) { window.history.pushState({}, "", nextPath + window.location.search); setPathname(nextPath); }
    setPage(next);
  }, []);
  return [page, navigate, pathname];
}
