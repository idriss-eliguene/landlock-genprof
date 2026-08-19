// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package spoimport implements docs/adr/0008: importing a
// security-profiles-operator SeccompProfile as DERIVED POLICY.
//
// The one sentence that determines everything else here: an SPO-generated
// profile is not observation. It carries syscall names with no timestamps
// and no occurrence counts, so it can never honestly acquire seenInRuns, a
// frequency, or a confidence tier. This package therefore terminates in an
// artifact and has no route into internal/evidence, internal/history or
// confidenceFor — not by convention, but because it imports none of them
// and returns no type any of them accept.
//
// What it produces is a snapshot: content copied out of the source object,
// never a reference to it. Mutating or deleting the source afterwards
// cannot change an approved candidate, because the approved candidate does
// not point at the source.
//
// Every gate below fails closed. There is no advisory mode, no
// best-effort import, and no fallback to internal synthesis — a caller who
// asked for SPO and cannot have it gets an error, never a quietly
// different profile than the one they asked to govern.
package spoimport

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
)

// ErrImport is the class of every fail-closed refusal in this package.
// Callers map it to a blocking exit; nothing here is advisory.
var ErrImport = errors.New("spo import refused")

// Source names the SPO material explicitly. Both fields are supplied by
// the operator and neither is discovered: ADR-0008 forbids scanning the
// cluster for a profile that looks plausible, because "looks plausible" is
// exactly how one workload's authority gets applied to another.
type Source struct {
	// RecordingName is the ProfileRecording that produced the profile.
	// ProfileRecording is namespaced and lives in Target.Namespace.
	RecordingName string

	// ProfileName is the SPO-generated SeccompProfile. Cluster-scoped, so
	// there is no namespace to give.
	ProfileName string
}

// Target is the workload being governed.
type Target struct {
	Namespace string
	Pod       string
	Container string
}

// Result is the immutable outcome of an import. Profile holds copied
// enforcement content; Provenance records what was verified to get it.
// Neither references the source object.
type Result struct {
	Profile    *seccomp.Profile
	Provenance map[string]string
}

// Supported enforcement content — a CLOSED allow-list, not a deny-list of
// known-bad fields. A field SPO adds after this was written is refused
// rather than silently dropped, which is what makes the boundary
// forward-safe: dropping baseProfileName narrows authority and dropping
// syscalls[].args widens it, and both break the property that the reviewed
// source and the enforced copy mean the same thing.
//
// "state" is listed because it is lifecycle, not enforcement: it is
// permitted in the source, deliberately checked, and deliberately not
// copied — inheriting Disabled would produce an approved artifact that
// enforces nothing.
var (
	supportedSpecFields = map[string]bool{
		"defaultAction": true,
		"architectures": true,
		"syscalls":      true,
		"state":         true,
	}
	supportedSyscallFields = map[string]bool{
		"names":  true,
		"action": true,
	}
)

// Import fetches the named recording and profile and snapshots the profile
// if every gate passes.
//
// The recording is fetched even though lineage is checked against the
// profile's labels, for two reasons: its existence is what makes the
// operator's assertion checkable rather than decorative, and its
// disableProfileAfterRecording setting is what guarantees the source stays
// inert rather than merely being inert at this instant.
func Import(ctx context.Context, dyn dynamic.Interface, src Source, tgt Target) (*Result, error) {
	if err := validateRequest(src, tgt); err != nil {
		return nil, err
	}

	recording, err := dyn.Resource(spobackend.ProfileRecordingGVR()).
		Namespace(tgt.Namespace).Get(ctx, src.RecordingName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: ProfileRecording %s/%s not found; the recording naming the source must exist so its lineage can be verified (is security-profiles-operator installed, and was the recording created in the target namespace?)",
				ErrImport, tgt.Namespace, src.RecordingName)
		}
		return nil, fmt.Errorf("%w: reading ProfileRecording %s/%s: %w", ErrImport, tgt.Namespace, src.RecordingName, err)
	}

	profile, err := dyn.Resource(spobackend.SeccompProfileGVR()).
		Get(ctx, src.ProfileName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: SeccompProfile %s not found; it is cluster-scoped on the targeted API, so no namespace applies (has SPO finished generating it?)",
				ErrImport, src.ProfileName)
		}
		return nil, fmt.Errorf("%w: reading SeccompProfile %s: %w", ErrImport, src.ProfileName, err)
	}

	return Snapshot(recording, profile, src, tgt)
}

