package proposal

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

func TestReviewAttributionCountEncodingBounds(t *testing.T) {
	max := int(^uint32(0))
	var encoded bytes.Buffer
	if err := putReviewCount(&encoded, max); err != nil {
		t.Fatalf("maximum uint32 count rejected: %v", err)
	}
	if encoded.Len() != 4 || binary.BigEndian.Uint32(encoded.Bytes()) != ^uint32(0) {
		t.Fatalf("maximum count encoding = %x", encoded.Bytes())
	}
	if err := putReviewCount(&encoded, max+1); err == nil || err.Error() != "capability attribution count: canonical field length exceeds uint32" {
		t.Fatalf("out-of-range attribution count error = %v", err)
	}
}

func v2SpecFixture() Spec {
	return Spec{
		CandidateVersion:   CandidateVersionV2,
		Subject:            &SubjectV2{Scope: CandidateV2ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:" + strings.Repeat("a", 64)},
		CapabilityArtifact: &ArtifactV2{Type: CandidateV2ArtifactContainerCaps, ContainerCapabilities: ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: []string{"CAP_NET_ADMIN", "CAP_CHOWN"}}},
		Provenance:         &ProposalProvenance{PopulationScope: CandidateV2ScopeContainer, ObservationIDs: []string{"obs-b", "obs-a"}},
		Qualification:      &ProposalQualification{Filesystem: "UNKNOWN", Exec: "UNKNOWN", NetworkConnect: "AVAILABLE", NetworkBind: "EMPTY", Capabilities: "AVAILABLE"},
		DerivationStatus:   &ProposalDerivationStatus{Capabilities: "SUPPORTED", PodLock: "NOT_AVAILABLE", NetworkPolicy: "NOT_AVAILABLE", Seccomp: "NOT_AVAILABLE"},
	}
}

func TestReviewContextV2FixedFixture(t *testing.T) {
	c, err := v2SpecFixture().ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReviewContextCanonicalBytesV2(c)
	if err != nil {
		t.Fatal(err)
	}
	const wantHex = "0000001a70726f706f73616c2d7265766965772d636f6e746578742d763200000009434f4e5441494e455200000002000000056f62732d61000000056f62732d6200000007554e4b4e4f574e00000007554e4b4e4f574e00000009415641494c41424c4500000005454d50545900000009415641494c41424c4500000009535550504f525445440000000d4e4f545f415641494c41424c450000000d4e4f545f415641494c41424c450000000d4e4f545f415641494c41424c45"
	if got := hex.EncodeToString(b); got != wantHex {
		t.Fatalf("review context hex = %s", got)
	}
	got, err := ReviewContextDigestV2(c)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sha256:559d6234e02a96a88bb3911cd2483b124bd9fd434ce6847ab76ca056cf3d6622" {
		t.Fatalf("review context digest = %s", got)
	}
}

func TestCandidateV2ApprovalDispatchAndContextBinding(t *testing.T) {
	ctx := context.Background()
	spec := v2SpecFixture()
	client := custodyClient(t, "v2-proposal", "v2-uid", spec)
	candidate, err := spec.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := CandidateDigestV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	review, err := spec.ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	reviewDigest, err := ReviewContextDigestV2(review)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", "v2-proposal", ApprovalApproved, "approve", digest); err != nil {
		t.Fatal(err)
	}
	status, err := GetStatus(ctx, client, "default", "v2-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if status.ApprovedReviewContextDigest != reviewDigest || status.LastApprovalSnapshot == nil || status.LastApprovalSnapshot.ReviewContextDigest != reviewDigest || status.ApprovalMechanismVersion != CandidateVersionV2 {
		t.Fatalf("v2 approval status = %+v", status)
	}
	if err := ValidateApprovedCandidate(&spec, status); err != nil {
		t.Fatalf("v2 approval validation = %v", err)
	}

	mutated := spec
	mutated.Provenance = &ProposalProvenance{PopulationScope: CandidateV2ScopeContainer, ObservationIDs: []string{"obs-c"}}
	if err := Save(ctx, client, "default", "v2-proposal", mutated); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", "v2-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateApprovedCandidate(&mutated, status); err == nil {
		t.Fatal("provenance mutation retained v2 approval")
	}
	if status.LastApprovalSnapshot.ReviewContextDigest != reviewDigest {
		t.Fatal("provenance mutation changed approval custody")
	}
}

