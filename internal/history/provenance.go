package history

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
)

const (
	maxObservationContributions    = 256
	maxPendingContributionMarkers  = 32
	maxObservationID               = 128
	maxContributionSourceSummaries = 16
)

var (
	ErrInvalidContribution     = errors.New("invalid observation contribution")
	ErrReceiptIdentityMismatch = errors.New("contribution receipt identity mismatch")
)

// ContributionKey identifies one Observation-to-population contribution.
// PopulationFingerprint remains the existing TrainingHistory population key.
type ContributionKey struct {
	ObservationID string
	Population    PopulationFingerprint
}

func (k ContributionKey) Valid() bool {
	return len(k.ObservationID) > 0 && len(k.ObservationID) <= maxObservationID && k.Population.Valid()
}

// CanonicalBytes is a versioned, length-prefixed UTF-8 encoding. Lengths are
// encoded as big-endian uint32 values, preventing field-boundary ambiguity.
func (k ContributionKey) CanonicalBytes() ([]byte, error) {
	if !k.Valid() {
		return nil, fmt.Errorf("%w: invalid contribution key", ErrInvalidContribution)
	}
	var b bytes.Buffer
	b.WriteString("contribution-key-v1")
	population, err := k.Population.normalized()
	if err != nil {
		return nil, err
	}
	fields := []string{k.ObservationID}
	if population.Scope == ScopeContainer {
		fields = append(fields, "population-container-v2")
	}
	fields = append(fields, population.Target, population.Container, population.ImageIdentity)
	if population.Scope == ScopeBinary {
		fields = append(fields, population.BinaryPath)
	}
	for _, field := range fields {
		length, err := checkedUint32Length(len(field))
		if err != nil {
			return nil, fmt.Errorf("%w: contribution key field too long", ErrInvalidContribution)
		}
		if err := binary.Write(&b, binary.BigEndian, length); err != nil {
			return nil, err
		}
		b.WriteString(field)
	}
	return b.Bytes(), nil
}

