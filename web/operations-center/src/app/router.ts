import { useCallback, useEffect, useState } from "react";

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
  if (typeof window === "undefined") return "/next";
  const path = window.location.pathname;
  return path.startsWith("/next") ? "/next" : "";
}

export function pathForPage(page: AppPage): string {
  const suffix = page === "home" ? "/" : `/${page}`;
  return `${basePath()}${suffix}`;
}

export function pageForLocation(pathname = window.location.pathname): AppPage {
  const path = pathname.startsWith("/next") ? pathname.slice("/next".length) || "/" : pathname;
  return pageByPath[path] || "home";
}

export function useAppRouter(): [AppPage, (page: AppPage) => void] {
  const [page, setPage] = useState<AppPage>(() => pageForLocation());
  useEffect(() => {
    const onPopState = () => setPage(pageForLocation());
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);
  const navigate = useCallback((next: AppPage) => {
    const nextPath = pathForPage(next);
    if (window.location.pathname !== nextPath) window.history.pushState({}, "", nextPath + window.location.search);
    setPage(next);
  }, []);
  return [page, navigate];
}
