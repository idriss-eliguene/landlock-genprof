// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

const maxContributionFacts = 256

const (
	contributionConvergenceReads = 5
	contributionConvergenceDelay = 10 * time.Millisecond
)

var (
	ErrUnsupportedContributionFact = errors.New("unsupported contribution fact")
	ErrContributionContentMismatch = errors.New("contribution content mismatch")
	ErrContributionKeyCollision    = errors.New("contribution key collision")
	ErrAuditProvenanceCapacity     = errors.New("audit provenance capacity exceeded")
	ErrContributionMarkerCapacity  = errors.New("contribution marker capacity exceeded")
	ErrHistoryPopulationNotFound   = errors.New("training history population not found")
	ErrHistoryCASExhausted         = errors.New("training history CAS retries exhausted")
	ErrReceiptCommitFailure        = errors.New("contribution receipt commit failed")
	ErrMarkerCleanupFailure        = errors.New("contribution marker cleanup failed")
	errProvenanceWithoutMarker     = errors.New("provenance already exists without marker")
)

// Contribution is the explicit, already-normalized input to G6.3. It has no
// Observation or raw-event dependency. Exec is represented only by Sources;
// there is deliberately no exec behavior fact field.
type Contribution struct {
	ObservationID  string
	Population     PopulationFingerprint
	Filesystem     []profile.FileAccess
	NetworkConnect []profile.NetworkAccess
	NetworkBind    []profile.NetworkAccess
	Capabilities   []profile.CapabilityAccess
	Sources        []ObservationSourceContribution
}

type ContributionApplyResult string

const (
	ContributionApplied                 ContributionApplyResult = "APPLIED"
	ContributionAlreadyCommitted        ContributionApplyResult = "ALREADY_COMMITTED"
	ContributionRecoveredAndCommitted   ContributionApplyResult = "RECOVERED_AND_COMMITTED"
	ContributionCommittedCleanupPending ContributionApplyResult = "COMMITTED_CLEANUP_PENDING"
)

type normalizedContribution struct {
	Contribution
	Filesystem     []profile.FileAccess
	NetworkConnect []profile.NetworkAccess
	NetworkBind    []profile.NetworkAccess
	Capabilities   []profile.CapabilityAccess
	Sources        []ObservationSourceContribution
}

func (c Contribution) normalize() (normalizedContribution, error) {
	population, populationErr := c.Population.normalized()
	if c.ObservationID == "" || len(c.ObservationID) > maxObservationID || populationErr != nil {
		return normalizedContribution{}, fmt.Errorf("%w: invalid contribution identity", ErrInvalidContribution)
	}
	c.Population = population
	if len(c.Filesystem)+len(c.NetworkConnect)+len(c.NetworkBind)+len(c.Capabilities) > maxContributionFacts {
		return normalizedContribution{}, fmt.Errorf("%w: fact bound exceeded", ErrInvalidContribution)
	}
	for _, source := range c.Sources {
		if err := source.Validate(); err != nil {
			return normalizedContribution{}, err
		}
	}
	if len(c.Sources) == 0 || len(c.Sources) > maxContributionSourceSummaries {
		return normalizedContribution{}, fmt.Errorf("%w: source bound exceeded", ErrInvalidContribution)
	}
	seen := map[string]bool{}
	sources := append([]ObservationSourceContribution(nil), c.Sources...)
	for _, source := range sources {
		if seen[source.Source] {
			return normalizedContribution{}, fmt.Errorf("%w: duplicate source summary", ErrInvalidContribution)
		}
		seen[source.Source] = true
	}
	n := normalizedContribution{Contribution: c, Sources: sources}
	n.Filesystem = append([]profile.FileAccess(nil), c.Filesystem...)
	n.NetworkConnect = append([]profile.NetworkAccess(nil), c.NetworkConnect...)
	n.NetworkBind = append([]profile.NetworkAccess(nil), c.NetworkBind...)
	n.Capabilities = append([]profile.CapabilityAccess(nil), c.Capabilities...)
	for i := range n.Filesystem {
		n.Filesystem[i].Confidence = ""
		n.Filesystem[i].SeenCount = 0
	}
	for i := range n.NetworkConnect {
		n.NetworkConnect[i].Confidence = ""
		n.NetworkConnect[i].SeenCount = 0
		if n.NetworkConnect[i].Direction != profile.DirectionEgress {
			return normalizedContribution{}, fmt.Errorf("%w: network connect direction", ErrInvalidContribution)
		}
	}
	for i := range n.NetworkBind {
		n.NetworkBind[i].Confidence = ""
		n.NetworkBind[i].SeenCount = 0
		if n.NetworkBind[i].Direction != profile.DirectionIngress {
			return normalizedContribution{}, fmt.Errorf("%w: network bind direction", ErrInvalidContribution)
		}
	}
	for i := range n.Capabilities {
		n.Capabilities[i].Confidence = ""
		n.Capabilities[i].SeenCount = 0
	}
	n.Filesystem = dedupFileFacts(n.Filesystem)
	n.NetworkConnect = dedupNetworkFacts(n.NetworkConnect)
	n.NetworkBind = dedupNetworkFacts(n.NetworkBind)
	n.Capabilities = dedupCapabilityFacts(n.Capabilities)
	if len(n.Filesystem)+len(n.NetworkConnect)+len(n.NetworkBind)+len(n.Capabilities) > maxContributionFacts {
		return normalizedContribution{}, fmt.Errorf("%w: fact bound exceeded", ErrInvalidContribution)
	}
	sort.Slice(n.Sources, func(i, j int) bool { return n.Sources[i].Source < n.Sources[j].Source })
	return n, nil
}

