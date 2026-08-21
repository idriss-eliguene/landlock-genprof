// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"context"
	"errors"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	exportspo "github.com/idriss-eliguene/landlock-genprof/internal/exporter/spo"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/spoimport"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
)

// --- Source selection is explicit -----------------------------------------

// Every case here is a malformed invocation, so every case must be exit 3
// (usage) rather than exit 2 (a governance refusal). Collapsing them would
// make a typo indistinguishable from a workload failing its gates.
func TestValidateSeccompSourceFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    traceOptions
		wantErr bool
		wantIn  string
	}{
		{
			name: "internal is the default and needs nothing",
			opts: traceOptions{seccompSource: spobackend.SeccompSourceInternal},
		},
		{
			name: "spo with both names is complete",
			opts: traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "r", spoProfile: "p"},
		},
		{
			name: "explicit merged provenance is complete",
			opts: traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "r", spoProfile: "p", spoImportMode: string(spoimport.ModeMergedProvenance), spoRecordingNamespace: "observations"},
		},
		{
			name:    "merged provenance requires source namespace",
			opts:    traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "r", spoProfile: "p", spoImportMode: string(spoimport.ModeMergedProvenance)},
			wantErr: true, wantIn: "--spo-recording-namespace",
		},
		{
			name:    "strong lineage rejects merged-only namespace",
			opts:    traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "r", spoProfile: "p", spoImportMode: string(spoimport.ModeStrongLineage), spoRecordingNamespace: "observations"},
			wantErr: true, wantIn: "only valid",
		},
		{
			name:    "spo without a recording is ambiguous",
			opts:    traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoProfile: "p"},
			wantErr: true, wantIn: "--spo-recording",
		},
		{
			name:    "spo without a profile is ambiguous",
			opts:    traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "r"},
			wantErr: true, wantIn: "--spo-profile",
		},
		{
			name:    "spo material named while internal is selected",
			opts:    traceOptions{seccompSource: spobackend.SeccompSourceInternal, spoRecording: "r"},
			wantErr: true, wantIn: "--seccomp-source",
		},
		{
			name:    "local seccomp output is unavailable in spo mode",
			opts:    traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "r", spoProfile: "p", seccompOut: "-"},
			wantErr: true, wantIn: "--seccomp-out",
		},
		{
			name:    "an unknown source is refused, never defaulted",
			opts:    traceOptions{seccompSource: "auto"},
			wantErr: true, wantIn: "must be",
		},
		{
			name:    "an empty source is refused rather than inferred",
			opts:    traceOptions{seccompSource: ""},
			wantErr: true, wantIn: "must be",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSeccompSourceFlags(tc.opts)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateSeccompSourceFlags() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				return
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}
			coder, ok := err.(interface{ ExitCode() int })
			if !ok || coder.ExitCode() != 3 {
				t.Errorf("error does not carry usage exit code 3; a malformed invocation must not look like a governance refusal")
			}
		})
	}
}

// The flag default is what makes standalone the unchanged path: a user who
// has never heard of SPO gets internal synthesis without asking.
func TestTraceFlagDefault_IsInternal(t *testing.T) {
	cmd := newTraceCmd()
	flag := cmd.Flags().Lookup("seccomp-source")
	if flag == nil {
		t.Fatal("trace has no --seccomp-source flag")
	}
	if flag.DefValue != spobackend.SeccompSourceInternal {
		t.Errorf("--seccomp-source default = %q, want %q", flag.DefValue, spobackend.SeccompSourceInternal)
	}
}

// --- Structural TrainingHistory boundary ----------------------------------

