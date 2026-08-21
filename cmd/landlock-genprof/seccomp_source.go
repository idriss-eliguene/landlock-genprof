// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/spoimport"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"

	exportseccomp "github.com/idriss-eliguene/landlock-genprof/internal/exporter/seccomp"
)

// Seccomp source selection — docs/adr/0008, "Seccomp source modes".
//
// Two modes, always explicit. Auto-detection is prohibited: if the source
// were inferred from whether SPO happens to be installed, the meaning of
// the enforced profile — and of what `explain` says about it — would depend
// on invisible cluster state, and the same command would produce different
// authority on different clusters.
//
// There is no fallback in either direction. A caller who selected SPO and
// cannot have it gets a blocking failure, never internally-synthesised
// syscalls wearing SPO's name.

// usageErrorf builds a usage error carrying ADR-0001's exit code 3. A
// malformed invocation is not a governance refusal, and conflating the two
// would make a typo indistinguishable from a workload failing its gates.
func usageErrorf(format string, args ...interface{}) error {
	return &exitCodeError{code: 3, wrapped: fmt.Errorf(format, args...)}
}

func selectedSPOImportMode(opts traceOptions) spoimport.Mode {
	if opts.spoImportMode == "" {
		return spoimport.ModeStrongLineage
	}
	return spoimport.Mode(opts.spoImportMode)
}

// seccompSource is the resolved seccomp authority for one run: which
// source produced it, the enforcement content itself, and the provenance
// that will ride inside the governed artifact and therefore into the
// candidate digest.
//
// Resolved exactly once per run, so every downstream artifact — the local
// SeccompProfile file, the proposal's spoSeccompProfile, the
// localhostProfile the patched manifest references — is derived from one
// decision rather than each re-deciding for itself.
type seccompSource struct {
	kind       string
	profile    *seccomp.Profile
	provenance map[string]string
}

// hasProfile reports whether this run has seccomp authority to govern.
//
// In internal mode that means syscalls were observed; in SPO mode it is
// always true, because a failed import is a blocking error rather than an
// empty result.
func (s seccompSource) hasProfile() bool { return s.profile != nil }

// isSPO reports whether the authority came from SPO-derived policy.
func (s seccompSource) isSPO() bool { return s.kind == spobackend.SeccompSourceSPO }

// validateSeccompSourceFlags checks the flag combination before anything
// expensive happens — before a training run is started, not after.
//
// Returns a usage error (exit 3 under ADR-0001) rather than a blocking
// failure, because every case here is a malformed invocation rather than a
// governance refusal.
func validateSeccompSourceFlags(opts traceOptions) error {
	switch opts.seccompSource {
	case spobackend.SeccompSourceInternal:
		// Naming SPO material while selecting internal synthesis is
		// ambiguous about which authority should govern, and ADR-0008
		// resolves ambiguity by refusing rather than by picking one.
		if opts.spoRecording != "" || opts.spoProfile != "" || opts.spoRecordingNamespace != "" || selectedSPOImportMode(opts) != spoimport.ModeStrongLineage {
			return usageErrorf("--spo-recording/--spo-profile require --seccomp-source=%s; they name SPO material that internal synthesis would ignore, and silently ignoring them would hide which source is actually governing",
				spobackend.SeccompSourceSPO)
		}
	case spobackend.SeccompSourceSPO:
		if opts.spoRecording == "" || opts.spoProfile == "" {
			return usageErrorf("--seccomp-source=%s requires both --spo-recording and --spo-profile: the source is named explicitly and is never discovered by searching the cluster for a profile that looks plausible",
				spobackend.SeccompSourceSPO)
		}
		// ADR-0008 accepts that SPO mode produces no local plain-seccomp
		// output. Refusing is better than writing an empty or internally-
		// derived file that would disagree with the governed artifact.
		if opts.seccompOut != "" {
			return usageErrorf("--seccomp-out is not available with --seccomp-source=%s: in SPO mode this project does not observe syscalls at all, so there is no local seccomp profile to write that would agree with the imported authority",
				spobackend.SeccompSourceSPO)
		}
		switch selectedSPOImportMode(opts) {
		case spoimport.ModeStrongLineage:
			if opts.spoRecordingNamespace != "" {
				return usageErrorf("--spo-recording-namespace is only valid with --spo-import-mode=%s; strong lineage uses the target namespace",
					spoimport.ModeMergedProvenance)
			}
		case spoimport.ModeMergedProvenance:
			if opts.spoRecordingNamespace == "" {
				return usageErrorf("--spo-import-mode=%s requires --spo-recording-namespace so source provenance is independent of the application target",
					spoimport.ModeMergedProvenance)
			}
		default:
			return usageErrorf("--spo-import-mode must be %q or %q, got %q",
				spoimport.ModeStrongLineage, spoimport.ModeMergedProvenance, opts.spoImportMode)
		}
	default:
		return usageErrorf("--seccomp-source must be %q or %q, got %q",
			spobackend.SeccompSourceInternal, spobackend.SeccompSourceSPO, opts.seccompSource)
	}
	return nil
}

