package proposal

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

const (
	CandidateVersionV1    = "candidate-v1"
	CandidateVersionV2    = CandidateV2Version
	ReviewContextV2Domain = "proposal-review-context-v2"
)

// ProposalProvenance is a bounded snapshot of the contributions used to
// construct a proposal. It is not a live history reference.
type ProposalProvenance struct {
	PopulationScope string   `json:"populationScope"`
	ObservationIDs  []string `json:"observationIDs"`
}

// ProposalQualification records only the frozen evidence-state vocabulary.
type ProposalQualification struct {
	Filesystem     string `json:"filesystem"`
	Exec           string `json:"exec"`
	NetworkConnect string `json:"networkConnect"`
	NetworkBind    string `json:"networkBind"`
	Capabilities   string `json:"capabilities"`
}

// ProposalDerivationStatus records bounded backend availability, not a
// score and not an enforcement artifact.
type ProposalDerivationStatus struct {
	Capabilities  string `json:"capabilities"`
	PodLock       string `json:"podLock"`
	NetworkPolicy string `json:"networkPolicy"`
	Seccomp       string `json:"seccomp"`
}

func normalizedCandidateVersion(value string) (string, error) {
	if value == "" {
		return CandidateVersionV1, nil
	}
	if value != CandidateVersionV1 && value != CandidateVersionV2 {
		return "", fmt.Errorf("unsupported candidate version %q", value)
	}
	return value, nil
}

// NormalizedCandidateVersion resolves the persistence-compatible version
// without rewriting legacy objects.
func (s Spec) NormalizedCandidateVersion() (string, error) {
	return normalizedCandidateVersion(s.CandidateVersion)
}

func (p ProposalProvenance) normalize() (ProposalProvenance, error) {
	if p.PopulationScope != CandidateV2ScopeContainer {
		return ProposalProvenance{}, fmt.Errorf("provenance population scope must be CONTAINER")
	}
	if len(p.ObservationIDs) > 256 {
		return ProposalProvenance{}, fmt.Errorf("provenance has %d ObservationIDs; maximum is 256", len(p.ObservationIDs))
	}
	ids := append([]string(nil), p.ObservationIDs...)
	for _, id := range ids {
		if id == "" {
			return ProposalProvenance{}, fmt.Errorf("provenance ObservationID must be non-empty")
		}
	}
	sort.Strings(ids)
	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			return ProposalProvenance{}, fmt.Errorf("duplicate provenance ObservationID %q", ids[i])
		}
	}
	return ProposalProvenance{PopulationScope: p.PopulationScope, ObservationIDs: ids}, nil
}

func validEvidenceState(value string) bool {
	return value == "AVAILABLE" || value == "UNKNOWN" || value == "EMPTY"
}

func (q ProposalQualification) validate() error {
	values := []string{q.Filesystem, q.Exec, q.NetworkConnect, q.NetworkBind, q.Capabilities}
	for _, value := range values {
		if !validEvidenceState(value) {
			return fmt.Errorf("invalid proposal evidence state %q", value)
		}
	}
	return nil
}

func (d ProposalDerivationStatus) validate() error {
	if d.Capabilities != "SUPPORTED" {
		return fmt.Errorf("capabilities derivation status must be SUPPORTED")
	}
	for name, value := range map[string]string{"podLock": d.PodLock, "networkPolicy": d.NetworkPolicy, "seccomp": d.Seccomp} {
		if value != "NOT_AVAILABLE" && value != "UNSUPPORTED" {
			return fmt.Errorf("invalid %s derivation status %q", name, value)
		}
	}
	return nil
}

// ValidateProposalSpec applies the explicit v1/v2 boundary. Legacy v1
// objects remain valid when CandidateVersion is absent.
func ValidateProposalSpec(spec Spec) error {
	version, err := normalizedCandidateVersion(spec.CandidateVersion)
	if err != nil {
		return err
	}
	if version == CandidateVersionV1 {
		if spec.Subject != nil || spec.CapabilityArtifact != nil || spec.Provenance != nil || spec.Qualification != nil || spec.DerivationStatus != nil {
			return fmt.Errorf("candidate-v1 proposal cannot contain candidate-v2 fields")
		}
		return nil
	}
	if spec.Container != "" || spec.Binary != "" || spec.PodLock != "" || spec.NetworkPolicy != "" || spec.PatchedManifest != "" || spec.SPOSeccompProfile != "" || spec.TargetBinding != nil {
		return fmt.Errorf("candidate-v2 proposal cannot contain legacy v1 fields")
	}
	if spec.Subject == nil || spec.CapabilityArtifact == nil || spec.Provenance == nil || spec.Qualification == nil || spec.DerivationStatus == nil {
		return fmt.Errorf("candidate-v2 proposal requires subject, artifact, provenance, qualification, and derivation status")
	}
	if err := spec.Subject.validate(); err != nil {
		return err
	}
	if err := spec.CapabilityArtifact.validate(); err != nil {
		return err
	}
	if _, err := spec.Provenance.normalize(); err != nil {
		return err
	}
	if err := spec.Qualification.validate(); err != nil {
		return err
	}
	if err := spec.DerivationStatus.validate(); err != nil {
		return err
	}
	return nil
}