func (k ContributionKey) Digest() (string, error) {
	b, err := k.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func (k ContributionKey) ReceiptName() (string, error) {
	digest, err := k.Digest()
	if err != nil {
		return "", err
	}
	return "obscontrib-" + digest[:32], nil
}

type ContributionSource string

const (
	SourceFilesystem     ContributionSource = "filesystem"
	SourceExec           ContributionSource = "exec"
	SourceNetworkConnect ContributionSource = "networkConnect"
	SourceNetworkBind    ContributionSource = "networkBind"
	SourceCapabilities   ContributionSource = "capabilities"
)

type ObservationSourceContribution struct {
	Source              string `json:"source"`
	EvidenceState       string `json:"evidenceState"`
	AttributionState    string `json:"attributionState"`
	BackendHealthy      bool   `json:"backendHealthy"`
	AttachedForWindow   bool   `json:"attachedForWindow"`
	FlushConfirmed      bool   `json:"flushConfirmed"`
	AttributedCount     int64  `json:"attributedCount"`
	ExcludedCount       int64  `json:"excludedCount"`
	NormalizedFactCount int64  `json:"normalizedFactCount"`
}

type ObservationContribution struct {
	ObservationID string                          `json:"observationID"`
	Sources       []ObservationSourceContribution `json:"sources"`
}

type ContributionMarker struct {
	ObservationID string                `json:"observationID"`
	Population    PopulationFingerprint `json:"population"`
	KeyDigest     string                `json:"keyDigest"`
}

func validSource(source string) bool {
	return slices.Contains([]string{string(SourceFilesystem), string(SourceExec), string(SourceNetworkConnect), string(SourceNetworkBind), string(SourceCapabilities)}, source)
}
func validEvidence(value string) bool {
	return value == "EMPTY" || value == "AVAILABLE" || value == "UNKNOWN"
}
func validAttribution(value string) bool {
	return value == "NOT_STARTED" || value == "IN_PROGRESS" || value == "COMPLETED" || value == "FAILED"
}

func (s ObservationSourceContribution) Validate() error {
	if !validSource(s.Source) || !validEvidence(s.EvidenceState) || !validAttribution(s.AttributionState) || s.AttributedCount < 0 || s.ExcludedCount < 0 || s.NormalizedFactCount < 0 || s.NormalizedFactCount > 0 && s.AttributedCount == 0 {
		return fmt.Errorf("%w: invalid source summary", ErrInvalidContribution)
	}
	return nil
}
func (c ObservationContribution) Validate() error {
	if c.ObservationID == "" || len(c.ObservationID) > maxObservationID || len(c.Sources) == 0 || len(c.Sources) > maxContributionSourceSummaries {
		return fmt.Errorf("%w: invalid contribution", ErrInvalidContribution)
	}
	seen := map[string]bool{}
	for _, source := range c.Sources {
		if err := source.Validate(); err != nil {
			return err
		}
		if seen[source.Source] {
			return fmt.Errorf("%w: duplicate source summary", ErrInvalidContribution)
		}
		seen[source.Source] = true
	}
	return nil
}
func (m ContributionMarker) Validate() error {
	if !m.KeyValid() {
		return fmt.Errorf("%w: invalid contribution marker", ErrInvalidContribution)
	}
	return nil
}
func (m ContributionMarker) KeyValid() bool {
	k := ContributionKey{ObservationID: m.ObservationID, Population: m.Population}
	digest, err := k.Digest()
	return err == nil && digest == m.KeyDigest && len(m.KeyDigest) == 64
}

func (p Population) ValidateObservationMetadata() error {
	if len(p.ObservationContributions) > maxObservationContributions || len(p.PendingContributionMarkers) > maxPendingContributionMarkers {
		return fmt.Errorf("%w: metadata bound exceeded", ErrInvalidContribution)
	}
	seen := map[string]bool{}
	for _, c := range p.ObservationContributions {
		if err := c.Validate(); err != nil {
			return err
		}
		if seen[c.ObservationID] {
			return fmt.Errorf("%w: duplicate observation contribution", ErrInvalidContribution)
		}
		seen[c.ObservationID] = true
	}
	for _, m := range p.PendingContributionMarkers {
		if err := m.Validate(); err != nil {
			return err
		}
		if seen[m.KeyDigest] {
			return fmt.Errorf("%w: duplicate contribution marker", ErrInvalidContribution)
		}
		seen[m.KeyDigest] = true
	}
	for i := 1; i < len(p.ObservationContributions); i++ {
		if p.ObservationContributions[i-1].ObservationID > p.ObservationContributions[i].ObservationID {
			return fmt.Errorf("%w: contributions not canonical", ErrInvalidContribution)
		}
	}
	for _, c := range p.ObservationContributions {
		for i := 1; i < len(c.Sources); i++ {
			if c.Sources[i-1].Source > c.Sources[i].Source {
				return fmt.Errorf("%w: source summaries not canonical", ErrInvalidContribution)
			}
		}
	}
	for i := 1; i < len(p.PendingContributionMarkers); i++ {
		if p.PendingContributionMarkers[i-1].KeyDigest > p.PendingContributionMarkers[i].KeyDigest {
			return fmt.Errorf("%w: markers not canonical", ErrInvalidContribution)
		}
	}
	return nil
}

func sortObservationMetadata(p *Population) {
	sort.Slice(p.ObservationContributions, func(i, j int) bool {
		return p.ObservationContributions[i].ObservationID < p.ObservationContributions[j].ObservationID
	})
	for i := range p.ObservationContributions {
		sort.Slice(p.ObservationContributions[i].Sources, func(a, b int) bool {
			return p.ObservationContributions[i].Sources[a].Source < p.ObservationContributions[i].Sources[b].Source
		})
	}
	sort.Slice(p.PendingContributionMarkers, func(i, j int) bool {
		return p.PendingContributionMarkers[i].KeyDigest < p.PendingContributionMarkers[j].KeyDigest
	})
}