func dedupFileFacts(values []profile.FileAccess) []profile.FileAccess {
	byPath := map[string]profile.FileAccess{}
	for _, value := range values {
		if old, ok := byPath[value.Path]; ok {
			old.Permissions = mergePermissions(old.Permissions, value.Permissions)
			byPath[value.Path] = old
		} else {
			byPath[value.Path] = value
		}
	}
	out := make([]profile.FileAccess, 0, len(byPath))
	for _, value := range byPath {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func dedupNetworkFacts(values []profile.NetworkAccess) []profile.NetworkAccess {
	byKey := map[netRecordKey]profile.NetworkAccess{}
	for _, value := range values {
		byKey[netRecordKey{value.Port, value.Direction}] = value
	}
	out := make([]profile.NetworkAccess, 0, len(byKey))
	for _, value := range byKey {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Direction < out[j].Direction
	})
	return out
}

func dedupCapabilityFacts(values []profile.CapabilityAccess) []profile.CapabilityAccess {
	byName := map[string]profile.CapabilityAccess{}
	for _, value := range values {
		byName[value.Name] = value
	}
	out := make([]profile.CapabilityAccess, 0, len(byName))
	for _, value := range byName {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c normalizedContribution) contentDigest() (string, error) {
	var b bytes.Buffer
	b.WriteString("observation-contribution-v1")
	write := func(value string) error {
		length, err := checkedUint32Length(len(value))
		if err != nil {
			return err
		}
		if err := binary.Write(&b, binary.BigEndian, length); err != nil {
			return err
		}
		b.WriteString(value)
		return nil
	}
	writeAll := func(values ...string) error {
		for _, value := range values {
			if err := write(value); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeAll(c.ObservationID); err != nil {
		return "", err
	}
	if c.Population.Scope == ScopeContainer {
		if err := writeAll("population-container-v2"); err != nil {
			return "", err
		}
	}
	if err := writeAll(c.Population.Target, c.Population.Container, c.Population.ImageIdentity); err != nil {
		return "", err
	}
	if c.Population.Scope == ScopeBinary {
		if err := writeAll(c.Population.BinaryPath); err != nil {
			return "", err
		}
	}
	if err := writeAll("filesystem"); err != nil {
		return "", err
	}
	for _, value := range c.Filesystem {
		if err := writeAll(value.Path); err != nil {
			return "", err
		}
		for _, permission := range mergePermissions(nil, value.Permissions) {
			if err := writeAll(string(permission)); err != nil {
				return "", err
			}
		}
	}
	if err := writeAll("networkConnect"); err != nil {
		return "", err
	}
	for _, value := range c.NetworkConnect {
		if err := writeAll(fmt.Sprint(value.Port), string(value.Direction)); err != nil {
			return "", err
		}
	}
	if err := writeAll("networkBind"); err != nil {
		return "", err
	}
	for _, value := range c.NetworkBind {
		if err := writeAll(fmt.Sprint(value.Port), string(value.Direction)); err != nil {
			return "", err
		}
	}
	if err := writeAll("capabilities"); err != nil {
		return "", err
	}
	for _, value := range c.Capabilities {
		if err := writeAll(value.Name); err != nil {
			return "", err
		}
	}
	for _, source := range c.Sources {
		if err := writeAll(source.Source, source.EvidenceState, source.AttributionState, fmt.Sprint(source.BackendHealthy), fmt.Sprint(source.AttachedForWindow), fmt.Sprint(source.FlushConfirmed), fmt.Sprint(source.AttributedCount), fmt.Sprint(source.ExcludedCount), fmt.Sprint(source.NormalizedFactCount)); err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

// ContributionDigest binds a normalized payload to its ContributionKey. It
// is a separate digest domain from candidate-v1 and is used only by G6.3
// receipt bookkeeping.
func ContributionDigest(c Contribution) (string, error) {
	normalized, err := c.normalize()
	if err != nil {
		return "", err
	}
	return normalized.contentDigest()
}

func addContributionFacts(pop *Population, c normalizedContribution) {
	for _, value := range c.Filesystem {
		idx := -1
		for i := range pop.FilesystemAccesses {
			if pop.FilesystemAccesses[i].Path == value.Path {
				idx = i
				break
			}
		}
		if idx >= 0 {
			pop.FilesystemAccesses[idx].Permissions = mergePermissions(pop.FilesystemAccesses[idx].Permissions, value.Permissions)
		} else {
			pop.FilesystemAccesses = append(pop.FilesystemAccesses, FileAccessRecord{Path: value.Path, Permissions: mergePermissions(nil, value.Permissions)})
		}
	}
	for _, value := range c.NetworkConnect {
		addNetworkFact(pop, value)
	}
	for _, value := range c.NetworkBind {
		addNetworkFact(pop, value)
	}
	for _, value := range c.Capabilities {
		idx := -1
		for i := range pop.CapabilityAccesses {
			if pop.CapabilityAccesses[i].Name == value.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			pop.CapabilityAccesses = append(pop.CapabilityAccesses, CapabilityAccessRecord{Name: value.Name})
		}
	}
	sort.Slice(pop.FilesystemAccesses, func(i, j int) bool { return pop.FilesystemAccesses[i].Path < pop.FilesystemAccesses[j].Path })
	sort.Slice(pop.NetworkAccesses, func(i, j int) bool {
		if pop.NetworkAccesses[i].Port != pop.NetworkAccesses[j].Port {
			return pop.NetworkAccesses[i].Port < pop.NetworkAccesses[j].Port
		}
		return pop.NetworkAccesses[i].Direction < pop.NetworkAccesses[j].Direction
	})
	sort.Slice(pop.CapabilityAccesses, func(i, j int) bool { return pop.CapabilityAccesses[i].Name < pop.CapabilityAccesses[j].Name })
}

func addNetworkFact(pop *Population, value profile.NetworkAccess) {
	for i := range pop.NetworkAccesses {
		if pop.NetworkAccesses[i].Port == value.Port && pop.NetworkAccesses[i].Direction == value.Direction {
			return
		}
	}
	pop.NetworkAccesses = append(pop.NetworkAccesses, NetworkAccessRecord{Port: value.Port, Direction: value.Direction})
}

// ApplyContribution is the G6.3 crash-recoverable primitive. It accepts only
// explicit normalized facts; G6.4 owns any Observation-to-Contribution step.
func ApplyContribution(ctx context.Context, client dynamic.Interface, namespace string, c Contribution) (ContributionApplyResult, error) {
	if client == nil {
		return "", fmt.Errorf("%w: nil client", ErrInvalidContribution)
	}
	normalized, err := c.normalize()
	if err != nil {
		return "", err
	}
	contentDigest, err := normalized.contentDigest()
	if err != nil {
		return "", err
	}
	key := ContributionKey{ObservationID: normalized.ObservationID, Population: normalized.Population}
	receipts, err := NewReceiptStore(client)
	if err != nil {
		return "", err
	}
	historyName, err := resolveContributionHistoryName(ctx, client, namespace, normalized.Population)
	if err != nil {
		return "", err
	}
	receipt, rv, err := receipts.GetAfterInitialization(ctx, namespace, key)
	if err != nil {
		return "", err
	}
	if receipt == nil {
		created, createdRV, createErr := receipts.CreatePrepared(ctx, namespace, key, namespace, historyName, contentDigest)
		err = createErr
		rv = createdRV
		if err == nil {
			receipt = &created
		}
		if apierrors.IsAlreadyExists(err) {
			receipt, rv, err = receipts.GetAfterInitialization(ctx, namespace, key)
			if errors.Is(err, ErrReceiptIdentityMismatch) {
				return "", fmt.Errorf("%w: deterministic receipt name collision", ErrContributionKeyCollision)
			}
		}
		if err != nil {
			return "", err
		}
	}
	if receipt.ContentDigest != contentDigest {
		return "", fmt.Errorf("%w: expected %s, got %s", ErrContributionContentMismatch, receipt.ContentDigest, contentDigest)
	}
	if receipt.State == ReceiptCommitted {
		return ContributionAlreadyCommitted, nil
	}

	marker := ContributionMarker{ObservationID: normalized.ObservationID, Population: normalized.Population}
	marker.KeyDigest, err = key.Digest()
	if err != nil {
		return "", err
	}
	markerPresent, applied, err := applyHistoryEffect(ctx, client, namespace, historyName, key, normalized, marker)
	if err != nil {
		return "", err
	}
	if markerPresent {
		applied = false
	}
	committed, _, err := receipts.Commit(ctx, namespace, key, rv, contentDigest)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrReceiptCommitFailure, err)
	}
	if committed.State != ReceiptCommitted {
		return "", fmt.Errorf("%w: unexpected receipt state", ErrReceiptCommitFailure)
	}
	if err := cleanupContributionMarker(ctx, client, namespace, historyName, key, marker); err != nil {
		return ContributionCommittedCleanupPending, fmt.Errorf("%w: %v", ErrMarkerCleanupFailure, err)
	}
	if applied {
		return ContributionApplied, nil
	}
	return ContributionRecoveredAndCommitted, nil
}

func resolveContributionHistoryName(ctx context.Context, client dynamic.Interface, namespace string, fingerprint PopulationFingerprint) (string, error) {
	resource := client.Resource(trainingHistoryGVR).Namespace(namespace)
	identity, err := fingerprint.Identity()
	if err != nil {
		return "", err
	}
	if identity.Scope == ScopeContainer {
		return RecordNameContainerV2(identity)
	}
	v2Name := RecordNameV2(fingerprint.Container, fingerprint.BinaryPath)
	if _, err := resource.Get(ctx, v2Name, metav1.GetOptions{}); err == nil {
		return v2Name, nil
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}
	legacyName := RecordNameLegacy(fingerprint.Container, fingerprint.BinaryPath)
	if legacyName != v2Name {
		if _, err := resource.Get(ctx, legacyName, metav1.GetOptions{}); err == nil {
			return legacyName, nil
		} else if !apierrors.IsNotFound(err) {
			return "", err
		}
	}
	return v2Name, nil
}

func applyHistoryEffect(ctx context.Context, client dynamic.Interface, namespace, name string, key ContributionKey, c normalizedContribution, marker ContributionMarker) (markerPresent, applied bool, err error) {
	resource := client.Resource(trainingHistoryGVR).Namespace(namespace)
	contentDigest, err := c.contentDigest()
	if err != nil {
		return false, false, err
	}
	for attempt := 0; attempt < 5; attempt++ {
		obj, getErr := resource.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			obj = nil
		} else if getErr != nil {
			return false, false, getErr
		}
		var record *Record
		if obj != nil {
			record, err = fromUnstructured(obj)
			if err != nil {
				return false, false, err
			}
		} else {
			record = &Record{}
		}
		idx := -1
		for i := range record.Populations {
			if populationFingerprint(record.Populations[i]).Equal(key.Population) {
				idx = i
				break
			}
		}
		if idx < 0 {
			record.Populations = append(record.Populations, Population{Qualified: true, Scope: key.Population.Scope, Target: key.Population.Target, Container: key.Population.Container, ImageIdentity: key.Population.ImageIdentity, BinaryPath: key.Population.BinaryPath})
			idx = len(record.Populations) - 1
		}
		pop := &record.Populations[idx]
		for _, existing := range pop.PendingContributionMarkers {
			if existing.KeyDigest == marker.KeyDigest {
				return true, false, nil
			}
		}
		if len(pop.ObservationContributions) >= maxObservationContributions {
			return false, false, ErrAuditProvenanceCapacity
		}
		if len(pop.PendingContributionMarkers) >= maxPendingContributionMarkers {
			return false, false, ErrContributionMarkerCapacity
		}
		for _, existing := range pop.ObservationContributions {
			if existing.ObservationID == c.ObservationID {
				// A concurrent equivalent caller may have completed the
				// entire protocol, including marker cleanup, after this
				// caller's earlier receipt/marker decision.  Re-read the
				// authoritative receipt before treating the markerless
				// provenance as corruption.  Only an exact committed receipt
				// proves benign convergence; absent, prepared, or mismatched
				// receipts retain the fail-closed behavior.
				receipts, receiptErr := NewReceiptStore(client)
				if receiptErr != nil {
					return false, false, receiptErr
				}
				receipt, _, receiptErr := receipts.Get(ctx, namespace, key)
				if receiptErr != nil {
					return false, false, receiptErr
				}
				if receipt != nil && receipt.State == ReceiptCommitted {
					if receipt.ContentDigest != contentDigest {
						return false, false, fmt.Errorf("%w: committed receipt content mismatch", ErrContributionContentMismatch)
					}
					return true, false, nil
				}
				if receipt != nil && receipt.State == ReceiptPrepared {
					// The durable history update and receipt commit are separate
					// Kubernetes writes. A duplicate caller can observe the
					// history effect after the winning caller's marker cleanup but
					// before its receipt commit becomes visible. Re-read the
					// authoritative receipt within a bounded convergence window;
					// if it remains PREPARED, retain the fail-closed corruption
					// result below so crash recovery is not weakened.
					for attempt := 1; attempt < contributionConvergenceReads; attempt++ {
						timer := time.NewTimer(contributionConvergenceDelay)
						select {
						case <-ctx.Done():
							timer.Stop()
							return false, false, ctx.Err()
						case <-timer.C:
						}
						receipt, _, receiptErr = receipts.Get(ctx, namespace, key)
						if receiptErr != nil {
							return false, false, receiptErr
						}
						if receipt != nil && receipt.State == ReceiptCommitted {
							if receipt.ContentDigest != contentDigest {
								return false, false, fmt.Errorf("%w: committed receipt content mismatch", ErrContributionContentMismatch)
							}
							return true, false, nil
						}
					}
				}
				return false, false, fmt.Errorf("%w: %w", ErrInvalidContribution, errProvenanceWithoutMarker)
			}
		}
		addContributionFacts(pop, c)
		capabilityFacts := make([]string, 0, len(c.Capabilities))
		for _, capability := range c.Capabilities {
			capabilityFacts = append(capabilityFacts, capability.Name)
		}
		capabilityFactsComplete := false
		for _, source := range c.Sources {
			if source.Source == string(SourceCapabilities) {
				capabilityFactsComplete = source.EvidenceState != "UNKNOWN" && source.AttributionState == "COMPLETED" && source.BackendHealthy && source.AttachedForWindow && source.FlushConfirmed && source.ExcludedCount == 0
				break
			}
		}
		pop.ObservationContributions = append(pop.ObservationContributions, ObservationContribution{
			ObservationID: c.ObservationID, Sources: c.Sources,
			CapabilityFacts: capabilityFacts, CapabilityFactsComplete: capabilityFactsComplete,
		})
		pop.PendingContributionMarkers = append(pop.PendingContributionMarkers, marker)
		sortObservationMetadata(pop)
		out := toUnstructured(namespace, name, record)
		if obj == nil {
			if _, createErr := resource.Create(ctx, out, metav1.CreateOptions{}); createErr == nil {
				return false, true, nil
			} else if apierrors.IsAlreadyExists(createErr) {
				continue
			} else {
				return false, false, createErr
			}
		}
		out.SetResourceVersion(obj.GetResourceVersion())
		if _, updateErr := resource.Update(ctx, out, metav1.UpdateOptions{}); updateErr == nil {
			return false, true, nil
		} else if apierrors.IsConflict(updateErr) {
			continue
		} else {
			return false, false, updateErr
		}
	}
	return false, false, ErrHistoryCASExhausted
}

func cleanupContributionMarker(ctx context.Context, client dynamic.Interface, namespace, name string, key ContributionKey, marker ContributionMarker) error {
	resource := client.Resource(trainingHistoryGVR).Namespace(namespace)
	for attempt := 0; attempt < 5; attempt++ {
		obj, err := resource.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		record, err := fromUnstructured(obj)
		if err != nil {
			return err
		}
		idx := -1
		for i := range record.Populations {
			if populationFingerprint(record.Populations[i]).Equal(key.Population) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrHistoryPopulationNotFound
		}
		pop := &record.Populations[idx]
		markerIdx := -1
		for i := range pop.PendingContributionMarkers {
			if pop.PendingContributionMarkers[i].KeyDigest == marker.KeyDigest {
				markerIdx = i
				break
			}
		}
		if markerIdx < 0 {
			return nil
		}
		pop.PendingContributionMarkers = append(pop.PendingContributionMarkers[:markerIdx], pop.PendingContributionMarkers[markerIdx+1:]...)
		sortObservationMetadata(pop)
		out := toUnstructured(namespace, name, record)
		out.SetResourceVersion(obj.GetResourceVersion())
		if _, err := resource.Update(ctx, out, metav1.UpdateOptions{}); err == nil {
			return nil
		} else if !apierrors.IsConflict(err) {
			return err
		}
	}
	return ErrHistoryCASExhausted
}
