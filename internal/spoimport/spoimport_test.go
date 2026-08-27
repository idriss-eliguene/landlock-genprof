// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package spoimport

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
)

// Every gate in this package is a refusal, so the tests are mostly
// counterexamples: each one takes a source that would import cleanly and
// breaks exactly one property, to prove that property is what the gate is
// actually checking rather than something incidental passing alongside it.

func testTarget() Target {
	return Target{Namespace: "prod", Pod: "nginx-demo", Container: "tools"}
}

func testSource() Source {
	return Source{RecordingName: "nginx-rec", ProfileName: "nginx-rec-tools"}
}

func mergedSource() Source {
	return Source{Mode: ModeMergedProvenance, RecordingNamespace: "observations", RecordingName: "nginx-rec", ProfileName: "nginx-rec-tools"}
}

func mergedRecording() *unstructured.Unstructured {
	r := validRecording()
	r.SetNamespace("observations")
	_ = unstructured.SetNestedField(r.Object, spobackend.MergeStrategyContainers, "spec", "mergeStrategy")
	return r
}

func mergedProfile() *unstructured.Unstructured {
	p := validProfile()
	setLabel(p, spobackend.RecordingNamespaceLabel, "observations")
	setLabel(p, spobackend.ContainerIDLabel, nil)
	return p
}

// validRecording is a ProfileRecording that satisfies every gate.
func validRecording() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": spobackend.APIVersion,
		"kind":       spobackend.ProfileRecordingKind,
		"metadata": map[string]interface{}{
			"name":      "nginx-rec",
			"namespace": "prod",
		},
		"spec": map[string]interface{}{
			"kind":                         "SeccompProfile",
			"recorder":                     spobackend.RecorderBpf,
			"mergeStrategy":                spobackend.MergeStrategyNone,
			"disableProfileAfterRecording": true,
		},
	}}
}

// validProfile is an SPO-generated SeccompProfile that satisfies every
// gate: modern API, cluster-scoped, inert, complete, correct lineage, and
// only representable enforcement content.
func validProfile() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": spobackend.APIVersion,
		"kind":       spobackend.SeccompProfileKind,
		"metadata": map[string]interface{}{
			"name": "nginx-rec-tools",
			"labels": map[string]interface{}{
				spobackend.RecordingIDLabel:        "nginx-rec",
				spobackend.RecordingNamespaceLabel: "prod",
				spobackend.ContainerIDLabel:        "tools",
			},
		},
		"spec": map[string]interface{}{
			"state":         spobackend.SpecStateDisabled,
			"defaultAction": "SCMP_ACT_ERRNO",
			"architectures": []interface{}{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86"},
			"syscalls": []interface{}{
				map[string]interface{}{
					"names":  []interface{}{"openat", "read", "write"},
					"action": "SCMP_ACT_ALLOW",
				},
				map[string]interface{}{
					"names":  []interface{}{"ptrace"},
					"action": "SCMP_ACT_LOG",
				},
			},
		},
	}}
}

// setSpec mutates one spec field on a copy-in-place object, for building
// counterexamples from the valid fixture.
func setSpec(obj *unstructured.Unstructured, key string, value interface{}) *unstructured.Unstructured {
	spec := obj.Object["spec"].(map[string]interface{})
	if value == nil {
		delete(spec, key)
	} else {
		spec[key] = value
	}
	return obj
}

func setLabel(obj *unstructured.Unstructured, key string, value interface{}) *unstructured.Unstructured {
	meta := obj.Object["metadata"].(map[string]interface{})
	labels, ok := meta["labels"].(map[string]interface{})
	if !ok {
		labels = map[string]interface{}{}
		meta["labels"] = labels
	}
	if value == nil {
		delete(labels, key)
	} else {
		labels[key] = value
	}
	return obj
}

func setAnnotation(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	return obj
}

