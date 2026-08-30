// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func TestLoadWorkbenchView_PreservesCanonicalProposalState(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	spec := proposal.Spec{
		Container:   "nginx",
		Binary:      "/usr/sbin/nginx",
		GeneratedAt: "2026-08-29T10:00:00Z",
		PodLock: `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
spec:
  profilesByContainer:
    nginx:
      /usr/sbin/nginx:
        readOnly: [/etc/nginx]
`,
		NetworkPolicy: `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
spec:
  policyTypes: [Egress]
  egress:
    - ports:
        - protocol: TCP
          port: 443
`,
		SPOSeccompProfile: `apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  name: nginx-demo
  annotations:
    landlockgenprof.io/seccomp-source: spo
    landlockgenprof.io/seccomp-origin: derived
    landlockgenprof.io/spo-source-profile: nginx-source
    landlockgenprof.io/spo-recording-namespace: default
    landlockgenprof.io/spo-recording-name: nginx-recording
    landlockgenprof.io/spo-syscall-coverage: unknown
spec:
  defaultAction: SCMP_ACT_ERRNO
  syscalls:
    - action: SCMP_ACT_ALLOW
      names: [read, write]
`,
	}
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}

	view, err := loadWorkbenchView(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("loadWorkbenchView() error = %v", err)
	}
	wantDigest, err := proposal.CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	if view.CandidateDigest != wantDigest {
		t.Fatalf("CandidateDigest = %q, want %q", view.CandidateDigest, wantDigest)
	}
	if view.Approval != string(proposal.ApprovalDraft) {
		t.Fatalf("Approval = %q, want %q", view.Approval, proposal.ApprovalDraft)
	}
	if view.Application != "NOT_AVAILABLE — application outcome is not persisted in SecurityProfileProposal" {
		t.Errorf("Application = %q, want explicit unavailable state", view.Application)
	}
	if view.Verification != "NOT_AVAILABLE — behavioral verification is not persisted in SecurityProfileProposal" {
		t.Errorf("Verification = %q, want explicit unavailable state", view.Verification)
	}
	if view.ReadAt == "" {
		t.Error("ReadAt is empty; snapshot time must be disclosed")
	}
	boundaries := strings.Join(view.Boundaries, "\n")
	if !strings.Contains(boundaries, "not a current-to-proposed comparison") {
		t.Errorf("candidate view does not disclose unavailable current state:\n%s", boundaries)
	}
	joined := make([]string, 0, len(view.Domains))
	for _, domain := range view.Domains {
		joined = append(joined, domain.Candidate)
	}
	text := strings.Join(joined, "\n")
	for _, want := range []string{"STRUCTURED candidate artifact", "STRUCTURED derived policy artifact"} {
		if !strings.Contains(text, want) {
			t.Errorf("domain projection missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"container(s)", "port rule(s)", "syscall name(s)", "filesystem path rule(s)"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("domain projection exposes invented authority metric %q:\n%s", forbidden, text)
		}
	}
	provenance := strings.Join(view.Provenance, "\n")
	for _, want := range []string{"DERIVED POLICY / SECURITY-PROFILES-OPERATOR", "Origin: derived policy", "Source profile: nginx-source", "Recording: default/nginx-recording", "Coverage: unknown"} {
		if !strings.Contains(provenance, want) {
			t.Errorf("provenance projection missing %q:\n%s", want, provenance)
		}
	}
}

