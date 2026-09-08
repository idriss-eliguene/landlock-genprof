package reconciliation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func subject(scope history.PopulationScope, binaryPath string) EnvironmentSubject {
	return EnvironmentSubject{Scope: scope, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:" + strings.Repeat("a", 64), BinaryPath: binaryPath}
}

func candidateSpec(target, container, image string) proposal.Spec {
	return proposal.Spec{
		CandidateVersion:   proposal.CandidateVersionV2,
		Subject:            &proposal.SubjectV2{Scope: proposal.CandidateV2ScopeContainer, Target: target, Container: container, ImageIdentity: image},
		CapabilityArtifact: &proposal.ArtifactV2{Type: proposal.CandidateV2ArtifactContainerCaps, ContainerCapabilities: proposal.ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: []string{"CAP_CHOWN"}}},
		Provenance:         &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"observation-1"}},
		Qualification:      &proposal.ProposalQualification{Filesystem: "AVAILABLE", Exec: "UNKNOWN", NetworkConnect: "EMPTY", NetworkBind: "EMPTY", Capabilities: "AVAILABLE"},
		DerivationStatus:   &proposal.ProposalDerivationStatus{Capabilities: "SUPPORTED", PodLock: "NOT_AVAILABLE", NetworkPolicy: "NOT_AVAILABLE", Seccomp: "NOT_AVAILABLE"},
	}
}

func approvedCandidate(t *testing.T, ref ProposalRef) CandidateProposal {
	t.Helper()
	spec := candidateSpec("Deployment/api", "app", "sha256:"+strings.Repeat("a", 64))
	candidate, err := spec.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	candidateDigest, err := proposal.CandidateDigestV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	context, err := spec.ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	contextDigest, err := proposal.ReviewContextDigestV2(context)
	if err != nil {
		t.Fatal(err)
	}
	return CandidateProposal{Ref: ref, Spec: spec, Status: proposal.Status{ApprovalState: proposal.ApprovalApproved, ApprovedCandidateDigest: candidateDigest, ApprovedReviewContextDigest: contextDigest, ApprovalMechanismVersion: proposal.CandidateVersionV2}}
}

func TestEnvironmentSubjectIdentityAndLegacyNormalization(t *testing.T) {
	base := subject(history.ScopeContainer, "")
	tests := []struct {
		name  string
		other EnvironmentSubject
		want  bool
	}{
		{"exact", base, true},
		{"scope", subject(history.ScopeBinary, "/app/server"), false},
		{"target", EnvironmentSubject{Scope: base.Scope, Target: "Deployment/other", Container: base.Container, ImageIdentity: base.ImageIdentity}, false},
		{"container", EnvironmentSubject{Scope: base.Scope, Target: base.Target, Container: "sidecar", ImageIdentity: base.ImageIdentity}, false},
		{"image", EnvironmentSubject{Scope: base.Scope, Target: base.Target, Container: base.Container, ImageIdentity: "sha256:" + strings.Repeat("b", 64)}, false},
		{"binary path", EnvironmentSubject{Scope: history.ScopeBinary, Target: base.Target, Container: base.Container, ImageIdentity: base.ImageIdentity, BinaryPath: "/app/server"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SubjectsEqual(base, test.other); got != test.want {
				t.Fatalf("SubjectsEqual() = %v, want %v", got, test.want)
			}
		})
	}
	legacy, err := EnvironmentSubjectFromPopulationFingerprint(history.PopulationFingerprint{Target: base.Target, Container: base.Container, ImageIdentity: base.ImageIdentity, BinaryPath: "/app/server"})
	if err != nil || legacy.Scope != history.ScopeBinary {
		t.Fatalf("legacy normalization = %#v, %v", legacy, err)
	}
	binaryA := subject(history.ScopeBinary, "/app/server")
	binaryB := subject(history.ScopeBinary, "/app/worker")
	if SubjectsEqual(binaryA, binaryB) {
		t.Fatal("different binary paths unexpectedly matched")
	}
}

func TestSubjectMatchesCandidateV2RejectsBinary(t *testing.T) {
	valid := approvedCandidate(t, ProposalRef{Name: "valid"})
	candidate, err := valid.Spec.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	if !SubjectMatchesCandidateV2(subject(history.ScopeContainer, ""), candidate) {
		t.Fatal("container subject did not match candidate-v2")
	}
	if SubjectMatchesCandidateV2(subject(history.ScopeBinary, "/app/server"), candidate) {
		t.Fatal("binary subject matched candidate-v2")
	}
}