func mustSnapshot(t *testing.T) *Result {
	t.Helper()
	got, err := Snapshot(validRecording(), validProfile(), testSource(), testTarget())
	if err != nil {
		t.Fatalf("Snapshot() on a valid source error = %v", err)
	}
	return got
}

// --- Happy path ------------------------------------------------------------

func TestSnapshot_ValidModernSource(t *testing.T) {
	got := mustSnapshot(t)

	if got.Profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("DefaultAction = %q, want SCMP_ACT_ERRNO", got.Profile.DefaultAction)
	}
	if want := []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86"}; !reflect.DeepEqual(got.Profile.Architectures, want) {
		t.Errorf("Architectures = %v, want %v", got.Profile.Architectures, want)
	}
}

func TestSnapshot_ExplicitMergedProvenance(t *testing.T) {
	got, err := Snapshot(mergedRecording(), mergedProfile(), mergedSource(), testTarget())
	if err != nil {
		t.Fatalf("Snapshot() merged source error = %v", err)
	}
	if got.Provenance[spobackend.SourceDerivationAnnotation] != spobackend.SourceDerivationMerged ||
		got.Provenance[spobackend.SourceMergeStrategyAnnotation] != spobackend.MergeStrategyContainers ||
		got.Provenance[spobackend.ContributorLineageAnnotation] != spobackend.ContributorLineageUnavailable {
		t.Fatalf("merged provenance = %#v", got.Provenance)
	}
	if _, fabricated := got.Provenance[spobackend.SourceContainerAnnotation]; fabricated {
		t.Fatal("merged provenance fabricated a unique source container")
	}
	if got.Profile.Syscalls[0].Names[0] != "openat" {
		t.Fatal("merged import did not preserve exact SeccompProfile policy")
	}
}

func TestSnapshot_MergedCoverageStatesDoNotBlockPolicyImport(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      CoverageState
	}{
		{"known", `{"version":"v1","total":2,"syscalls":{"read":2}}`, CoverageKnown},
		{"malformed", `{`, CoverageMalformed},
		{"unsupported", `{"version":"v2","total":2,"syscalls":{}}`, CoverageUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := setAnnotation(mergedProfile(), spobackend.SyscallCoverageAnnotation, tc.raw)
			got, err := Snapshot(mergedRecording(), profile, mergedSource(), testTarget())
			if err != nil {
				t.Fatalf("Snapshot() blocked valid policy because optional coverage was %s: %v", tc.want, err)
			}
			coverage := ParseCanonicalCoverage(got.Provenance[spobackend.SourceCoverageAnnotation])
			if coverage.State != tc.want {
				t.Fatalf("coverage state = %s, want %s", coverage.State, tc.want)
			}
			if got.Provenance[spobackend.ContributorLineageAnnotation] != spobackend.ContributorLineageUnavailable {
				t.Fatal("coverage changed contributor lineage")
			}
		})
	}
}

func TestSnapshot_MergedCoverageDoesNotRetainRawBytes(t *testing.T) {
	raw := "{\n  \"syscalls\": {\"write\": 1, \"read\": 2},\n  \"total\": 2,\n  \"version\": \"v1\"\n}"
	profile := setAnnotation(mergedProfile(), spobackend.SyscallCoverageAnnotation, raw)
	got, err := Snapshot(mergedRecording(), profile, mergedSource(), testTarget())
	if err != nil {
		t.Fatal(err)
	}
	stored := got.Provenance[spobackend.SourceCoverageAnnotation]
	if stored == raw || strings.Contains(stored, "\n") {
		t.Fatalf("raw SPO coverage bytes reached governed provenance: %q", stored)
	}
	want := ParseCoverage(raw, true).Canonical()
	if stored != want {
		t.Fatalf("stored coverage = %q, want canonical %q", stored, want)
	}
}

