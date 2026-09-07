package history

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// PopulationScope describes the attribution identity represented by a
// TrainingHistory population. It is deliberately narrower than an evidence
// or completeness state.
type PopulationScope string

const (
	ScopeBinary    PopulationScope = "BINARY"
	ScopeContainer PopulationScope = "CONTAINER"
)

var ErrInvalidPopulationIdentity = errors.New("invalid population identity")

// PopulationIdentity is the explicit identity-bearing portion of Population.
// BinaryPath is meaningful only for ScopeBinary.
type PopulationIdentity struct {
	Scope         PopulationScope
	Target        string
	Container     string
	ImageIdentity string
	BinaryPath    string
}

func (i PopulationIdentity) Validate() error {
	if i.Target == "" || i.Container == "" || i.ImageIdentity == "" {
		return fmt.Errorf("%w: target, container, and image identity are required", ErrInvalidPopulationIdentity)
	}
	switch i.Scope {
	case ScopeBinary:
		if i.BinaryPath == "" {
			return fmt.Errorf("%w: binary scope requires binary path", ErrInvalidPopulationIdentity)
		}
	case ScopeContainer:
		if i.BinaryPath != "" {
			return fmt.Errorf("%w: container scope forbids binary path", ErrInvalidPopulationIdentity)
		}
	default:
		return fmt.Errorf("%w: unsupported population scope %q", ErrInvalidPopulationIdentity, i.Scope)
	}
	return nil
}

// ContainerPopulationCanonicalBytes returns the versioned, scope-separated
// canonical identity. The domain and every field are length-prefixed
// big-endian UTF-8 strings.
func ContainerPopulationCanonicalBytes(i PopulationIdentity) ([]byte, error) {
	if i.Scope != ScopeContainer {
		return nil, fmt.Errorf("%w: container fingerprint requires CONTAINER scope", ErrInvalidPopulationIdentity)
	}
	if err := i.Validate(); err != nil {
		return nil, err
	}
	var canonical bytes.Buffer
	for _, field := range []string{"population-container-v2", i.Target, i.Container, i.ImageIdentity} {
		length, err := checkedUint32Length(len(field))
		if err != nil {
			return nil, fmt.Errorf("%w: fingerprint field too long", ErrInvalidPopulationIdentity)
		}
		if err := binary.Write(&canonical, binary.BigEndian, length); err != nil {
			return nil, fmt.Errorf("%w: encoding container fingerprint: %v", ErrInvalidPopulationIdentity, err)
		}
		canonical.WriteString(field)
	}
	return canonical.Bytes(), nil
}

// ContainerPopulationFingerprint returns the versioned, scope-separated
// SHA-256 fingerprint for a container population.
func ContainerPopulationFingerprint(i PopulationIdentity) (string, error) {
	canonical, err := ContainerPopulationCanonicalBytes(i)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// FingerprintForPopulation dispatches by explicit scope. The binary result
// uses the existing four-field population key encoding; container results use
// ContainerPopulationFingerprint. No empty-binary fallback is permitted.
func FingerprintForPopulation(i PopulationIdentity) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	if i.Scope == ScopeBinary {
		return populationKey(PopulationFingerprint{Target: i.Target, Container: i.Container, ImageIdentity: i.ImageIdentity, BinaryPath: i.BinaryPath}), nil
	}
	return ContainerPopulationFingerprint(i)
}

// Identity returns the explicit identity. An absent scope is not accepted in
// the domain; persistence compatibility belongs to NormalizeLegacyScope.
func (p Population) Identity() (PopulationIdentity, error) {
	i := PopulationIdentity{Scope: p.Scope, Target: p.Target, Container: p.Container, ImageIdentity: p.ImageIdentity, BinaryPath: p.BinaryPath}
	if err := i.Validate(); err != nil {
		return PopulationIdentity{}, err
	}
	return i, nil
}

// NormalizeLegacyScope converts the released representation (no scope field)
// into an explicit binary population without rewriting persistence.
func NormalizeLegacyScope(p Population) (Population, error) {
	if p.Scope == "" {
		if p.BinaryPath == "" {
			return Population{}, fmt.Errorf("%w: legacy population has no binary path", ErrInvalidPopulationIdentity)
		}
		p.Scope = ScopeBinary
	}
	if _, err := p.Identity(); err != nil {
		return Population{}, err
	}
	return p, nil
}

// PopulationIdentityEqual compares only same-scope identity. It is provided
// for future scope-aware matching; legacy fingerprint and merge algorithms
// remain unchanged in this gate.
func PopulationIdentityEqual(a, b Population) bool {
	aa, errA := a.Identity()
	bb, errB := b.Identity()
	if errA != nil || errB != nil || aa.Scope != bb.Scope || aa.Target != bb.Target || aa.Container != bb.Container || aa.ImageIdentity != bb.ImageIdentity {
		return false
	}
	return aa.Scope == ScopeContainer || aa.BinaryPath == bb.BinaryPath
}