// The prohibition is that no value from an SPO profile may become a
// frequency or a confidence. ADR-0008 makes that structural: the import
// path has no route into the evidence or history models.
//
// This asserts it on the dependency graph rather than on behavior, because
// behavior only proves today's code does not do it, while the graph proves
// no ordinary refactor inside spoimport CAN do it.
func TestSPOImport_HasNoRouteIntoEvidenceOrHistory(t *testing.T) {
	const module = "github.com/idriss-eliguene/landlock-genprof"
	forbidden := map[string]string{
		module + "/internal/history":  "TrainingHistory accumulation",
		module + "/internal/evidence": "the evidence model",
		module + "/internal/analysis": "confidence scoring",
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	root = filepath.Dir(root)

	seen := map[string]bool{}
	var walk func(pkgPath string, chain []string)
	walk = func(pkgPath string, chain []string) {
		if seen[pkgPath] {
			return
		}
		seen[pkgPath] = true

		if why, bad := forbidden[pkgPath]; bad {
			t.Errorf("internal/spoimport reaches %s (%s) via %s — imported policy must have no route to a frequency or a confidence tier",
				pkgPath, why, strings.Join(append(chain, pkgPath), " -> "))
			return
		}
		if !strings.HasPrefix(pkgPath, module) {
			return // third-party and stdlib cannot reach our models
		}

		dir := filepath.Join(root, strings.TrimPrefix(pkgPath, module+"/"))
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			return
		}
		for _, imp := range pkg.Imports {
			walk(imp, append(chain, pkgPath))
		}
	}
	walk(module+"/internal/spoimport", nil)
}

// --- Syscall suppression is structural ------------------------------------

func syscallEvent(name string) tracer.Event {
	return tracer.Event{Syscall: name, Mode: "syscall"}
}

func fileEvent(path string) tracer.Event {
	return tracer.Event{Syscall: "openat", Path: path, Mode: "read"}
}

func networkEvent(port int) tracer.Event {
	return tracer.Event{Syscall: "connect", Port: port, Mode: "egress"}
}

// In SPO mode our syscall observations are not collected at all, rather
// than collected and filtered later — so nothing downstream can accumulate
// them into TrainingHistory even by mistake. Filesystem and network stay
// ours in both modes and must survive untouched.
func TestDropSyscallObservations(t *testing.T) {
	events := []tracer.Event{
		fileEvent("/etc/nginx/nginx.conf"),
		syscallEvent("openat"),
		networkEvent(443),
		syscallEvent("bind"), // a network-NAMED syscall: still a syscall observation
		fileEvent("/var/log/nginx"),
	}

	got := dropSyscallObservations(events)

	if len(got) != 3 {
		t.Fatalf("kept %d events, want 3 (2 filesystem + 1 network)", len(got))
	}
	for _, ev := range got {
		if ev.Mode == "syscall" {
			t.Errorf("syscall observation %q survived suppression", ev.Syscall)
		}
	}
	if got[0].Path != "/etc/nginx/nginx.conf" || got[2].Path != "/var/log/nginx" {
		t.Errorf("filesystem observations were not preserved in order: %+v", got)
	}
	if got[1].Port != 443 {
		t.Errorf("network observation was not preserved: %+v", got[1])
	}
}

func TestDropSyscallObservations_LeavesStandaloneUntouched(t *testing.T) {
	events := []tracer.Event{fileEvent("/etc"), networkEvent(80)}
	if got := dropSyscallObservations(events); len(got) != len(events) {
		t.Errorf("dropped %d non-syscall events", len(events)-len(got))
	}
}

// --- resolveSeccompSource --------------------------------------------------

func behaviorWithSyscalls(names ...string) profile.BehaviorProfile {
	accesses := make([]profile.SyscallAccess, 0, len(names))
	for _, n := range names {
		accesses = append(accesses, profile.SyscallAccess{
			Name:       n,
			Confidence: profile.ConfidenceHigh,
			SeenCount:  3,
		})
	}
	return profile.BehaviorProfile{Syscalls: profile.SyscallProfile{Accesses: accesses}}
}

func spoTarget() spoimport.Target {
	return spoimport.Target{Namespace: "prod", Pod: "nginx-demo", Container: "tools"}
}

func validSPORecording() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": spobackend.APIVersion,
		"kind":       spobackend.ProfileRecordingKind,
		"metadata":   map[string]interface{}{"name": "nginx-rec", "namespace": "prod"},
		"spec":       map[string]interface{}{"disableProfileAfterRecording": true},
	}}
}

