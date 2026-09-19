import { useState } from "react";

export function CopyButton({ value }: { value: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  return <button type="button" className="secondary-button" onClick={async () => { try { await navigator.clipboard.writeText(value); setState("copied"); } catch { setState("failed"); } }} aria-live="polite">{state === "copied" ? "Copied" : state === "failed" ? "Copy unavailable" : "Copy YAML"}</button>;
}