// Snapshot applies every ADR-0008 gate to already-fetched objects and
// copies the supported enforcement content out.
//
// Separated from Import so the gates are testable without a cluster, and
// so it is visibly true that nothing here reads back from the API after
// the objects were fetched — the snapshot is taken from these bytes.
func Snapshot(recording, profile *unstructured.Unstructured, src Source, tgt Target) (*Result, error) {
	if err := validateRequest(src, tgt); err != nil {
		return nil, err
	}
	if recording == nil || profile == nil {
		return nil, fmt.Errorf("%w: source objects missing", ErrImport)
	}

	if err := checkAPIShape(profile); err != nil {
		return nil, err
	}
	if err := checkInert(recording, profile, src); err != nil {
		return nil, err
	}
	if err := checkComplete(recording, profile, src); err != nil {
		return nil, err
	}
	if err := checkLineage(profile, src, tgt); err != nil {
		return nil, err
	}

	spec, err := enforcementContent(profile)
	if err != nil {
		return nil, err
	}

	return &Result{
		Profile: spec,
		Provenance: spobackend.SPOSeccompProvenance(
			profile.GetName(),
			tgt.Namespace,
			src.RecordingName,
			tgt.Container,
			coverage(profile),
		),
	}, nil
}

// validateRequest rejects an under-specified import before it touches the
// cluster. Ambiguity is never resolved by guessing, so an unnamed source is
// a refusal rather than a search.
func validateRequest(src Source, tgt Target) error {
	var missing []string
	if src.RecordingName == "" {
		missing = append(missing, "ProfileRecording name")
	}
	if src.ProfileName == "" {
		missing = append(missing, "source SeccompProfile name")
	}
	if tgt.Namespace == "" {
		missing = append(missing, "target namespace")
	}
	if tgt.Container == "" {
		missing = append(missing, "target container")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: ambiguous source selection — %s must be named explicitly; ADR-0008 forbids inferring the source from cluster state",
			ErrImport, strings.Join(missing, ", "))
	}
	return nil
}

// checkAPIShape refuses anything that is not the API this project targets.
// v1beta1 and the namespaced scope it was served under through SPO v0.8.4
// are deliberately out of scope: a namespaced source would imply a
// namespaced localhostProfile path, and one backend contract is what keeps
// CandidateDigest from becoming a function of which SPO happens to be
// installed.
func checkAPIShape(profile *unstructured.Unstructured) error {
	if got := profile.GetAPIVersion(); got != spobackend.APIVersion {
		return fmt.Errorf("%w: source SeccompProfile %s has apiVersion %q, want %q; this project targets modern SPO only",
			ErrImport, profile.GetName(), got, spobackend.APIVersion)
	}
	if got := profile.GetKind(); got != spobackend.SeccompProfileKind {
		return fmt.Errorf("%w: source object %s is a %s, want %s",
			ErrImport, profile.GetName(), got, spobackend.SeccompProfileKind)
	}
	if ns := profile.GetNamespace(); ns != "" {
		return fmt.Errorf("%w: source SeccompProfile %s carries namespace %q, but SeccompProfile is cluster-scoped on the targeted API; a namespaced object belongs to SPO v0.8.4 and is not importable",
			ErrImport, profile.GetName(), ns)
	}
	return nil
}

