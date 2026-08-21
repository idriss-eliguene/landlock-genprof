// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package spobackend is the single place this project knows anything
// about security-profiles-operator's API shape.
//
// It exists because that shape has already moved once in a way that broke
// assumptions scattered across an exporter, a generic apply path, a
// readiness gate and a trace command: SeccompProfile was Namespaced
// through SPO v0.8.4 and became cluster-scoped at v0.9.0, which also
// dropped the namespace segment from the node-local profile path. A CRD's
// scope belongs to the whole CRD rather than to a version, so serving
// v1beta1 alongside v1 at v1.0.0 does not soften that — a conversion
// webhook maps between versions, never between scopes.
//
// So: API version, scope, GVK/GVR, path construction and parsing,
// lineage label keys and governed-object naming all live here, and
// generic governance code depends on this package instead of restating
// any of it. See docs/adr/0007 ("Backend contract, not scattered
// assumptions") and docs/adr/0008.
//
// Target: SPO v1.0.0 — SeccompProfile v1 (cluster-scoped),
// ProfileRecording v1 (namespaced). v1beta1 and the namespaced scope it
// was served under are deliberately out of scope.
package spobackend

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SPO API identity ---------------------------------------------------------

const (
	// Group is SPO's API group.
	Group = "security-profiles-operator.x-k8s.io"

	// Version is the API version this project targets. v1 is SPO's
	// storage version as of v1.0.0.
	Version = "v1"

	// APIVersion is the group/version string generated manifests carry.
	APIVersion = Group + "/" + Version

	// SeccompProfileKind is the kind this project generates and governs.
	SeccompProfileKind = "SeccompProfile"

	// ProfileRecordingKind is SPO's recording resource. Named here so the
	// ADR-0008 importer has one place to find it; nothing consumes it yet.
	ProfileRecordingKind = "ProfileRecording"
)

// SeccompProfileGVK identifies the generated/governed profile.
func SeccompProfileGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: Group, Version: Version, Kind: SeccompProfileKind}
}

// SeccompProfileGVR is the resource used for API operations.
func SeccompProfileGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: Group, Version: Version, Resource: "seccompprofiles"}
}

// SeccompProfileClusterScoped reports that SeccompProfile is cluster-scoped
// on the targeted API. Callers use this rather than assuming: applying a
// namespace to a cluster-scoped object is rejected by the API server, and
// omitting one from a namespaced object silently lands it in "default".
func SeccompProfileClusterScoped() bool { return true }

// ProfileRecordingGVR is namespaced — the scope change hit SeccompProfile,
// not ProfileRecording. That asymmetry is why lineage is carried by labels
// (below) rather than by comparing namespaces.
func ProfileRecordingGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: Group, Version: Version, Resource: "profilerecordings"}
}

// ProfileRecordingClusterScoped reports that ProfileRecording is namespaced.
func ProfileRecordingClusterScoped() bool { return false }

// ProfileRecording v1 enum values. v1alpha1 spelled these lowercase; the
// v1 forms are capitalised and are the ones to emit.
const (
	RecorderBpf             = "Bpf"
	RecorderLogs            = "Logs"
	MergeStrategyNone       = "None"
	MergeStrategyContainers = "Containers"
)

// Lineage labels SPO sets on every profile it generates from a recording.
// Consumed by the ADR-0008 importer; declared here so the label strings
// exist in exactly one place.
const (
	// RecordingIDLabel names the ProfileRecording that produced a profile.
	RecordingIDLabel = "spo.x-k8s.io/recording-id"

	// RecordingNamespaceLabel names that recording's namespace. SPO added
	// it specifically to disambiguate cluster-scoped profiles produced by
	// recordings sharing a name across namespaces, which is what makes
	// lineage checkable at all now that the profile itself has no
	// namespace.
	RecordingNamespaceLabel = "spo.x-k8s.io/recording-namespace"

	// ContainerIDLabel names the container a profile was recorded from.
	ContainerIDLabel = "spo.x-k8s.io/container-id"
)