func TestSnapshot_MergedModeMaySnapshotAfterRecordingDeletion(t *testing.T) {
	got, err := Snapshot(nil, mergedProfile(), mergedSource(), testTarget())
	if err != nil || got == nil {
		t.Fatalf("Snapshot(nil recording) = (%v, %v), want governed snapshot", got, err)
	}
}

func TestSnapshot_MergedProvenanceFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  Source
		rec  *unstructured.Unstructured
		prof *unstructured.Unstructured
		tgt  Target
	}{
		{"missing recording identity", Source{Mode: ModeMergedProvenance, RecordingNamespace: "observations", ProfileName: "nginx-rec-tools"}, mergedRecording(), mergedProfile(), testTarget()},
		{"malformed recording identity", Source{Mode: ModeMergedProvenance, RecordingNamespace: "observations", RecordingName: "BAD_NAME", ProfileName: "nginx-rec-tools"}, mergedRecording(), mergedProfile(), testTarget()},
		{"unsupported derivation", Source{Mode: "automatic", RecordingNamespace: "observations", RecordingName: "nginx-rec", ProfileName: "nginx-rec-tools"}, mergedRecording(), mergedProfile(), testTarget()},
		{"partial final profile", mergedSource(), mergedRecording(), setLabel(mergedProfile(), spobackend.PartialLabel, "true"), testTarget()},
		{"unique contributor claim", mergedSource(), mergedRecording(), setLabel(mergedProfile(), spobackend.ContainerIDLabel, "tools"), testTarget()},
		{"missing target", mergedSource(), mergedRecording(), mergedProfile(), Target{Namespace: "prod", Container: "tools"}},
		{"invalid target", mergedSource(), mergedRecording(), mergedProfile(), Target{Namespace: "prod", Pod: "BAD_NAME", Container: "tools"}},
		{"wrong recording strategy", mergedSource(), validRecording(), mergedProfile(), testTarget()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Snapshot(tc.rec, tc.prof, tc.src, tc.tgt); err == nil {
				t.Fatal("Snapshot() accepted ambiguous merged provenance")
			}
		})
	}
}

func TestSnapshot_StrongFailureNeverFallsBackToMerged(t *testing.T) {
	profile := setLabel(validProfile(), spobackend.ContainerIDLabel, nil)
	if _, err := Snapshot(validRecording(), profile, testSource(), testTarget()); err == nil {
		t.Fatal("strong-lineage import silently accepted merged provenance")
	}
}

// Semantic equivalence is the contract, so every supported field must
// survive with its meaning AND its order intact: seccomp rules are
// evaluated in order, so reordering them is a semantic change even when the
// set is identical.
func TestSnapshot_PreservesSupportedSemanticsExactly(t *testing.T) {
	got := mustSnapshot(t)

	if len(got.Profile.Syscalls) != 2 {
		t.Fatalf("got %d syscall rules, want 2", len(got.Profile.Syscalls))
	}
	if want := []string{"openat", "read", "write"}; !reflect.DeepEqual(got.Profile.Syscalls[0].Names, want) {
		t.Errorf("syscalls[0].Names = %v, want %v (order is semantic)", got.Profile.Syscalls[0].Names, want)
	}
	if got.Profile.Syscalls[0].Action != "SCMP_ACT_ALLOW" {
		t.Errorf("syscalls[0].Action = %q, want SCMP_ACT_ALLOW", got.Profile.Syscalls[0].Action)
	}
	if got.Profile.Syscalls[1].Action != "SCMP_ACT_LOG" {
		t.Errorf("syscalls[1].Action = %q, want SCMP_ACT_LOG — a second distinct action must not collapse into the first",
			got.Profile.Syscalls[1].Action)
	}
}

// Determinism matters because the artifact is digested: the same source
// must produce the same candidate, or a re-import would invalidate an
// approval that nothing actually changed.
func TestSnapshot_Deterministic(t *testing.T) {
	a := mustSnapshot(t)
	b := mustSnapshot(t)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Snapshot() is not deterministic:\n%+v\n%+v", a, b)
	}
}