// checkInert enforces the safety property ADR-0008 leans on: the recorded
// profile must not be enforcing anything before a human has authorized it.
//
// Both halves are required. The recording's disableProfileAfterRecording is
// what makes inertness durable — without it the profile could be reconciled
// at any moment by SPO. The profile's own state is what makes it true right
// now. Checking only the first would trust a setting over the object;
// checking only the second would pass a profile that is momentarily
// disabled and about to be enabled.
func checkInert(recording, profile *unstructured.Unstructured, src Source) error {
	disable, found, err := unstructured.NestedBool(recording.Object, "spec", "disableProfileAfterRecording")
	if err != nil {
		return fmt.Errorf("%w: reading disableProfileAfterRecording on ProfileRecording %s: %w", ErrImport, src.RecordingName, err)
	}
	if !found || !disable {
		return fmt.Errorf("%w: ProfileRecording %s does not set disableProfileAfterRecording: true, so its generated profile is not guaranteed to stay inert before approval; re-record with it enabled",
			ErrImport, src.RecordingName)
	}

	state, _, err := unstructured.NestedString(profile.Object, "spec", "state")
	if err != nil {
		return fmt.Errorf("%w: reading spec.state on SeccompProfile %s: %w", ErrImport, profile.GetName(), err)
	}
	if !spobackend.IsInert(state) {
		return fmt.Errorf("%w: source SeccompProfile %s has spec.state %q, want %q; an enforcing source profile means SPO is already applying unreviewed authority to nodes",
			ErrImport, profile.GetName(), state, spobackend.SpecStateDisabled)
	}
	return nil
}

// checkComplete refuses a profile that does not represent the whole
// recorded authority. A partial profile is one fragment of a union, so
// importing it would under-permit — the CrashLoop class, arriving through
// the import path.
func checkComplete(recording, profile *unstructured.Unstructured, src Source) error {
	if spobackend.IsPartial(profile.GetLabels()) {
		return fmt.Errorf("%w: source SeccompProfile %s carries the %s label, so SPO considers it an unmerged fragment; import the merged profile instead",
			ErrImport, profile.GetName(), spobackend.PartialLabel)
	}
	for _, f := range recording.GetFinalizers() {
		if f == spobackend.UnmergedProfilesFinalizer {
			return fmt.Errorf("%w: ProfileRecording %s still carries the %s finalizer, so partial profiles are outstanding and the merge has not completed; delete the recording to trigger the merge and retry",
				ErrImport, src.RecordingName, spobackend.UnmergedProfilesFinalizer)
		}
	}
	return nil
}

// checkLineage is the defence against cross-workload authority
// substitution — accepting a perfectly valid profile that belongs to a
// different workload, which would silently authorize unrelated syscalls.
//
// Read from SPO's own labels, never parsed from the object's name. The
// strength ceiling is structural and stated rather than hidden: these
// labels bind by name and namespace, not by UID, because Kubernetes cannot
// express a cluster-scoped dependent owned by a namespaced owner. A
// principal that can write cluster-scoped SeccompProfiles can forge them.
// RBAC is the trust boundary; this is not authentication.
func checkLineage(profile *unstructured.Unstructured, src Source, tgt Target) error {
	labels := profile.GetLabels()

	// Absent entirely is its own message: it usually means the object was
	// not produced by a recording at all, which no amount of comparing
	// would reveal.
	_, hasNS := labels[spobackend.RecordingNamespaceLabel]
	_, hasID := labels[spobackend.RecordingIDLabel]
	_, hasCtr := labels[spobackend.ContainerIDLabel]
	if !hasNS && !hasID && !hasCtr {
		return fmt.Errorf("%w: source SeccompProfile %s carries none of SPO's recording labels (%s, %s, %s), so its lineage cannot be established; it was probably not generated by a ProfileRecording",
			ErrImport, profile.GetName(),
			spobackend.RecordingNamespaceLabel, spobackend.RecordingIDLabel, spobackend.ContainerIDLabel)
	}

	for _, check := range []struct {
		label, want, what string
		present           bool
	}{
		{spobackend.RecordingNamespaceLabel, tgt.Namespace, "recording namespace", hasNS},
		{spobackend.RecordingIDLabel, src.RecordingName, "recording", hasID},
		{spobackend.ContainerIDLabel, tgt.Container, "container", hasCtr},
	} {
		if !check.present {
			return fmt.Errorf("%w: source SeccompProfile %s has no %s label, so its %s lineage cannot be verified; SPO's merged profiles omit %s, and a profile whose container cannot be confirmed must not be imported",
				ErrImport, profile.GetName(), check.label, check.what, spobackend.ContainerIDLabel)
		}
		if got := labels[check.label]; got != check.want {
			return fmt.Errorf("%w: source SeccompProfile %s has %s=%q, want %q; importing it would apply another workload's recorded authority to %s/%s",
				ErrImport, profile.GetName(), check.label, got, check.want, tgt.Namespace, tgt.Pod)
		}
	}
	return nil
}