// dropSyscallObservations removes syscall events from a run's captured
// events.
//
// This is what makes the TrainingHistory boundary structural rather than
// conventional (docs/adr/0008): in SPO mode our syscall observations are
// not collected at all, instead of being collected and filtered out later.
// Filtering later would leave the invariant one forgotten branch away from
// violation, and would produce a TrainingHistory describing syscall
// authority that is not the authority being enforced — a semantic lie in
// the reviewer's own evidence view.
//
// Classification reuses tracer.ToObservation rather than re-testing Mode
// here, so there is exactly one notion of "this is a syscall observation"
// in the codebase and this filter cannot drift from the projector that
// feeds TrainingHistory.
func dropSyscallObservations(events []tracer.Event) []tracer.Event {
	kept := make([]tracer.Event, 0, len(events))
	for _, ev := range events {
		if tracer.ToObservation(ev).Kind == observation.KindSyscall {
			continue
		}
		kept = append(kept, ev)
	}
	return kept
}

// resolveSeccompSource turns the selected mode into the concrete seccomp
// authority for this run.
//
// Internal mode synthesises from what was observed. SPO mode imports,
// verifies and snapshots an SPO-generated profile through
// internal/spoimport, which fails closed on every gate — so a returned
// source is always one whose lineage, completeness, inertness and
// enforcement content were checked.
func resolveSeccompSource(
	ctx context.Context,
	stdout io.Writer,
	dyn dynamic.Interface,
	opts traceOptions,
	tgt spoimport.Target,
	behavior profile.BehaviorProfile,
) (seccompSource, error) {
	// Switched exhaustively rather than defaulting to internal on
	// anything-that-is-not-spo. validateSeccompSourceFlags already
	// rejected unknown values, but a boundary whose whole point is that
	// the source is never inferred should not have a branch that quietly
	// picks one if that check is ever bypassed or reordered.
	switch opts.seccompSource {
	case spobackend.SeccompSourceInternal:
		src := seccompSource{
			kind:       spobackend.SeccompSourceInternal,
			provenance: spobackend.InternalSeccompProvenance(),
		}
		if len(behavior.Syscalls.Accesses) > 0 {
			src.profile = exportseccomp.ToProfile(behavior.Syscalls)
		}
		return src, nil
	case spobackend.SeccompSourceSPO:
		// fall through to the import below
	default:
		return seccompSource{}, usageErrorf("unknown seccomp source %q", opts.seccompSource)
	}

	result, err := spoimport.Import(ctx, dyn, spoimport.Source{
		Mode:               selectedSPOImportMode(opts),
		RecordingName:      opts.spoRecording,
		RecordingNamespace: opts.spoRecordingNamespace,
		ProfileName:        opts.spoProfile,
	}, tgt)
	if err != nil {
		// No fallback. The caller asked for SPO-derived authority; giving
		// them internally-synthesised syscalls instead would silently
		// change what is being governed.
		//
		// Exit 2, matching ADR-0007's readiness gate: a refused import is a
		// blocking governance failure, not a non-blocking finding and not a
		// malformed invocation. CI branching on the exit code alone should
		// not have to tell an import refusal apart from an enforcement-
		// readiness refusal — both mean "this workload was not governed".
		return seccompSource{}, &exitCodeError{code: 2, wrapped: err}
	}

	if selectedSPOImportMode(opts) == spoimport.ModeMergedProvenance {
		fmt.Fprintf(stdout, "Seccomp source: SPO merged derived policy — imported %s (recording %s/%s; contributor lineage unavailable; target %s/%s container %s)\n",
			opts.spoProfile, opts.spoRecordingNamespace, opts.spoRecording, tgt.Namespace, tgt.Pod, tgt.Container)
	} else {
		fmt.Fprintf(stdout, "Seccomp source: SPO derived policy — imported %s (recording %s/%s, container %s, coverage %s)\n",
			opts.spoProfile, tgt.Namespace, opts.spoRecording, tgt.Container,
			result.Provenance[spobackend.SourceCoverageAnnotation])
	}

	return seccompSource{
		kind:       spobackend.SeccompSourceSPO,
		profile:    result.Profile,
		provenance: result.Provenance,
	}, nil
}