// --- Snapshot / immutability ----------------------------------------------

// The source is read, never written. Mutating an SPO-owned cluster-scoped
// object would have cluster-wide blast radius and is what Model B exists to
// avoid.
func TestSnapshot_DoesNotMutateSource(t *testing.T) {
	recording, profile := validRecording(), validProfile()
	before := profile.DeepCopy()
	beforeRec := recording.DeepCopy()

	if _, err := Snapshot(recording, profile, testSource(), testTarget()); err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if !reflect.DeepEqual(profile, before) {
		t.Error("Snapshot() mutated the source SeccompProfile")
	}
	if !reflect.DeepEqual(recording, beforeRec) {
		t.Error("Snapshot() mutated the source ProfileRecording")
	}
}

// The candidate holds copied content, not a reference. This is what makes
// an approval survive the source changing underneath it — and what stops a
// post-approval edit of the SPO object from changing what gets enforced.
func TestSnapshot_SourceMutationCannotAlterResult(t *testing.T) {
	recording, profile := validRecording(), validProfile()
	got, err := Snapshot(recording, profile, testSource(), testTarget())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	before := append([]string(nil), got.Profile.Syscalls[0].Names...)
	beforeAction := got.Profile.DefaultAction

	// Widen the source drastically after the snapshot was taken.
	setSpec(profile, "defaultAction", "SCMP_ACT_ALLOW")
	setSpec(profile, "syscalls", []interface{}{
		map[string]interface{}{"names": []interface{}{"execve", "ptrace", "mount"}, "action": "SCMP_ACT_ALLOW"},
	})

	if got.Profile.DefaultAction != beforeAction {
		t.Errorf("DefaultAction changed to %q after source mutation; the snapshot holds a reference, not a copy", got.Profile.DefaultAction)
	}
	if !reflect.DeepEqual(got.Profile.Syscalls[0].Names, before) {
		t.Errorf("syscall names changed to %v after source mutation, want %v", got.Profile.Syscalls[0].Names, before)
	}
}

// Deletion is the same property as mutation: an approved candidate must
// remain appliable when the recording output is cleaned up.
func TestSnapshot_SourceDeletionCannotAlterResult(t *testing.T) {
	recording, profile := validRecording(), validProfile()
	got, err := Snapshot(recording, profile, testSource(), testTarget())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	want := append([]string(nil), got.Profile.Syscalls[0].Names...)

	// Simulate deletion as hard as an in-memory test can: empty the object.
	profile.Object = map[string]interface{}{}
	recording.Object = map[string]interface{}{}

	if !reflect.DeepEqual(got.Profile.Syscalls[0].Names, want) {
		t.Errorf("syscall names = %v after source deletion, want %v", got.Profile.Syscalls[0].Names, want)
	}
	if got.Provenance[spobackend.SourceProfileAnnotation] != "nginx-rec-tools" {
		t.Error("provenance lost its source profile name after the source was deleted")
	}
}

// --- Lineage ---------------------------------------------------------------

func TestSnapshot_LineageMismatchIsFatal(t *testing.T) {
	for _, tc := range []struct {
		name, label string
		value       interface{}
		wantIn      string
	}{
		{"wrong recording namespace", spobackend.RecordingNamespaceLabel, "staging", "recording-namespace"},
		{"wrong recording id", spobackend.RecordingIDLabel, "other-rec", "recording-id"},
		{"wrong container id", spobackend.ContainerIDLabel, "sidecar", "container-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := setLabel(validProfile(), tc.label, tc.value)
			_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
			if err == nil {
				t.Fatal("Snapshot() = nil error, want a lineage refusal — this is cross-workload authority substitution")
			}
			if !errors.Is(err, ErrImport) {
				t.Errorf("error %v does not wrap ErrImport", err)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not name the failing label %q", err, tc.wantIn)
			}
		})
	}
}