func validSPOProfile() *unstructured.Unstructured {
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
			"architectures": []interface{}{"SCMP_ARCH_X86_64"},
			"syscalls": []interface{}{map[string]interface{}{
				"names":  []interface{}{"openat", "read"},
				"action": "SCMP_ACT_ALLOW",
			}},
		},
	}}
}

func fakeSPOCluster(objs ...runtime.Object) dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			spobackend.SeccompProfileGVR():   "SeccompProfileList",
			spobackend.ProfileRecordingGVR(): "ProfileRecordingList",
		},
		objs...,
	)
}

func TestResolveSeccompSource_Internal(t *testing.T) {
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out, fakeSPOCluster(),
		traceOptions{seccompSource: spobackend.SeccompSourceInternal},
		spoTarget(), behaviorWithSyscalls("openat", "read"))
	if err != nil {
		t.Fatalf("resolveSeccompSource() error = %v", err)
	}
	if src.isSPO() {
		t.Error("internal mode resolved to the SPO source")
	}
	if !src.hasProfile() {
		t.Fatal("internal mode with observed syscalls produced no profile")
	}
	if src.provenance[spobackend.SeccompSourceAnnotation] != spobackend.SeccompSourceInternal {
		t.Errorf("internal provenance = %v", src.provenance)
	}
	if src.provenance[spobackend.SeccompOriginAnnotation] != spobackend.SeccompOriginObserved {
		t.Error("internal synthesis must be marked observed, not derived")
	}
}

// No syscalls observed is not an error in internal mode — it just means
// this run has no seccomp authority to govern.
func TestResolveSeccompSource_InternalWithNoSyscalls(t *testing.T) {
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out, fakeSPOCluster(),
		traceOptions{seccompSource: spobackend.SeccompSourceInternal},
		spoTarget(), profile.BehaviorProfile{})
	if err != nil {
		t.Fatalf("resolveSeccompSource() error = %v", err)
	}
	if src.hasProfile() {
		t.Error("internal mode invented a profile from no observations")
	}
}

func TestResolveSeccompSource_SPO(t *testing.T) {
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out,
		fakeSPOCluster(validSPORecording(), validSPOProfile()),
		traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "nginx-rec", spoProfile: "nginx-rec-tools"},
		spoTarget(), profile.BehaviorProfile{})
	if err != nil {
		t.Fatalf("resolveSeccompSource() error = %v", err)
	}
	if !src.isSPO() {
		t.Fatal("SPO mode did not resolve to the SPO source")
	}
	if src.profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("imported DefaultAction = %q", src.profile.DefaultAction)
	}
	if !strings.Contains(out.String(), "SPO derived policy") {
		t.Errorf("SPO mode did not announce the source:\n%s", out.String())
	}
}

// The single most important negative: SPO was selected, SPO material is
// unavailable, and this project HAS observed syscalls it could fall back
// to. It must refuse rather than silently govern different authority.
func TestResolveSeccompSource_SPOUnavailableDoesNotFallBack(t *testing.T) {
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out, fakeSPOCluster(),
		traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "nginx-rec", spoProfile: "nginx-rec-tools"},
		spoTarget(), behaviorWithSyscalls("openat", "read", "execve"))
	if err == nil {
		t.Fatal("SPO mode fell back to internal synthesis when SPO was unavailable")
	}
	if src.hasProfile() {
		t.Error("a failed import still produced a profile")
	}
}

// --- Provenance is visible to the reviewer --------------------------------

func renderArtifact(t *testing.T, src seccompSource) string {
	t.Helper()
	cr := exportspo.ToSeccompProfile(
		exportspo.Meta{Namespace: "prod", Pod: "nginx-demo", Container: "tools"},
		src.profile, src.provenance)
	out, err := exportspo.ToYAML(cr)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	return string(out)
}

func spoSourceForTest(t *testing.T) seccompSource {
	t.Helper()
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out,
		fakeSPOCluster(validSPORecording(), validSPOProfile()),
		traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "nginx-rec", spoProfile: "nginx-rec-tools"},
		spoTarget(), profile.BehaviorProfile{})
	if err != nil {
		t.Fatalf("resolveSeccompSource() error = %v", err)
	}
	return src
}