func TestCandidateV2ReviewAndCandidateDigestsAreSeparated(t *testing.T) {
	a := v2SpecFixture()
	b := v2SpecFixture()
	b.Provenance = &ProposalProvenance{PopulationScope: CandidateV2ScopeContainer, ObservationIDs: []string{"different"}}
	ca, err := a.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	cb, err := b.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	da, err := CandidateDigestV2(ca)
	if err != nil {
		t.Fatal(err)
	}
	db, err := CandidateDigestV2(cb)
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("provenance changed candidate digest: %s != %s", da, db)
	}
	ra, err := ReviewContextDigestV2(mustReviewContext(t, a))
	if err != nil {
		t.Fatal(err)
	}
	rb, err := ReviewContextDigestV2(mustReviewContext(t, b))
	if err != nil {
		t.Fatal(err)
	}
	if ra == rb {
		t.Fatal("provenance did not change review context digest")
	}
	rbBeforeArtifact := rb
	b.CapabilityArtifact.ContainerCapabilities.Add = []string{"CAP_SYS_ADMIN", "CAP_CHOWN"}
	cb, err = b.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	db, err = CandidateDigestV2(cb)
	if err != nil {
		t.Fatal(err)
	}
	if da == db {
		t.Fatal("artifact did not change candidate digest")
	}
	if rb, err = ReviewContextDigestV2(mustReviewContext(t, b)); err != nil {
		t.Fatal(err)
	}
	if rbBeforeArtifact != rb {
		t.Fatal("artifact changed review context digest")
	}
}

func TestCapabilityAttributionChangesReviewContextNotCandidateDigest(t *testing.T) {
	a := v2SpecFixture()
	b := v2SpecFixture()
	b.Provenance.CapabilityAttribution = []CapabilityAttribution{
		{Capability: "CAP_NET_ADMIN", State: CapabilityAttributionKnown, ObservationIDs: []string{"obs-a"}},
		{Capability: "CAP_CHOWN", State: CapabilityAttributionUnknown},
	}
	candidateA, err := a.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	da, err := CandidateDigestV2(candidateA)
	if err != nil {
		t.Fatal(err)
	}
	candidateB, err := b.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	db, err := CandidateDigestV2(candidateB)
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("provenance changed candidate digest: %s != %s", da, db)
	}
	ra, err := ReviewContextDigestV2(mustReviewContext(t, a))
	if err != nil {
		t.Fatal(err)
	}
	rb, err := ReviewContextDigestV2(mustReviewContext(t, b))
	if err != nil {
		t.Fatal(err)
	}
	if ra == rb {
		t.Fatal("capability attribution did not bind review context")
	}
}

func mustReviewContext(t *testing.T, spec Spec) ProposalReviewContextV2 {
	t.Helper()
	c, err := spec.ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCandidateV2HybridValidationAndVersionNormalization(t *testing.T) {
	legacy := Spec{Container: "app", Binary: "/bin/app"}
	if got, err := legacy.NormalizedCandidateVersion(); err != nil || got != CandidateVersionV1 {
		t.Fatalf("legacy version = %q, %v", got, err)
	}
	if err := ValidateProposalSpec(Spec{CandidateVersion: "candidate-v3"}); err == nil {
		t.Fatal("unknown version accepted")
	}
	hybrid := v2SpecFixture()
	hybrid.Binary = "/bin/forbidden"
	if err := ValidateProposalSpec(hybrid); err == nil {
		t.Fatal("v2/v1 hybrid accepted")
	}
	v1Hybrid := legacy
	v1Hybrid.Provenance = &ProposalProvenance{}
	if err := ValidateProposalSpec(v1Hybrid); err == nil {
		t.Fatal("v1/v2 hybrid accepted")
	}
}