// A single missing label is refused individually, because a merged SPO
// profile carries recording-id and recording-namespace but NOT
// container-id — so this is the realistic case, not a synthetic one.
func TestSnapshot_MissingSingleLineageLabelIsFatal(t *testing.T) {
	for _, label := range []string{
		spobackend.RecordingNamespaceLabel,
		spobackend.RecordingIDLabel,
		spobackend.ContainerIDLabel,
	} {
		t.Run(label, func(t *testing.T) {
			profile := setLabel(validProfile(), label, nil)
			if _, err := Snapshot(validRecording(), profile, testSource(), testTarget()); err == nil {
				t.Fatalf("Snapshot() accepted a profile with no %s label", label)
			}
		})
	}
}

func TestSnapshot_NoLineageLabelsAtAllIsFatal(t *testing.T) {
	profile := validProfile()
	profile.Object["metadata"].(map[string]interface{})["labels"] = map[string]interface{}{}

	_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
	if err == nil {
		t.Fatal("Snapshot() accepted a profile with no recording labels; lineage cannot be established")
	}
	if !strings.Contains(err.Error(), "none of SPO's recording labels") {
		t.Errorf("error %q should say lineage could not be established at all", err)
	}
}

// Lineage is read from labels. A profile whose NAME looks exactly right but
// whose labels disagree must still be refused — name parsing is prohibited
// precisely because a name is not authority.
func TestSnapshot_NameIsNotLineage(t *testing.T) {
	profile := setLabel(validProfile(), spobackend.ContainerIDLabel, "someone-else")
	profile.SetName("nginx-rec-tools") // still the "expected" name

	if _, err := Snapshot(validRecording(), profile, testSource(), testTarget()); err == nil {
		t.Fatal("Snapshot() trusted the object name over its labels")
	}
}

// --- Completion ------------------------------------------------------------

// SPO's own IsPartial tests for the key's PRESENCE and ignores the value,
// so partial="false" is still partial. Matching that exactly is the whole
// point: a value-based check would import a fragment as if it were whole.
func TestSnapshot_PartialIsFatalRegardlessOfValue(t *testing.T) {
	for _, value := range []string{"true", "false", ""} {
		t.Run("partial="+value, func(t *testing.T) {
			profile := setLabel(validProfile(), spobackend.PartialLabel, value)
			_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
			if err == nil {
				t.Fatalf("Snapshot() accepted a profile labelled partial=%q; SPO treats any presence as partial", value)
			}
			if !strings.Contains(err.Error(), spobackend.PartialLabel) {
				t.Errorf("error %q does not name the partial label", err)
			}
		})
	}
}

// An outstanding merge means the authority is still being assembled.
func TestSnapshot_UnmergedRecordingIsFatal(t *testing.T) {
	recording := validRecording()
	recording.SetFinalizers([]string{spobackend.UnmergedProfilesFinalizer})

	_, err := Snapshot(recording, validProfile(), testSource(), testTarget())
	if err == nil {
		t.Fatal("Snapshot() accepted a recording with partial profiles still outstanding")
	}
	if !strings.Contains(err.Error(), "merge has not completed") {
		t.Errorf("error %q should explain that the merge is incomplete", err)
	}
}

// --- Inertness -------------------------------------------------------------

// An enforcing source profile means SPO is already applying unreviewed
// authority to nodes, which is exactly the state approval exists to precede.
func TestSnapshot_NonInertSourceIsFatal(t *testing.T) {
	for _, state := range []interface{}{spobackend.SpecStateEnabled, "", nil} {
		name := "state=" + strings.TrimSpace(strings.Trim(strings.Join([]string{toString(state)}, ""), " "))
		t.Run(name, func(t *testing.T) {
			profile := setSpec(validProfile(), "state", state)
			if _, err := Snapshot(validRecording(), profile, testSource(), testTarget()); err == nil {
				t.Fatalf("Snapshot() accepted a source profile with state %v", state)
			}
		})
	}
}