func internalSourceForTest() seccompSource {
	return seccompSource{
		kind:       spobackend.SeccompSourceInternal,
		profile:    &seccomp.Profile{DefaultAction: "SCMP_ACT_ERRNO", Syscalls: []seccomp.SyscallRule{{Names: []string{"openat", "read"}, Action: "SCMP_ACT_ALLOW"}}},
		provenance: spobackend.InternalSeccompProvenance(),
	}
}

// Confidence must be stated as not applicable rather than omitted: a blank
// where filesystem rules show "high" reads as "low", which would be an
// invented tier for data that has no occurrence information at all.
func TestPrintSeccompProvenance_SPOShowsDerivedAndNoConfidence(t *testing.T) {
	artifact := renderArtifact(t, spoSourceForTest(t))

	var out bytes.Buffer
	printSeccompProvenance(&out, artifact)
	got := out.String()

	for _, want := range []string{
		"security-profiles-operator",
		"derived policy",
		"nginx-rec-tools",
		"prod/nginx-rec",
		"tools",
		"Coverage: unknown",
		"Confidence: not applicable",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), "observed by landlock-genprof\n") {
		t.Error("derived policy was presented as this project's own observation")
	}
}

func TestPrintSeccompProvenance_InternalShowsObserved(t *testing.T) {
	artifact := renderArtifact(t, internalSourceForTest())

	var out bytes.Buffer
	printSeccompProvenance(&out, artifact)
	got := out.String()

	if !strings.Contains(got, "landlock-genprof observation") {
		t.Errorf("internal mode not shown as our own observation:\n%s", got)
	}
	if strings.Contains(got, "derived policy") {
		t.Errorf("internal synthesis was labelled derived policy:\n%s", got)
	}
}

// A pre-ADR-0008 artifact has no provenance. Reporting it as unattributed
// is honest; assuming internal would put a source label on something nobody
// recorded a source for.
func TestPrintSeccompProvenance_LegacyArtifactIsUnattributed(t *testing.T) {
	var out bytes.Buffer
	printSeccompProvenance(&out, "apiVersion: security-profiles-operator.x-k8s.io/v1\nkind: SeccompProfile\nmetadata:\n  name: old\n")
	if !strings.Contains(out.String(), "unattributed") {
		t.Errorf("legacy artifact not reported as unattributed:\n%s", out.String())
	}
}

// --- Digest and stale approval --------------------------------------------

func specWith(artifact string) proposal.Spec {
	return proposal.Spec{
		Container:         "tools",
		Binary:            "/usr/sbin/nginx",
		PodLock:           "podlock: yaml",
		SPOSeccompProfile: artifact,
	}
}

func digestOf(t *testing.T, spec proposal.Spec) string {
	t.Helper()
	d, err := proposal.CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	return d
}

// Switching the seccomp source must invalidate an approval even when the
// syscalls are byte-identical: what changed is which system's authority is
// being enforced, and that is exactly what a reviewer signed off on.
func TestCandidateDigest_ChangesWhenSeccompSourceChanges(t *testing.T) {
	sameSyscalls := &seccomp.Profile{
		DefaultAction: "SCMP_ACT_ERRNO",
		Architectures: []string{"SCMP_ARCH_X86_64"},
		Syscalls:      []seccomp.SyscallRule{{Names: []string{"openat", "read"}, Action: "SCMP_ACT_ALLOW"}},
	}

	spoSrc := spoSourceForTest(t)
	spoSrc.profile = sameSyscalls
	internal := internalSourceForTest()
	internal.profile = sameSyscalls

	spoDigest := digestOf(t, specWith(renderArtifact(t, spoSrc)))
	internalDigest := digestOf(t, specWith(renderArtifact(t, internal)))

	if spoDigest == internalDigest {
		t.Error("identical syscalls from different sources produced the same digest; an approval bound to SPO-derived authority would silently authorize internally-synthesised authority")
	}
}