func TestSelectApprovedPolicyStates(t *testing.T) {
	valid := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "valid", UID: "uid-valid"})
	tests := []struct {
		name       string
		candidates []CandidateProposal
		want       SelectionState
	}{
		{"none", nil, SelectionNone},
		{"candidate-v1", []CandidateProposal{{Ref: ProposalRef{Name: "v1"}, Spec: proposal.Spec{}, Status: proposal.Status{ApprovalState: proposal.ApprovalApproved}}}, SelectionNone},
		{"draft", []CandidateProposal{{Ref: valid.Ref, Spec: valid.Spec, Status: proposal.Status{ApprovalState: proposal.ApprovalDraft}}}, SelectionNone},
		{"rejected", []CandidateProposal{{Ref: valid.Ref, Spec: valid.Spec, Status: proposal.Status{ApprovalState: proposal.ApprovalRejected}}}, SelectionNone},
		{"valid", []CandidateProposal{valid}, SelectionSelected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := SelectApprovedPolicy(subject(history.ScopeContainer, ""), test.candidates)
			if got.State != test.want {
				t.Fatalf("state = %q, want %q", got.State, test.want)
			}
		})
	}
}

func TestSelectApprovedPolicyRejectsStaleApprovalAndLastSnapshot(t *testing.T) {
	valid := approvedCandidate(t, ProposalRef{Name: "stale"})
	valid.Status.LastApprovalSnapshot = &proposal.ApprovalSnapshot{ProposalUID: "uid-old", ApprovalMechanismVersion: proposal.CandidateVersionV2, ApprovedCandidateDigest: valid.Status.ApprovedCandidateDigest, ReviewContextDigest: valid.Status.ApprovedReviewContextDigest, ApprovedAt: "2026-01-01T00:00:00Z"}
	valid.Status.ApprovedCandidateDigest = "sha256:" + strings.Repeat("0", 64)
	if got := SelectApprovedPolicy(subject(history.ScopeContainer, ""), []CandidateProposal{valid}); got.State != SelectionNone {
		t.Fatalf("stale candidate digest state = %q, want NONE", got.State)
	}
	valid = approvedCandidate(t, ProposalRef{Name: "stale-context"})
	valid.Status.LastApprovalSnapshot = &proposal.ApprovalSnapshot{ProposalUID: "uid-old", ApprovalMechanismVersion: proposal.CandidateVersionV2, ApprovedCandidateDigest: valid.Status.ApprovedCandidateDigest, ReviewContextDigest: valid.Status.ApprovedReviewContextDigest, ApprovedAt: "2026-01-01T00:00:00Z"}
	valid.Status.ApprovedReviewContextDigest = "sha256:" + strings.Repeat("1", 64)
	if got := SelectApprovedPolicy(subject(history.ScopeContainer, ""), []CandidateProposal{valid}); got.State != SelectionNone {
		t.Fatalf("stale review context state = %q, want NONE", got.State)
	}
}

func TestSelectApprovedPolicyFiltersMismatchesAndPreservesAmbiguity(t *testing.T) {
	first := approvedCandidate(t, ProposalRef{Namespace: "z", Name: "zeta", UID: "2"})
	second := approvedCandidate(t, ProposalRef{Namespace: "a", Name: "alpha", UID: "1"})
	second.Status = first.Status // equal digest does not transfer object authority.
	got := SelectApprovedPolicy(subject(history.ScopeContainer, ""), []CandidateProposal{first, second})
	if got.State != SelectionAmbiguous {
		t.Fatalf("state = %q, want AMBIGUOUS", got.State)
	}
	wantRefs := []ProposalRef{second.Ref, first.Ref}
	if !reflect.DeepEqual(got.AmbiguousRefs, wantRefs) {
		t.Fatalf("refs = %#v, want %#v", got.AmbiguousRefs, wantRefs)
	}
	if got.Proposal != nil {
		t.Fatal("ambiguous selection returned a Proposal")
	}

	permuted := SelectApprovedPolicy(subject(history.ScopeContainer, ""), []CandidateProposal{second, first})
	if !reflect.DeepEqual(got, permuted) {
		t.Fatalf("permuted input changed result: %#v vs %#v", got, permuted)
	}

	mismatch := approvedCandidate(t, ProposalRef{Name: "mismatch"})
	mismatch.Spec.Subject.Target = "Deployment/other"
	if got := SelectApprovedPolicy(subject(history.ScopeContainer, ""), []CandidateProposal{mismatch}); got.State != SelectionNone {
		t.Fatalf("mismatched subject state = %q, want NONE", got.State)
	}
	if got := SelectApprovedPolicy(subject(history.ScopeBinary, "/app/server"), []CandidateProposal{first}); got.State != SelectionNone {
		t.Fatalf("binary population state = %q, want NONE", got.State)
	}
}

func TestSelectApprovedPolicyValidPlusInvalidSelectsValid(t *testing.T) {
	valid := approvedCandidate(t, ProposalRef{Name: "valid"})
	stale := approvedCandidate(t, ProposalRef{Name: "stale"})
	stale.Status.ApprovedCandidateDigest = "sha256:" + strings.Repeat("f", 64)
	got := SelectApprovedPolicy(subject(history.ScopeContainer, ""), []CandidateProposal{stale, valid})
	if got.State != SelectionSelected || got.Proposal == nil || got.Proposal.Ref.Name != "valid" {
		t.Fatalf("selection = %#v, want valid selected", got)
	}
}