// Reviewer-facing provenance ------------------------------------------------

// seccompProvenance is what a reviewer needs to tell the two epistemic
// kinds apart without inference (docs/adr/0008, "Explainability").
//
// Read back out of the rendered artifact rather than carried alongside it,
// so what is displayed is exactly what is digested and approved. A reviewer
// looking at a stored proposal sees the same thing the digest bound.
type seccompProvenance struct {
	source             string
	origin             string
	sourceProfile      string
	recordingNamespace string
	recordingName      string
	container          string
	coverage           string
	sourceKind         string
	derivation         string
	mergeStrategy      string
	contributorLineage string
	targetNamespace    string
	targetPod          string
	targetContainer    string
}

// parseSeccompProvenance extracts provenance from a rendered SPO
// SeccompProfile artifact.
//
// ok is false for an artifact carrying no provenance at all, which is what
// a proposal generated before ADR-0008 looks like. Such a proposal is
// reported as unattributed rather than assumed to be internal: guessing
// would put a source label on something nobody recorded a source for.
func parseSeccompProvenance(artifact string) (seccompProvenance, bool) {
	if strings.TrimSpace(artifact) == "" {
		return seccompProvenance{}, false
	}
	var doc struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(artifact), &doc); err != nil {
		return seccompProvenance{}, false
	}
	a := doc.Metadata.Annotations
	source := a[spobackend.SeccompSourceAnnotation]
	if source == "" {
		return seccompProvenance{}, false
	}
	return seccompProvenance{
		source:             source,
		origin:             a[spobackend.SeccompOriginAnnotation],
		sourceProfile:      a[spobackend.SourceProfileAnnotation],
		recordingNamespace: a[spobackend.SourceRecordingNamespaceAnnotation],
		recordingName:      a[spobackend.SourceRecordingAnnotation],
		container:          a[spobackend.SourceContainerAnnotation],
		coverage:           a[spobackend.SourceCoverageAnnotation],
		sourceKind:         a[spobackend.SourceKindAnnotation],
		derivation:         a[spobackend.SourceDerivationAnnotation],
		mergeStrategy:      a[spobackend.SourceMergeStrategyAnnotation],
		contributorLineage: a[spobackend.ContributorLineageAnnotation],
		targetNamespace:    a[spobackend.TargetNamespaceAnnotation],
		targetPod:          a[spobackend.TargetPodAnnotation],
		targetContainer:    a[spobackend.TargetContainerAnnotation],
	}, true
}