// Re-importing from a DIFFERENT recording changes provenance, so the digest
// changes and the previous approval goes stale — ADR-0008 accepts this
// friction deliberately, because a new recording is genuinely new evidence.
func TestCandidateDigest_ChangesWhenRecordingChanges(t *testing.T) {
	a := spoSourceForTest(t)

	b := spoSourceForTest(t)
	b.provenance[spobackend.SourceRecordingAnnotation] = "nginx-rec-2"

	if digestOf(t, specWith(renderArtifact(t, a))) == digestOf(t, specWith(renderArtifact(t, b))) {
		t.Error("a different source recording produced the same digest; the approval would not go stale")
	}
}

// The same source imported twice is the same candidate, so a retry does not
// gratuitously invalidate an approval.
func TestCandidateDigest_StableForRepeatedImportOfSameSource(t *testing.T) {
	a := digestOf(t, specWith(renderArtifact(t, spoSourceForTest(t))))
	b := digestOf(t, specWith(renderArtifact(t, spoSourceForTest(t))))
	if a != b {
		t.Errorf("repeated import of one source produced %s then %s", a, b)
	}
}

func TestCandidateDigest_BindsMergedProvenanceAndTarget(t *testing.T) {
	profile := &seccomp.Profile{DefaultAction: "SCMP_ACT_ERRNO", Syscalls: []seccomp.SyscallRule{{Names: []string{"read"}, Action: "SCMP_ACT_ALLOW"}}}
	baseProv := spobackend.SPOMergedSeccompProvenance("merged-profile", "observations", "recording-a")
	render := func(prov map[string]string, meta exportspo.Meta) string {
		src := seccompSource{kind: spobackend.SeccompSourceSPO, profile: profile, provenance: prov}
		cr := exportspo.ToSeccompProfile(meta, src.profile, src.provenance)
		out, err := exportspo.ToYAML(cr)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	meta := exportspo.Meta{Namespace: "prod", Pod: "nginx", Container: "app"}
	base := digestOf(t, specWith(render(baseProv, meta)))

	mutations := map[string]struct {
		prov map[string]string
		meta exportspo.Meta
	}{
		"recording identity": {spobackend.SPOMergedSeccompProvenance("merged-profile", "observations", "recording-b"), meta},
		"derivation": {func() map[string]string {
			p := spobackend.SPOMergedSeccompProvenance("merged-profile", "observations", "recording-a")
			p[spobackend.SourceDerivationAnnotation] = "other"
			return p
		}(), meta},
		"target": {baseProv, exportspo.Meta{Namespace: "prod", Pod: "other", Container: "app"}},
		"policy": {baseProv, meta},
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			artifact := render(mutation.prov, mutation.meta)
			if name == "policy" {
				artifact = strings.Replace(artifact, "read", "write", 1)
			}
			if got := digestOf(t, specWith(artifact)); got == base {
				t.Fatalf("%s mutation did not change CandidateDigest", name)
			}
		})
	}
	contextOnly := specWith(render(baseProv, meta))
	contextOnly.GeneratedAt = "2026-08-21T12:00:00Z"
	contextOnly.HistoryUsed = true
	if got := digestOf(t, contextOnly); got != base {
		t.Fatalf("context-only mutation changed CandidateDigest: %s -> %s", base, got)
	}
}

func TestPrintSeccompProvenance_MergedReviewContract(t *testing.T) {
	src := seccompSource{
		kind:       spobackend.SeccompSourceSPO,
		profile:    &seccomp.Profile{DefaultAction: "SCMP_ACT_ERRNO"},
		provenance: spobackend.SPOMergedSeccompProvenance("merged-profile", "observations", "nginx-rec"),
	}
	var out bytes.Buffer
	printSeccompProvenance(&out, renderArtifact(t, src))
	for _, want := range []string{
		"Source: security-profiles-operator",
		"Recording: observations/nginx-rec",
		"Derivation: merged",
		"Merge strategy: Containers",
		"Contributor lineage: unavailable",
		"Application target: prod/nginx-demo container tools",
		"Widening warning: this profile is a union of SPO partial profiles",
		"Confidence: not applicable",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("merged review missing %q:\n%s", want, out.String())
		}
	}
}