// Node-local profile path --------------------------------------------------

const localhostProfilePrefix = "operator/"
const localhostProfileSuffix = ".json"

// LocalhostProfilePath returns the value a workload's
// securityContext.seccompProfile.localhostProfile must carry for the named
// profile, and the value SPO reports in status.localhostProfile once it has
// materialised the profile on the node.
//
// Cluster-scoped, so there is no namespace segment. Through SPO v0.8.4
// this was "operator/<namespace>/<name>.json"; that form belongs to an API
// this project no longer targets.
func LocalhostProfilePath(name string) string {
	return localhostProfilePrefix + name + localhostProfileSuffix
}

// ParseLocalhostProfilePath recovers the profile name from a path produced
// by LocalhostProfilePath. It reports ok=false for anything else —
// including the obsolete namespaced form, which must not be silently
// reinterpreted, since a workload referencing it names a profile this
// project cannot establish readiness for.
func ParseLocalhostProfilePath(path string) (name string, ok bool) {
	if !strings.HasPrefix(path, localhostProfilePrefix) || !strings.HasSuffix(path, localhostProfileSuffix) {
		return "", false
	}
	name = strings.TrimSuffix(strings.TrimPrefix(path, localhostProfilePrefix), localhostProfileSuffix)
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

// Governed profile naming --------------------------------------------------
//
// Normative: see docs/adr/0008, "Governed profile name". Every constant
// below is identity semantics. Changing any of them changes which cluster
// object a candidate refers to, so a change is a NameScheme bump, never a
// refactor.

const (
	// NameScheme is the governed-name scheme version, not an SPO API
	// version. It is embedded in the name so a future scheme can coexist
	// with this one during a migration: lg-v2-… can never collide with
	// lg-v1-….
	NameScheme = "v1"

	namePrefix = "lg-" + NameScheme + "-"

	// nameHashBytes is how much of the SHA-256 digest is kept, rendered as
	// twice as many hex characters.
	nameHashBytes = 8

	// namePodSegmentMax bounds the readable segment so the whole name fits
	// in a DNS-1123 label: 6 + 40 + 1 + 16 = 63.
	namePodSegmentMax = 40

	// nameHashDomain separates this digest from any other use of SHA-256
	// in this project.
	nameHashDomain = "lg-seccomp-name-v1"

	// nameFallbackSegment is used when a pod name sanitises to nothing.
	nameFallbackSegment = "workload"
)

// GovernedProfileName returns the cluster-unique name of the SeccompProfile
// this project generates for one workload container.
//
// The readable segment is derived from the pod name and carries no identity
// weight; identity comes from the digest, which is taken over a
// length-prefixed encoding so it is injective. Delimiter-joining would not
// be: "<namespace>-<pod>" maps ("a-b","c") and ("a","b-c") to one name, and
// in a cluster-wide name space that is a cross-namespace authority
// collision.
//
// The digest reduces collisions; it is not what makes them safe. Safety
// comes from the ownership annotations below, which record the identity
// tuple so the apply path can refuse rather than overwrite.
func GovernedProfileName(namespace, pod, container string) string {
	h := sha256.New()
	h.Write([]byte(nameHashDomain))
	h.Write([]byte{0})
	for _, part := range []string{namespace, pod, container} {
		// Guarded rather than assumed. This function is exported and takes
		// plain strings, so "a Kubernetes name is at most 253 bytes" is a
		// property of today's callers, not of this code — and the package's
		// own tests already pass a 520-byte pod name. If the length were
		// allowed to wrap, the prefix would no longer encode the true
		// length and the encoding would stop being injective: two distinct
		// tuples could then produce one governed name, which for a
		// cluster-scoped object is a cross-namespace authority collision.
		// Refuse to compute an identity we cannot stand behind.
		if uint64(len(part)) > math.MaxUint32 {
			panic("spobackend: identity component exceeds the length the canonical encoding can represent")
		}
		var length [4]byte
		// The bound above makes this conversion provably lossless; gosec
		// does not recognise the guard pattern, so the finding is
		// annotated rather than left to fail the build.
		// #nosec G115 -- length is checked against math.MaxUint32 immediately above
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		h.Write(length[:])
		h.Write([]byte(part))
	}
	digest := hex.EncodeToString(h.Sum(nil)[:nameHashBytes])

	return namePrefix + sanitizeNameSegment(pod) + "-" + digest
}

// sanitizeNameSegment reduces s to something usable inside a DNS-1123
// label, bounded by namePodSegmentMax.
func sanitizeNameSegment(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > namePodSegmentMax {
		out = strings.Trim(out[:namePodSegmentMax], "-")
	}
	if out == "" {
		return nameFallbackSegment
	}
	return out
}

// Ownership ----------------------------------------------------------------
//
// What these prove: under the cluster's RBAC trust boundary, this object
// was created by this project, for this identity tuple, under this name
// scheme. What they do NOT prove: anything about authenticity. Any
// principal allowed to write cluster-scoped SeccompProfiles can write
// these annotations too. RBAC is the trust boundary; this is collision
// protection, not provenance cryptography (docs/adr/0008).

const (
	ManagedByAnnotation       = "landlockgenprof.io/managed-by"
	NameSchemeAnnotation      = "landlockgenprof.io/name-scheme"
	TargetNamespaceAnnotation = "landlockgenprof.io/target-namespace"
	TargetPodAnnotation       = "landlockgenprof.io/target-pod"
	TargetContainerAnnotation = "landlockgenprof.io/target-container"

	// ManagedByValue is the marker value.
	ManagedByValue = "landlock-genprof"
)

// OwnershipAnnotations returns the annotations a governed SeccompProfile
// carries. The identity tuple is recorded because the name cannot be
// inverted — the digest is one-way and the readable segment is lossy — so
// without it the apply path could not tell "ours, same workload" from
// "ours, different workload that happens to collide".
func OwnershipAnnotations(namespace, pod, container string) map[string]string {
	return map[string]string{
		ManagedByAnnotation:       ManagedByValue,
		NameSchemeAnnotation:      NameScheme,
		TargetNamespaceAnnotation: namespace,
		TargetPodAnnotation:       pod,
		TargetContainerAnnotation: container,
	}
}

// OwnershipVerdict is the result of inspecting an object that already
// occupies a governed name.
type OwnershipVerdict int

const (
	// OwnedSameTarget — ours, same scheme, same workload. Safe to update,
	// which is what makes a retry after a readiness timeout work.
	OwnedSameTarget OwnershipVerdict = iota
	// OwnedDifferentTarget — ours, but recorded against a different
	// workload or a different name scheme. The digest-collision and
	// scheme-migration case. Fail closed.
	OwnedDifferentTarget
	// NotOwned — no marker. Never overwrite. Fail closed.
	NotOwned
)

func (v OwnershipVerdict) String() string {
	switch v {
	case OwnedSameTarget:
		return "owned by landlock-genprof for this workload"
	case OwnedDifferentTarget:
		return "owned by landlock-genprof but recorded against a different workload or name scheme"
	default:
		return "not managed by landlock-genprof"
	}
}

// ClassifyOwnership inspects an existing object's annotations against the
// workload a candidate governs.
func ClassifyOwnership(annotations map[string]string, namespace, pod, container string) OwnershipVerdict {
	if annotations[ManagedByAnnotation] != ManagedByValue {
		return NotOwned
	}
	if annotations[NameSchemeAnnotation] != NameScheme ||
		annotations[TargetNamespaceAnnotation] != namespace ||
		annotations[TargetPodAnnotation] != pod ||
		annotations[TargetContainerAnnotation] != container {
		return OwnedDifferentTarget
	}
	return OwnedSameTarget
}

// Describe renders the backend target, for diagnostics that would
// otherwise restate the API shape at the call site.
func Describe() string {
	scope := "namespaced"
	if SeccompProfileClusterScoped() {
		scope = "cluster-scoped"
	}
	return fmt.Sprintf("%s %s (%s)", APIVersion, SeccompProfileKind, scope)
}

// Source-profile state ------------------------------------------------------
//
// Everything below is read from SPO's own API rather than assumed, because
// docs/adr/0008 makes the import gates normative but deliberately delegates
// the exact strings here. Verified against SPO v1.0.0 source:
// api/profilebase/v1/profilebase.go and
// internal/pkg/daemon/profilerecorder/profilerecorder.go.

const (
	// PartialLabel marks a profile SPO has not yet merged. SPO's own
	// IsPartial tests only for the key's PRESENCE and ignores its value
	// (profilebase.IsPartial), so a profile labelled "false" is still
	// partial to SPO. Matching that exactly matters: treating
	// partial="false" as complete would import a fragment of the recorded
	// authority as if it were all of it.
	PartialLabel = "spo.x-k8s.io/partial"

	// UnmergedProfilesFinalizer is set on a ProfileRecording while partial
	// profiles are still outstanding. It is a finalizer, not a label.
	UnmergedProfilesFinalizer = "spo.x-k8s.io/has-unmerged-profiles"
)

// SpecState values. This is the modern shape of what ADR-0008 called
// "disabled: true": at v1 inertness is an enum field, spec.state, not a
// boolean. The property the ADR relies on — a recorded profile that SPO
// will not reconcile onto nodes — is unchanged.
const (
	SpecStateEnabled  = "Enabled"
	SpecStateDisabled = "Disabled"
)

// IsPartial reports whether SPO considers a profile partial, using SPO's
// own presence-only semantics.
func IsPartial(labels map[string]string) bool {
	_, ok := labels[PartialLabel]
	return ok
}

// IsInert reports whether a source profile is in the state
// disableProfileAfterRecording produces — SPO will not reconcile it onto
// any node. Import requires this so the recorded authority cannot be
// enforcing anything before a human has authorized it.
func IsInert(specState string) bool { return specState == SpecStateDisabled }

// SyscallCoverageAnnotation is where ADR-0008 expects SPO to report how
// many recording units contributed each syscall.
//
// SPO v1.0.0 does not set it — verified by exhaustive search of the v1.0.0
// tree, which contains no such key. The importer therefore records coverage
// as CoverageUnknown in practice today. The constant exists because the ADR
// requires coverage to be carried verbatim if it is ever present, and
// because "absent" must stay distinguishable from "zero": absent coverage
// is unknown, never a quantity, and never a confidence.
const SyscallCoverageAnnotation = "spo.x-k8s.io/syscall-coverage"

// CoverageUnknown is the explicit token recorded when SPO reports no
// coverage. It is a string, not a number, so it cannot be arithmetic'd into
// a frequency or a tier by accident.
const CoverageUnknown = "unknown"

// Seccomp provenance -------------------------------------------------------
//
// Provenance rides inside the governed artifact as annotations, so it is
// covered by CandidateDigest: a falsified provenance claim invalidates the
// approval rather than riding alongside it (docs/adr/0008, "Provenance").

const (
	// SeccompSourceAnnotation records which source produced the seccomp
	// authority. Present in BOTH modes — that is what makes a switch from
	// SPO to internal change the artifact, hence the digest, hence stale
	// any approval bound to the previous source.
	SeccompSourceAnnotation = "landlockgenprof.io/seccomp-source"

	// SeccompOriginAnnotation distinguishes the epistemic kind: policy
	// derived by another system versus this project's own observation.
	SeccompOriginAnnotation = "landlockgenprof.io/seccomp-origin"

	// Source profile identity. Cluster-scoped, so there is no namespace to
	// record for the profile itself.
	SourceProfileAnnotation = "landlockgenprof.io/spo-source-profile"

	// The recording the source profile came from, and the container it was
	// recorded from — the lineage tuple, preserved so a reviewer can see
	// what was checked.
	SourceRecordingAnnotation          = "landlockgenprof.io/spo-recording-name"
	SourceRecordingNamespaceAnnotation = "landlockgenprof.io/spo-recording-namespace"
	SourceContainerAnnotation          = "landlockgenprof.io/spo-container-id"
	SourceKindAnnotation               = "landlockgenprof.io/spo-source-kind"
	SourceDerivationAnnotation         = "landlockgenprof.io/spo-derivation"
	SourceMergeStrategyAnnotation      = "landlockgenprof.io/spo-merge-strategy"
	ContributorLineageAnnotation       = "landlockgenprof.io/spo-contributor-lineage"

	// Coverage verbatim as SPO reported it, or CoverageUnknown.
	SourceCoverageAnnotation = "landlockgenprof.io/spo-syscall-coverage"
)

const (
	SourceKindProfileRecording    = "ProfileRecording"
	SourceDerivationMerged        = "merged"
	ContributorLineageUnavailable = "unavailable"
)

// Seccomp source kinds. Selection is explicit and never inferred from
// cluster state (INV-SPO-IMPORT-06).
const (
	SeccompSourceInternal = "internal"
	SeccompSourceSPO      = "spo"
)

// Seccomp origin kinds, naming the semantic class from ADR-0008's table.
const (
	// SeccompOriginObserved is this project's own synthesis. Its syscalls
	// come from our tracer, carry TrainingHistory frequency, and have a
	// confidence tier.
	SeccompOriginObserved = "observed"

	// SeccompOriginDerived is policy produced by another system from
	// observation it did not hand us. Occurrences are gone, so no
	// frequency and no confidence tier exist — and none may be invented.
	SeccompOriginDerived = "derived"
)

// InternalSeccompProvenance returns the provenance annotations for a
// seccomp artifact this project synthesised from its own observation.
func InternalSeccompProvenance() map[string]string {
	return map[string]string{
		SeccompSourceAnnotation: SeccompSourceInternal,
		SeccompOriginAnnotation: SeccompOriginObserved,
	}
}

// SPOSeccompProvenance returns the provenance annotations for a seccomp
// artifact imported from an SPO-generated profile. Every value here was
// verified during import, so the annotations record what was checked, not
// what was claimed.
func SPOSeccompProvenance(sourceProfile, recordingNamespace, recordingName, container, coverage string) map[string]string {
	if coverage == "" {
		coverage = CoverageUnknown
	}
	return map[string]string{
		SeccompSourceAnnotation:            SeccompSourceSPO,
		SeccompOriginAnnotation:            SeccompOriginDerived,
		SourceProfileAnnotation:            sourceProfile,
		SourceRecordingNamespaceAnnotation: recordingNamespace,
		SourceRecordingAnnotation:          recordingName,
		SourceContainerAnnotation:          container,
		SourceCoverageAnnotation:           coverage,
	}
}

// SPOMergedSeccompProvenance records ADR-0009's deliberately weaker,
// recording-level provenance. It never manufactures a source container or
// contributor set. Target identity is added independently by OwnershipAnnotations.
func SPOMergedSeccompProvenance(sourceProfile, recordingNamespace, recordingName, normalizedCoverage string) map[string]string {
	return map[string]string{
		SeccompSourceAnnotation:            SeccompSourceSPO,
		SeccompOriginAnnotation:            SeccompOriginDerived,
		SourceProfileAnnotation:            sourceProfile,
		SourceKindAnnotation:               SourceKindProfileRecording,
		SourceRecordingNamespaceAnnotation: recordingNamespace,
		SourceRecordingAnnotation:          recordingName,
		SourceDerivationAnnotation:         SourceDerivationMerged,
		SourceMergeStrategyAnnotation:      MergeStrategyContainers,
		ContributorLineageAnnotation:       ContributorLineageUnavailable,
		SourceCoverageAnnotation:           normalizedCoverage,
	}
}