func TestLoadWorkbenchView_ProjectsExactApprovalBinding(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	spec := proposal.Spec{Container: "api", Binary: "/usr/bin/api", PodLock: "apiVersion: v1\nkind: ConfigMap\n"}
	ctx := context.Background()
	if err := proposal.Save(ctx, client, "default", "api", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	digest, err := proposal.CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	if err := proposal.SetApprovalState(ctx, client, "default", "api", proposal.ApprovalApproved, "owner reviewed", digest); err != nil {
		t.Fatalf("SetApprovalState() error = %v", err)
	}

	view, err := loadWorkbenchView(ctx, client, "default", "api")
	if err != nil {
		t.Fatalf("loadWorkbenchView() error = %v", err)
	}
	if view.Approval != string(proposal.ApprovalApproved) {
		t.Errorf("Approval = %q, want %q", view.Approval, proposal.ApprovalApproved)
	}
	if view.ApprovedDigest != digest {
		t.Errorf("ApprovedDigest = %q, want canonical digest %q", view.ApprovedDigest, digest)
	}
	if view.ApprovalVersion != "candidate-v1" {
		t.Errorf("ApprovalVersion = %q, want candidate-v1", view.ApprovalVersion)
	}
	if view.ApprovalUpdated == "NOT_AVAILABLE" {
		t.Error("ApprovalUpdated is unavailable after an approval was recorded")
	}
	if !strings.Contains(view.Application, "NOT_AVAILABLE") || !strings.Contains(view.Verification, "NOT_AVAILABLE") {
		t.Errorf("approval was incorrectly promoted to application or verification: application=%q verification=%q", view.Application, view.Verification)
	}
}

func TestWorkbenchApprovalBinding_RejectsStaleAndLegacyApproval(t *testing.T) {
	spec := &proposal.Spec{Container: "api", Binary: "/usr/bin/api", PodLock: "candidate-a"}
	digest, err := proposal.CandidateDigest(*spec)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	valid := &proposal.Status{
		ApprovalState:            proposal.ApprovalApproved,
		ApprovedCandidateDigest:  digest,
		ApprovalMechanismVersion: "candidate-v1",
	}
	if got := workbenchApprovalBinding(spec, valid); !strings.HasPrefix(got, "BOUND") {
		t.Fatalf("valid approval binding = %q, want BOUND", got)
	}

	stale := *valid
	stale.ApprovedCandidateDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	if got := workbenchApprovalBinding(spec, &stale); !strings.Contains(got, "NOT BOUND / RE-APPROVAL REQUIRED") || !strings.Contains(got, "mismatch") {
		t.Fatalf("stale approval binding = %q, want canonical mismatch", got)
	}

	legacy := &proposal.Status{ApprovalState: proposal.ApprovalApproved}
	if got := workbenchApprovalBinding(spec, legacy); !strings.Contains(got, "legacy approval requires explicit re-approval") {
		t.Fatalf("legacy approval binding = %q, want re-approval requirement", got)
	}
}

func TestWorkbenchSeccompProvenance_UsesCanonicalSourceAndCoverageSemantics(t *testing.T) {
	internal := `apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  annotations:
    landlockgenprof.io/seccomp-source: internal
    landlockgenprof.io/seccomp-origin: observed
spec: {}
`
	internalText := strings.Join(workbenchSeccompProvenance(proposal.Spec{SPOSeccompProfile: internal}), "\n")
	if !strings.Contains(internalText, "DIRECT EVIDENCE / INTERNAL OBSERVATION") || strings.Contains(internalText, "DERIVED POLICY / SECURITY-PROFILES-OPERATOR") {
		t.Fatalf("internal provenance classification = %q", internalText)
	}
	internalDomains := summarizeWorkbenchDomains(proposal.Spec{SPOSeccompProfile: internal})
	if strings.Contains(internalDomains[2].Provenance, "DERIVED POLICY") || strings.Contains(internalDomains[2].Candidate, "derived policy") {
		t.Fatalf("internal seccomp domain was classified as derived policy: %+v", internalDomains[2])
	}

	spo := `apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  annotations:
    landlockgenprof.io/seccomp-source: spo
    landlockgenprof.io/seccomp-origin: derived
    landlockgenprof.io/spo-source-profile: source-profile
    landlockgenprof.io/spo-recording-namespace: observations
    landlockgenprof.io/spo-recording-name: recording
    landlockgenprof.io/spo-container-id: api
    landlockgenprof.io/spo-syscall-coverage: unknown
spec: {}
`
	spoText := strings.Join(workbenchSeccompProvenance(proposal.Spec{SPOSeccompProfile: spo}), "\n")
	if !strings.Contains(spoText, "DERIVED POLICY / SECURITY-PROFILES-OPERATOR") || !strings.Contains(spoText, "Origin: derived policy") {
		t.Fatalf("SPO provenance classification = %q", spoText)
	}

	mergedCoverage := `{"state":"known","version":"v1","total":2,"syscalls":{"read":2}}`
	merged := `apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  annotations:
    landlockgenprof.io/seccomp-source: spo
    landlockgenprof.io/seccomp-origin: derived
    landlockgenprof.io/spo-source-profile: merged-profile
    landlockgenprof.io/spo-recording-namespace: observations
    landlockgenprof.io/spo-recording-name: recording
    landlockgenprof.io/spo-derivation: merged
    landlockgenprof.io/spo-source-kind: ProfileRecording
    landlockgenprof.io/spo-merge-strategy: Containers
    landlockgenprof.io/spo-contributor-lineage: unavailable
    landlockgenprof.io/target-namespace: payments
    landlockgenprof.io/target-pod: api
    landlockgenprof.io/target-container: app
    landlockgenprof.io/spo-syscall-coverage: '` + mergedCoverage + `'
spec: {}
`
	mergedText := strings.Join(workbenchSeccompProvenance(proposal.Spec{SPOSeccompProfile: merged}), "\n")
	for _, want := range []string{"Derivation: merged", "Merge strategy: Containers", "Contributor lineage: unavailable", "Application target: payments/api container app", "Widening warning", "contributing partial profiles"} {
		if !strings.Contains(mergedText, want) {
			t.Errorf("merged provenance missing %q: %s", want, mergedText)
		}
	}
	if strings.Contains(mergedText, mergedCoverage) || strings.Contains(mergedText, "confidence: high") || strings.Contains(mergedText, "frequency") {
		t.Fatalf("merged coverage was rendered as raw/frequency/confidence data: %s", mergedText)
	}
}

func TestWorkbenchHandler_IsReadOnlyAndEscapesProposalData(t *testing.T) {
	view := workbenchView{
		Namespace:       "default",
		Proposal:        `<script>alert(1)</script>`,
		CandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Lifecycle:       "PROPOSAL — structured snapshot",
		Approval:        "Draft",
		ApprovedDigest:  "NOT_AVAILABLE — no approved candidate is recorded",
		ApprovalVersion: "NOT_AVAILABLE",
		ApprovalUpdated: "NOT_AVAILABLE",
		ReadAt:          "2026-08-30T12:00:00Z",
		Domains: []workbenchDomain{{
			Name:         "SPO SeccompProfile",
			Candidate:    "NOT_AVAILABLE",
			Availability: "NOT_AVAILABLE — artifact not present",
			Provenance:   "DERIVED POLICY / SPO snapshot",
			ReviewState:  "REVIEW REQUIRED",
		}},
		Application:  "NOT_AVAILABLE",
		Verification: "NOT_AVAILABLE",
	}
	handler := newWorkbenchHandler(view)

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", get.Code, http.StatusOK)
	}
	body := get.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("proposal identity was not safely escaped:\n%s", body)
	}
	for _, want := range []string{"Candidate authority / policy", "Evidence & provenance", "Authorization", "Enforcement evidence", "NOT_AVAILABLE — artifact not present", "current-to-proposed delta", "snapshot read at 2026-08-30T12:00:00Z", "Restart the command to refresh cluster state"} {
		if !strings.Contains(body, want) {
			t.Errorf("page omitted review boundary %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, view.CandidateDigest) || !strings.Contains(body, "Approval") {
		t.Fatalf("page omitted canonical review fields:\n%s", body)
	}
	if strings.Contains(body, "<button") || strings.Contains(body, "approve-proposal") || strings.Contains(body, "apply-proposal") {
		t.Fatalf("page exposes a mutation control:\n%s", body)
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / status = %d, want %d", post.Code, http.StatusMethodNotAllowed)
	}
	mutation := httptest.NewRecorder()
	handler.ServeHTTP(mutation, httptest.NewRequest(http.MethodGet, "/approve", nil))
	if mutation.Code != http.StatusNotFound {
		t.Fatalf("GET /approve status = %d, want %d", mutation.Code, http.StatusNotFound)
	}
}

func TestWorkbenchListenAddress_IsLoopbackOnly(t *testing.T) {
	if got := workbenchListenAddress(8080); got != "127.0.0.1:8080" {
		t.Fatalf("workbenchListenAddress() = %q, want loopback address", got)
	}
}

func TestWorkbenchReadHeaderTimeout_IsNonZero(t *testing.T) {
	if workbenchReadHeaderTimeout <= 0 || workbenchReadHeaderTimeout != 5*time.Second {
		t.Fatalf("workbenchReadHeaderTimeout = %s, want 5s", workbenchReadHeaderTimeout)
	}
}