// Inertness must be durable, not incidental: without
// disableProfileAfterRecording nothing stops SPO reconciling the profile a
// moment later.
func TestSnapshot_RecordingWithoutDisableAfterRecordingIsFatal(t *testing.T) {
	for _, value := range []interface{}{false, nil} {
		recording := setSpec(validRecording(), "disableProfileAfterRecording", value)
		_, err := Snapshot(recording, validProfile(), testSource(), testTarget())
		if err == nil {
			t.Fatalf("Snapshot() accepted a recording with disableProfileAfterRecording=%v", value)
		}
		if !strings.Contains(err.Error(), "disableProfileAfterRecording") {
			t.Errorf("error %q does not name the missing setting", err)
		}
	}
}

func toString(v interface{}) string {
	if v == nil {
		return "absent"
	}
	if s, ok := v.(string); ok && s == "" {
		return "empty"
	}
	return v.(string)
}

// --- Closed enforcement allow-list ----------------------------------------

// A closed allow-list, not a deny-list: the "futureField" case is the point
// — a field SPO adds after this was written must be refused, not dropped.
func TestSnapshot_UnsupportedSpecFieldIsFatal(t *testing.T) {
	for _, field := range []string{
		"baseProfileName", "listenerPath", "listenerMetadata", "flags", "futureField",
	} {
		t.Run(field, func(t *testing.T) {
			profile := setSpec(validProfile(), field, "something")
			_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
			if err == nil {
				t.Fatalf("Snapshot() accepted a source carrying %s; dropping it would change what is enforced", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("error %q does not name the offending field %q — a refusal must be actionable", err, field)
			}
		})
	}
}

func TestSnapshot_UnsupportedSyscallFieldIsFatal(t *testing.T) {
	for _, field := range []string{"args", "errnoRet", "futureRuleField"} {
		t.Run(field, func(t *testing.T) {
			profile := validProfile()
			rules := profile.Object["spec"].(map[string]interface{})["syscalls"].([]interface{})
			rules[0].(map[string]interface{})[field] = "something"

			_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
			if err == nil {
				t.Fatalf("Snapshot() accepted syscalls[0].%s", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("error %q does not name %q", err, field)
			}
		})
	}
}

// state is the one field permitted in the source and deliberately not
// copied: it is lifecycle, not enforcement. Carrying Disabled into the
// governed copy would produce an approved artifact that enforces nothing.
func TestSnapshot_StateIsAcceptedButNotCopied(t *testing.T) {
	got := mustSnapshot(t)
	// pkg/seccomp.Profile has no state field at all, so the check is that
	// the import succeeded with state present — proven by mustSnapshot —
	// and that nothing state-shaped leaked into provenance.
	for key, value := range got.Provenance {
		if strings.Contains(strings.ToLower(value), "disabled") {
			t.Errorf("provenance %s carries lifecycle state %q", key, value)
		}
	}
}

func TestSnapshot_MissingDefaultActionIsFatal(t *testing.T) {
	profile := setSpec(validProfile(), "defaultAction", nil)
	_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
	if err == nil {
		t.Fatal("Snapshot() accepted a profile with no defaultAction")
	}
	if !strings.Contains(err.Error(), "defaultAction") {
		t.Errorf("error %q does not name defaultAction", err)
	}
}

// --- API shape -------------------------------------------------------------

func TestSnapshot_RejectsLegacyAPI(t *testing.T) {
	profile := validProfile()
	profile.SetAPIVersion(spobackend.Group + "/v1beta1")

	_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
	if err == nil {
		t.Fatal("Snapshot() accepted a v1beta1 source; one backend contract is what keeps the digest independent of which SPO is installed")
	}
}

// A namespaced source belongs to SPO v0.8.4, whose localhostProfile path
// had a namespace segment — accepting it would produce an artifact this
// project's readiness gate could never verify.
func TestSnapshot_RejectsNamespacedSource(t *testing.T) {
	profile := validProfile()
	profile.SetNamespace("prod")

	_, err := Snapshot(validRecording(), profile, testSource(), testTarget())
	if err == nil {
		t.Fatal("Snapshot() accepted a namespaced SeccompProfile")
	}
	if !strings.Contains(err.Error(), "cluster-scoped") {
		t.Errorf("error %q should explain the scope violation", err)
	}
}

func TestSnapshot_RejectsWrongKind(t *testing.T) {
	profile := validProfile()
	profile.SetKind("SelinuxProfile")

	if _, err := Snapshot(validRecording(), profile, testSource(), testTarget()); err == nil {
		t.Fatal("Snapshot() accepted a non-SeccompProfile source")
	}
}

// --- Provenance and coverage ----------------------------------------------

func TestSnapshot_ProvenanceRecordsWhatWasVerified(t *testing.T) {
	got := mustSnapshot(t)

	for key, want := range map[string]string{
		spobackend.SeccompSourceAnnotation:            spobackend.SeccompSourceSPO,
		spobackend.SeccompOriginAnnotation:            spobackend.SeccompOriginDerived,
		spobackend.SourceProfileAnnotation:            "nginx-rec-tools",
		spobackend.SourceRecordingNamespaceAnnotation: "prod",
		spobackend.SourceRecordingAnnotation:          "nginx-rec",
		spobackend.SourceContainerAnnotation:          "tools",
	} {
		if got.Provenance[key] != want {
			t.Errorf("provenance[%s] = %q, want %q", key, got.Provenance[key], want)
		}
	}
}

// SPO v1.0.0 does not emit a coverage annotation, so this is what real
// imports record. Absent must be "unknown" — never 0, never "full", never a
// tier, because each of those is a claim nobody made.
func TestSnapshot_AbsentCoverageIsUnknown(t *testing.T) {
	got := mustSnapshot(t)
	if got.Provenance[spobackend.SourceCoverageAnnotation] != spobackend.CoverageUnknown {
		t.Errorf("coverage = %q, want %q", got.Provenance[spobackend.SourceCoverageAnnotation], spobackend.CoverageUnknown)
	}
}

// If SPO ever does report coverage, it is copied verbatim and never
// transformed: coverage is replicas-per-recording, not frequency over runs
// and not authorization strength.
func TestSnapshot_PresentCoverageIsCopiedVerbatim(t *testing.T) {
	profile := validProfile()
	profile.SetAnnotations(map[string]string{spobackend.SyscallCoverageAnnotation: "3/5"})

	got, err := Snapshot(validRecording(), profile, testSource(), testTarget())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got.Provenance[spobackend.SourceCoverageAnnotation] != "3/5" {
		t.Errorf("coverage = %q, want the verbatim %q", got.Provenance[spobackend.SourceCoverageAnnotation], "3/5")
	}
}

// Coverage must never become a confidence tier. This asserts on the
// vocabulary rather than a specific transform, so a future accidental
// mapping is caught however it is spelled.
func TestSnapshot_CoverageNeverBecomesConfidence(t *testing.T) {
	profile := validProfile()
	profile.SetAnnotations(map[string]string{spobackend.SyscallCoverageAnnotation: "5/5"})

	got, err := Snapshot(validRecording(), profile, testSource(), testTarget())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	for key, value := range got.Provenance {
		lower := strings.ToLower(key + "=" + value)
		for _, forbidden := range []string{"confidence", "seeninruns", "runsrecorded", "frequency", "probability"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("provenance %s=%q leaks the forbidden concept %q", key, value, forbidden)
			}
		}
	}
}

// --- Explicit source selection --------------------------------------------

// Ambiguity is refused, never resolved by searching. A cluster scan for a
// profile that "looks right" is how one workload's authority ends up on
// another.
func TestSnapshot_AmbiguousSelectionIsFatal(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  Source
		tgt  Target
	}{
		{"no recording named", Source{ProfileName: "p"}, testTarget()},
		{"no profile named", Source{RecordingName: "r"}, testTarget()},
		{"nothing named", Source{}, testTarget()},
		{"no target namespace", testSource(), Target{Pod: "p", Container: "c"}},
		{"no target container", testSource(), Target{Namespace: "prod", Pod: "p"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Snapshot(validRecording(), validProfile(), tc.src, tc.tgt)
			if err == nil {
				t.Fatal("Snapshot() accepted an under-specified source")
			}
			if !strings.Contains(err.Error(), "ambiguous source selection") {
				t.Errorf("error %q should refuse the ambiguity explicitly", err)
			}
		})
	}
}