// --- Governed identity and workload binding -------------------------------

// The workload must reference what was approved, never the SPO source
// object. Binding to the source would point enforcement at an object this
// project does not own and did not digest.
func TestGovernedBinding_NeverReferencesSourceProfile(t *testing.T) {
	src := spoSourceForTest(t)
	meta := exportspo.Meta{Namespace: "prod", Pod: "nginx-demo", Container: "tools"}

	governedName := exportspo.ProfileName(meta)
	path := exportspo.LocalhostProfilePath(meta)

	if governedName == "nginx-rec-tools" {
		t.Fatal("the governed name collided with the SPO source name")
	}
	if want := spobackend.LocalhostProfilePath(governedName); path != want {
		t.Errorf("localhostProfile = %q, want %q", path, want)
	}
	if strings.Contains(path, "nginx-rec-tools") {
		t.Errorf("localhostProfile %q references the SPO source profile rather than the governed copy", path)
	}

	artifact := renderArtifact(t, src)
	if !strings.Contains(artifact, governedName) {
		t.Errorf("governed artifact is not named %q:\n%s", governedName, artifact)
	}
	// The source name appears only as recorded provenance, never as the
	// object's own identity.
	if strings.Contains(artifact, "name: nginx-rec-tools\n") {
		t.Errorf("governed artifact adopted the SPO source object's name:\n%s", artifact)
	}
}

// Identity is deterministic and frozen: the same workload always maps to
// the same governed object, whichever source produced its content.
func TestGovernedIdentity_IsSourceIndependent(t *testing.T) {
	meta := exportspo.Meta{Namespace: "prod", Pod: "nginx-demo", Container: "tools"}
	spoArtifact := renderArtifact(t, spoSourceForTest(t))
	internalArtifact := renderArtifact(t, internalSourceForTest())

	name := exportspo.ProfileName(meta)
	for _, artifact := range []string{spoArtifact, internalArtifact} {
		if !strings.Contains(artifact, name) {
			t.Errorf("artifact does not carry the governed name %q:\n%s", name, artifact)
		}
	}
}

// Ownership annotations survive alongside provenance — provenance must not
// displace the collision protection that makes a governed name safe to
// apply.
func TestGovernedArtifact_KeepsOwnershipAlongsideProvenance(t *testing.T) {
	artifact := renderArtifact(t, spoSourceForTest(t))
	for _, key := range []string{
		spobackend.ManagedByAnnotation,
		spobackend.NameSchemeAnnotation,
		spobackend.TargetNamespaceAnnotation,
		spobackend.TargetPodAnnotation,
		spobackend.TargetContainerAnnotation,
		spobackend.SeccompSourceAnnotation,
	} {
		if !strings.Contains(artifact, key) {
			t.Errorf("governed artifact is missing %s:\n%s", key, artifact)
		}
	}
}

// --- Standalone regression -------------------------------------------------

// failingDynamic fails every call. Internal mode must never touch it: a
// user who has not selected SPO must not need SPO, its CRDs, cert-manager,
// or a ProfileRecording for the existing pipeline to work.
type failingDynamic struct{ t *testing.T }

func (f failingDynamic) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	f.t.Errorf("internal mode reached the Kubernetes API for %v; standalone must not depend on SPO being present", gvr)
	return nil
}

// The strongest form of "standalone is unchanged": internal synthesis
// completes against a client that would fail on any use, so it provably
// consults nothing about SPO.
func TestResolveSeccompSource_InternalNeverTouchesSPO(t *testing.T) {
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out, failingDynamic{t},
		traceOptions{seccompSource: spobackend.SeccompSourceInternal},
		spoTarget(), behaviorWithSyscalls("openat", "read"))
	if err != nil {
		t.Fatalf("internal mode failed: %v", err)
	}
	if !src.hasProfile() || src.isSPO() {
		t.Error("internal mode did not resolve to internal synthesis")
	}
	if out.Len() != 0 {
		t.Errorf("internal mode announced an SPO source: %q", out.String())
	}
}

