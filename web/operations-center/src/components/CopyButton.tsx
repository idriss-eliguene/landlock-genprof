import { useEffect, useState } from "react";

export function CopyButton({ value, label = "Copy YAML" }: { value: string; label?: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  useEffect(() => { setState("idle"); }, [value, label]);
  return <button type="button" className="secondary-button" onClick={async () => { try { await navigator.clipboard.writeText(value); setState("copied"); } catch { setState("failed"); } }} aria-live="polite">{state === "copied" ? "Copied" : state === "failed" ? "Copy unavailable" : label}</button>;
}