// --- Import against a cluster ---------------------------------------------

var (
	seccompProfileListGVR = spobackend.SeccompProfileGVR()
	profileRecordingGVR   = spobackend.ProfileRecordingGVR()
)

func newFakeDynamic(objs ...runtime.Object) dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			seccompProfileListGVR: "SeccompProfileList",
			profileRecordingGVR:   "ProfileRecordingList",
		},
		objs...,
	)
}

func TestImport_HappyPath(t *testing.T) {
	client := newFakeDynamic(validRecording(), validProfile())

	got, err := Import(context.Background(), client, testSource(), testTarget())
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if got.Profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("DefaultAction = %q", got.Profile.DefaultAction)
	}
}

func TestImport_MergedProfileAfterRecordingDeletion(t *testing.T) {
	client := newFakeDynamic(mergedProfile())

	got, err := Import(context.Background(), client, mergedSource(), testTarget())
	if err != nil {
		t.Fatalf("Import() merged source after recording deletion error = %v", err)
	}
	if got.Provenance[spobackend.ContributorLineageAnnotation] != spobackend.ContributorLineageUnavailable {
		t.Fatalf("merged provenance = %#v", got.Provenance)
	}
}

// A missing recording is not "import anyway with a warning". The whole
// point of naming it is that its absence is a refusal.
func TestImport_MissingRecordingIsFatal(t *testing.T) {
	client := newFakeDynamic(validProfile())

	_, err := Import(context.Background(), client, testSource(), testTarget())
	if err == nil {
		t.Fatal("Import() succeeded with no ProfileRecording present")
	}
	if !strings.Contains(err.Error(), "ProfileRecording") {
		t.Errorf("error %q does not name the missing recording", err)
	}
}