// In SPO mode the enforced authority is the imported one. Even when this
// project has observed syscalls, they must not contribute — a mixed profile
// would be authority nobody reviewed as a whole.
func TestResolveSeccompSource_SPOExcludesInternalSyscalls(t *testing.T) {
	var out bytes.Buffer
	src, err := resolveSeccompSource(context.Background(), &out,
		fakeSPOCluster(validSPORecording(), validSPOProfile()),
		traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "nginx-rec", spoProfile: "nginx-rec-tools"},
		spoTarget(), behaviorWithSyscalls("execve", "ptrace", "mount"))
	if err != nil {
		t.Fatalf("resolveSeccompSource() error = %v", err)
	}

	var names []string
	for _, rule := range src.profile.Syscalls {
		names = append(names, rule.Names...)
	}
	joined := strings.Join(names, ",")
	for _, observed := range []string{"execve", "ptrace", "mount"} {
		if strings.Contains(joined, observed) {
			t.Errorf("internally-observed syscall %q leaked into the imported profile (%s)", observed, joined)
		}
	}
	if joined != "openat,read" {
		t.Errorf("imported syscalls = %q, want exactly the source profile's own", joined)
	}
}

// --- SPO API facts live in exactly one place ------------------------------

// A compatibility claim in two places is a compatibility claim that will
// disagree with itself — which is precisely how the v0.8.4/v0.9.0 scope
// change went unnoticed. Constants belong to internal/spobackend.
func TestSPOConstantsAreNotDuplicatedOutsideTheAdapter(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	var offenders []string
	err = filepath.Walk(filepath.Join(root, "cmd"), collectSPOLiterals(&offenders))
	if err != nil {
		t.Fatalf("walking cmd: %v", err)
	}
	for _, sub := range []string{"exporter", "spoimport", "k8s", "proposal"} {
		if err := filepath.Walk(filepath.Join(root, "internal", sub), collectSPOLiterals(&offenders)); err != nil {
			t.Fatalf("walking internal/%s: %v", sub, err)
		}
	}

	if len(offenders) > 0 {
		t.Errorf("SPO API strings appear outside internal/spobackend:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// collectSPOLiterals reports non-test Go files containing a raw SPO API
// string. Test files are exempt: fixtures legitimately spell out what the
// adapter should produce, which is how a wrong constant gets caught.
func collectSPOLiterals(out *[]string) filepath.WalkFunc {
	needles := []string{"spo.x-k8s.io/", "security-profiles-operator.x-k8s.io", "operator/"}
	return func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, line := range strings.Split(string(data), "\n") {
			code := line
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx] // comments may discuss the strings freely
			}
			for _, needle := range needles {
				if strings.Contains(code, `"`+needle) {
					*out = append(*out, path+": "+strings.TrimSpace(line))
				}
			}
		}
		return nil
	}
}

// A refused import is a blocking governance failure, the same class as
// ADR-0007's enforcement-readiness refusal. Exit 1 would report it as a
// non-blocking finding, and exit 3 would report it as a typo.
func TestResolveSeccompSource_ImportRefusalIsBlocking(t *testing.T) {
	var out bytes.Buffer
	_, err := resolveSeccompSource(context.Background(), &out, fakeSPOCluster(),
		traceOptions{seccompSource: spobackend.SeccompSourceSPO, spoRecording: "nginx-rec", spoProfile: "nginx-rec-tools"},
		spoTarget(), profile.BehaviorProfile{})
	if err == nil {
		t.Fatal("a missing source did not fail")
	}
	coder, ok := err.(interface{ ExitCode() int })
	if !ok {
		t.Fatal("import refusal carries no exit code")
	}
	if coder.ExitCode() != 2 {
		t.Errorf("import refusal exit code = %d, want 2 (blocking failure)", coder.ExitCode())
	}
	if !errors.Is(err, spoimport.ErrImport) {
		t.Error("import refusal no longer unwraps to spoimport.ErrImport")
	}
}
