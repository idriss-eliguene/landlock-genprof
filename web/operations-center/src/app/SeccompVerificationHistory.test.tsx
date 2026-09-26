import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { SeccompVerificationHistory, VerificationEvidenceSeparation } from "./App";
import type { ProposalRead } from "../types";

describe("bounded Seccomp verification presentation", () => {
  it("shows per-Pod evidence and never claims the application is verified", () => {
    const proposal: ProposalRead = {
      name: "proposal-a", candidateVersion: "candidate-v1", candidateDigest: "sha256:candidate", status: {
        approvalState: "Approved", approvedCandidateDigest: "sha256:candidate", behavioralVerifications: [{
          attemptID: "attempt-a", verifierIdentity: "system:serviceaccount:verify:seccomp-verifier", verifierVersion: "1",
          verifierImage: "probe@sha256:image", probeID: "linux-seccomp-getpriority-v1", result: "VERIFIED", revocation: "UNKNOWN",
          observedAt: "2026-09-26T10:00:00Z", validUntil: "2026-09-26T10:05:00Z",
          target: { namespace: "apps", workload: "Deployment/api", podName: "api-0", podUID: "pod-uid", container: "api", profileName: "api-profile", profileUID: "profile-uid", profileDigest: "sha256:profile", candidateDigest: "sha256:candidate" },
          experiment: { twin: { exitCode: 10 }, control: { exitCode: 0 } },
          profileMaterialization: { state: "MATERIALIZED", source: "SPO.status.localhostProfile", localhostPath: "operator/api.json", limitation: "node file digest not independently established" },
          targetConfiguration: { state: "CONFIGURED_NOT_RUNTIME_VERIFIED", podUID: "pod-uid", limitation: "configuration only" },
        }],
      },
    };
    const html = renderToStaticMarkup(<><SeccompVerificationHistory proposal={proposal} canVerify={false} pending={false} onVerify={vi.fn()} /><VerificationEvidenceSeparation proposal={proposal} /></>);
    expect(html).toContain("twin Pod");
    expect(html).toContain("does not prove that the application process is enforcing");
    expect(html).toContain("system:serviceaccount:verify:seccomp-verifier");
    expect(html).toContain("10 / 0");
    expect(html).toContain("Revocation");
    expect(html).toContain("Independent evidence dimensions");
    expect(html).toContain("CONFIGURED_NOT_RUNTIME_VERIFIED");
    expect(html).toContain("not independent proof of active enforcement");
    expect(html).toContain("proposal.verify");
    expect(html).toContain("disabled");
  });

  it("renders historical proposals with no verification records", () => {
    const html = renderToStaticMarkup(<SeccompVerificationHistory proposal={{ name: "old", candidateVersion: "candidate-v1", status: { approvalState: "Approved" } }} canVerify={false} pending={false} onVerify={vi.fn()} />);
    expect(html).toContain("No per-Pod experiment facts are recorded");
  });
});