func TestImport_MissingSourceProfileIsFatal(t *testing.T) {
	client := newFakeDynamic(validRecording())

	_, err := Import(context.Background(), client, testSource(), testTarget())
	if err == nil {
		t.Fatal("Import() succeeded with no source SeccompProfile present")
	}
	if !strings.Contains(err.Error(), "SeccompProfile") {
		t.Errorf("error %q does not name the missing profile", err)
	}
}

// SPO absent entirely is the "SPO unavailable in SPO mode" case: the CRDs
// do not exist, so the Get fails. It must be a refusal, never a quiet
// fallback to internal synthesis.
func TestImport_SPOUnavailableIsFatal(t *testing.T) {
	client := newFakeDynamic()

	_, err := Import(context.Background(), client, testSource(), testTarget())
	if err == nil {
		t.Fatal("Import() succeeded against a cluster with no SPO objects at all")
	}
	if !errors.Is(err, ErrImport) {
		t.Errorf("error %v does not wrap ErrImport", err)
	}
}

// The recording is fetched from the TARGET namespace. A recording of the
// same name in another namespace must not satisfy the import.
func TestImport_RecordingIsNamespaceScopedToTarget(t *testing.T) {
	recording := validRecording()
	recording.SetNamespace("staging")

	client := newFakeDynamic(recording, validProfile())

	if _, err := Import(context.Background(), client, testSource(), testTarget()); err == nil {
		t.Fatal("Import() accepted a ProfileRecording from a different namespace")
	}
}