// enforcementContent copies the supported enforcement semantics out of the
// source spec, refusing anything it cannot represent exactly.
//
// Represent exactly or reject. Refusing is honest and actionable; a silent
// copy is neither, because the reviewer would authorize one meaning while
// the cluster enforced another.
func enforcementContent(profile *unstructured.Unstructured) (*seccomp.Profile, error) {
	spec, found, err := unstructured.NestedMap(profile.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("%w: source SeccompProfile %s has no readable spec", ErrImport, profile.GetName())
	}

	if unsupported := unsupportedKeys(spec, supportedSpecFields); len(unsupported) > 0 {
		return nil, fmt.Errorf("%w: source SeccompProfile %s carries enforcement-relevant field(s) this project cannot represent: %s. Dropping them would change what is enforced relative to what was reviewed, so the import refuses rather than copies a different meaning",
			ErrImport, profile.GetName(), strings.Join(unsupported, ", "))
	}

	defaultAction, _, err := unstructured.NestedString(spec, "defaultAction")
	if err != nil {
		return nil, fmt.Errorf("%w: reading defaultAction on %s: %w", ErrImport, profile.GetName(), err)
	}
	if defaultAction == "" {
		return nil, fmt.Errorf("%w: source SeccompProfile %s has no defaultAction; without it the profile does not define what happens to unlisted syscalls and is not a usable candidate",
			ErrImport, profile.GetName())
	}

	architectures, _, err := unstructured.NestedStringSlice(spec, "architectures")
	if err != nil {
		return nil, fmt.Errorf("%w: reading architectures on %s: %w", ErrImport, profile.GetName(), err)
	}

	rawSyscalls, _, err := unstructured.NestedSlice(spec, "syscalls")
	if err != nil {
		return nil, fmt.Errorf("%w: reading syscalls on %s: %w", ErrImport, profile.GetName(), err)
	}

	rules := make([]seccomp.SyscallRule, 0, len(rawSyscalls))
	for i, item := range rawSyscalls {
		rule, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: source SeccompProfile %s has a malformed syscalls[%d]", ErrImport, profile.GetName(), i)
		}
		if unsupported := unsupportedKeys(rule, supportedSyscallFields); len(unsupported) > 0 {
			return nil, fmt.Errorf("%w: source SeccompProfile %s syscalls[%d] carries field(s) this project cannot represent: %s. args in particular widens authority if dropped — a rule permitted only for specific argument values would become unconditional",
				ErrImport, profile.GetName(), i, strings.Join(unsupported, ", "))
		}

		names, _, err := unstructured.NestedStringSlice(rule, "names")
		if err != nil {
			return nil, fmt.Errorf("%w: reading syscalls[%d].names on %s: %w", ErrImport, i, profile.GetName(), err)
		}
		action, _, err := unstructured.NestedString(rule, "action")
		if err != nil {
			return nil, fmt.Errorf("%w: reading syscalls[%d].action on %s: %w", ErrImport, i, profile.GetName(), err)
		}
		if action == "" {
			return nil, fmt.Errorf("%w: source SeccompProfile %s syscalls[%d] has no action", ErrImport, profile.GetName(), i)
		}
		rules = append(rules, seccomp.SyscallRule{Names: names, Action: action})
	}

	return &seccomp.Profile{
		DefaultAction: defaultAction,
		Architectures: architectures,
		Syscalls:      rules,
	}, nil
}

// unsupportedKeys returns the keys of got that allowed does not cover,
// sorted so the refusal message is deterministic and therefore assertable.
func unsupportedKeys(got map[string]interface{}, allowed map[string]bool) []string {
	var out []string
	for key := range got {
		if !allowed[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// coverage reads SPO's syscall-coverage annotation verbatim, or reports it
// unknown.
//
// SPO v1.0.0 does not set this annotation at all, so "unknown" is what real
// imports record today. Absent must never become 0, "full", or a tier:
// coverage means how many recording units contributed a syscall, which is
// not frequency over training runs and is not authorization strength.
func coverage(profile *unstructured.Unstructured) string {
	if v := profile.GetAnnotations()[spobackend.SyscallCoverageAnnotation]; v != "" {
		return v
	}
	return spobackend.CoverageUnknown
}