func (c ContainerCapabilitiesV2) validate() error {
	if len(c.Drop) != 1 || c.Drop[0] != "ALL" {
		return fmt.Errorf("candidate-v2 Drop must be exactly [ALL]")
	}
	_, err := normalizeCapabilities(c)
	return err
}

func (s Spec) candidateV2() (CandidateV2, error) {
	if err := ValidateProposalSpec(s); err != nil {
		return CandidateV2{}, err
	}
	return CandidateV2{Version: CandidateV2Version, Subject: *s.Subject, Artifact: *s.CapabilityArtifact}, nil
}

func (s Spec) CandidateV2() (CandidateV2, error) { return s.candidateV2() }

func (s Spec) reviewContextV2() (ProposalReviewContextV2, error) {
	if err := ValidateProposalSpec(s); err != nil {
		return ProposalReviewContextV2{}, err
	}
	p, err := s.Provenance.normalize()
	if err != nil {
		return ProposalReviewContextV2{}, err
	}
	return ProposalReviewContextV2{Provenance: p, Qualification: *s.Qualification, DerivationStatus: *s.DerivationStatus}, nil
}

func (s Spec) ReviewContextV2() (ProposalReviewContextV2, error) { return s.reviewContextV2() }

type ProposalReviewContextV2 struct {
	Provenance       ProposalProvenance
	Qualification    ProposalQualification
	DerivationStatus ProposalDerivationStatus
}

func putReviewString(b *bytes.Buffer, value string) error {
	length, err := checkedUint32Length(len(value))
	if err != nil {
		return fmt.Errorf("review context string too long")
	}
	if err := binary.Write(b, binary.BigEndian, length); err != nil {
		return err
	}
	_, err = b.WriteString(value)
	return err
}

func putReviewList(b *bytes.Buffer, values []string) error {
	length, err := checkedUint32Length(len(values))
	if err != nil {
		return fmt.Errorf("review context list too long")
	}
	if err := binary.Write(b, binary.BigEndian, length); err != nil {
		return err
	}
	for _, value := range values {
		if err := putReviewString(b, value); err != nil {
			return err
		}
	}
	return nil
}

// ReviewContextCanonicalBytesV2 is deliberately separate from CandidateV2
// canonicalization and contains no enforcement candidate fields.
func ReviewContextCanonicalBytesV2(c ProposalReviewContextV2) ([]byte, error) {
	p, err := c.Provenance.normalize()
	if err != nil {
		return nil, err
	}
	if err := c.Qualification.validate(); err != nil {
		return nil, err
	}
	if err := c.DerivationStatus.validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := putReviewString(&b, ReviewContextV2Domain); err != nil {
		return nil, err
	}
	if err := putReviewString(&b, p.PopulationScope); err != nil {
		return nil, err
	}
	if err := putReviewList(&b, p.ObservationIDs); err != nil {
		return nil, err
	}
	for _, value := range []string{c.Qualification.Filesystem, c.Qualification.Exec, c.Qualification.NetworkConnect, c.Qualification.NetworkBind, c.Qualification.Capabilities, c.DerivationStatus.Capabilities, c.DerivationStatus.PodLock, c.DerivationStatus.NetworkPolicy, c.DerivationStatus.Seccomp} {
		if err := putReviewString(&b, value); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func ReviewContextDigestV2(c ProposalReviewContextV2) (string, error) {
	b, err := ReviewContextCanonicalBytesV2(c)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}

func (s *ApprovalSnapshot) validateV2() error {
	if s == nil || s.ProposalUID == "" || s.ApprovalMechanismVersion != CandidateVersionV2 || s.ReviewContextDigest == "" {
		return fmt.Errorf("invalid candidate-v2 approval snapshot")
	}
	if err := ValidateCandidateDigest(s.ApprovedCandidateDigest); err != nil {
		return err
	}
	if err := ValidateCandidateDigest(s.ReviewContextDigest); err != nil {
		return fmt.Errorf("invalid review context digest: %w", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, s.ApprovedAt); err != nil {
		return err
	}
	return nil
}