// printSeccompProvenance renders the seccomp domain's epistemic status.
//
// The confidence line is the point of this block. Filesystem and network
// rules carry a landlock-genprof confidence tier derived from how often
// this project observed them across runs. SPO-derived syscalls have no such
// tier and must never be shown with one: SPO's generated profile carries
// syscall names with no occurrence data, so any tier would be invented.
// Printing "not applicable" states that plainly instead of leaving a blank
// a reviewer might read as "low".
func printSeccompProvenance(stdout io.Writer, artifact string) {
	prov, ok := parseSeccompProvenance(artifact)
	if !ok {
		if strings.TrimSpace(artifact) != "" {
			fmt.Fprintln(stdout, "Seccomp source: unattributed (artifact predates seccomp source provenance)")
		}
		return
	}

	fmt.Fprintln(stdout, "Seccomp:")
	switch prov.source {
	case spobackend.SeccompSourceSPO:
		fmt.Fprintln(stdout, "  Source: security-profiles-operator")
		fmt.Fprintln(stdout, "  Origin: derived policy (not observed by landlock-genprof)")
		fmt.Fprintf(stdout, "  Source profile: %s\n", prov.sourceProfile)
		fmt.Fprintf(stdout, "  Recording: %s/%s\n", prov.recordingNamespace, prov.recordingName)
		if prov.derivation == spobackend.SourceDerivationMerged {
			fmt.Fprintf(stdout, "  Source kind: %s\n", prov.sourceKind)
			fmt.Fprintf(stdout, "  Derivation: %s\n", prov.derivation)
			fmt.Fprintf(stdout, "  Merge strategy: %s\n", prov.mergeStrategy)
			fmt.Fprintf(stdout, "  Contributor lineage: %s\n", prov.contributorLineage)
			fmt.Fprintf(stdout, "  Application target: %s/%s container %s\n", prov.targetNamespace, prov.targetPod, prov.targetContainer)
			fmt.Fprintln(stdout, "  Widening warning: this profile is a union of SPO partial profiles and may contain syscalls learned from contributors other than the selected application target")
			printMergedCoverage(stdout, spoimport.ParseCanonicalCoverage(prov.coverage))
		} else {
			fmt.Fprintf(stdout, "  Container: %s\n", prov.container)
			fmt.Fprintf(stdout, "  Coverage: %s\n", prov.coverage)
		}
		fmt.Fprintln(stdout, "  Confidence: not applicable (derived policy carries no occurrence data)")
	case spobackend.SeccompSourceInternal:
		fmt.Fprintln(stdout, "  Source: landlock-genprof observation")
		fmt.Fprintln(stdout, "  Origin: observed")
	default:
		fmt.Fprintf(stdout, "  Source: %s (unrecognised)\n", prov.source)
	}
}

func printMergedCoverage(stdout io.Writer, coverage spoimport.Coverage) {
	switch coverage.State {
	case spoimport.CoverageAbsent:
		fmt.Fprintln(stdout, "  Syscall coverage: unavailable")
	case spoimport.CoverageMalformed:
		fmt.Fprintln(stdout, "  Syscall coverage: malformed metadata (no coverage value or confidence inferred)")
	case spoimport.CoverageUnsupported:
		fmt.Fprintf(stdout, "  Syscall coverage: unsupported schema %s (no coverage value or confidence inferred)\n", coverage.Version)
	case spoimport.CoverageKnown:
		fmt.Fprintf(stdout, "  Syscall coverage: schema %s; %d contributing partial profiles\n", coverage.Version, coverage.Total)
		names := make([]string, 0, len(coverage.Syscalls))
		for name := range coverage.Syscalls {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(stdout, "    %s: present in %d/%d contributing partial profiles\n", name, coverage.Syscalls[name], coverage.Total)
		}
	default:
		fmt.Fprintln(stdout, "  Syscall coverage: malformed metadata (no coverage value or confidence inferred)")
	}
}
