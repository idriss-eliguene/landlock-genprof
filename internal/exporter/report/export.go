// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package report renders a Behavior IR (internal/profile) into a single
// Markdown review artifact combining all four observed domains
// (filesystem, network, syscalls, capabilities) for one human review
// pass, instead of the four-to-five separate files the other exporters
// each produce on their own.
//
// Unlike internal/exporter/securitycontext, this package only depends on
// internal/profile: it presents IR data directly rather than converting
// it into another schema, so there's nothing to reuse from a sibling
// exporter.
//
// Self-contained, not just an index: internal/policy.Synthesize already
// populates all four IR domains every run, regardless of which
// individual --*-out flags were passed (all six gadgets always run —
// see docs/architecture.md §2), so the report shows the real data
// directly rather than only pointing at files that may not exist. When
// a sibling artifact was also generated this run, the report links to
// it by filename (see GeneratedFiles).
package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// Meta identifies the training run a report summarizes.
type Meta struct {
	Name      string
	Namespace string
	Container string
	Binary    string
	Duration  time.Duration
	// HistoryUsed is whether --history was passed this run — tailors the
	// checklist and the syscall-confidence note, since both read
	// differently once Confidence reflects a real cross-run ratio
	// instead of internal/policy.confidenceFor's single-run proxy (see
	// docs/policy-synthesis.md).
	HistoryUsed bool
}

// GeneratedFiles records which sibling artifacts were actually written
// this same run — empty string means that one wasn't (either the flag
// was omitted, or nothing was observed for it, mirroring exactly the
// same skip condition each exporter's own write* function in
// cmd/landlock-genprof/trace.go already checks). ToMarkdown links to
// whichever of these are set instead of repeating their content
// verbatim.
type GeneratedFiles struct {
	Profile         string
	NetworkPolicy   string
	Seccomp         string
	Capabilities    string
	SecurityContext string
}

// ToMarkdown renders behavior into a single Markdown review report.
func ToMarkdown(meta Meta, behavior profile.BehaviorProfile, files GeneratedFiles) []byte {
	var b strings.Builder

	writeHeader(&b, meta)
	writeFilesystemSection(&b, behavior.Filesystem, files.Profile)
	writeNetworkSection(&b, behavior.Network, files.NetworkPolicy)
	writeSyscallsSection(&b, behavior.Syscalls, files.Seccomp, meta.HistoryUsed)
	writeCapabilitiesSection(&b, behavior.Capabilities, files.Capabilities, files.SecurityContext)
	writeChecklist(&b, meta, behavior)

	return []byte(b.String())
}

func writeHeader(b *strings.Builder, meta Meta) {
	fmt.Fprintf(b, "# Security Profile Review — %s\n\n", meta.Name)
	fmt.Fprintf(b, "- **Generated:** %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(b, "- **Namespace/Container:** %s/%s\n", meta.Namespace, meta.Container)
	fmt.Fprintf(b, "- **Binary:** %s\n", meta.Binary)
	fmt.Fprintf(b, "- **Training duration:** %s\n", meta.Duration)
	historyNote := "no — Confidence below is internal/policy's single-run proxy, not a real cross-run ratio"
	if meta.HistoryUsed {
		historyNote = "yes — Confidence below reflects the real cross-run ratio"
	}
	fmt.Fprintf(b, "- **--history used:** %s\n\n", historyNote)
}

func writeFilesystemSection(b *strings.Builder, fs profile.FilesystemProfile, generatedFile string) {
	b.WriteString("## Filesystem\n\n")
	if generatedFile != "" {
		fmt.Fprintf(b, "See [`%s`](%s) for the full PodLock profile.\n\n", generatedFile, generatedFile)
	}
	if len(fs.Accesses) == 0 {
		b.WriteString("No filesystem access observed.\n\n")
		return
	}
	b.WriteString("| Path | Permissions | Confidence |\n|---|---|---|\n")
	for _, a := range fs.Accesses {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", a.Path, joinPermissions(a.Permissions), a.Confidence)
	}
	b.WriteString("\n")
}

func joinPermissions(perms []profile.FilePermission) string {
	names := make([]string, len(perms))
	for i, p := range perms {
		names[i] = string(p)
	}
	return strings.Join(names, ", ")
}

func writeNetworkSection(b *strings.Builder, net profile.NetworkProfile, generatedFile string) {
	b.WriteString("## Network\n\n")
	if generatedFile != "" {
		fmt.Fprintf(b, "See [`%s`](%s) for the full NetworkPolicy.\n\n", generatedFile, generatedFile)
	}
	if len(net.Accesses) == 0 {
		b.WriteString("No network activity observed.\n\n")
		return
	}
	b.WriteString("| Port | Direction | Confidence |\n|---|---|---|\n")
	for _, a := range net.Accesses {
		fmt.Fprintf(b, "| %d | %s | %s |\n", a.Port, a.Direction, a.Confidence)
	}
	b.WriteString("\n")
}

func writeSyscallsSection(b *strings.Builder, syscalls profile.SyscallProfile, generatedFile string, historyUsed bool) {
	b.WriteString("## Syscalls\n\n")
	if generatedFile != "" {
		fmt.Fprintf(b, "See [`%s`](%s) for the full seccomp profile.\n\n", generatedFile, generatedFile)
	}
	if len(syscalls.Accesses) == 0 {
		b.WriteString("No syscalls observed. If this container was already running before this " +
			"trace started, this is likely the startup blind spot, not a real absence of syscalls " +
			"— see `docs/e2e-demo.md` Finding 2 and re-run with `--restart`.\n\n")
		return
	}
	if !historyUsed {
		b.WriteString("⚠ **Every syscall below is Low confidence without `--history`**: " +
			"the seccomp gadget reports one deduplicated set per run, not per-occurrence events, " +
			"so a single run can never confirm completeness — see `docs/policy-synthesis.md`'s " +
			"\"Syscall aggregation\" section.\n\n")
	}
	b.WriteString("| Syscall | Confidence |\n|---|---|\n")
	for _, a := range syscalls.Accesses {
		fmt.Fprintf(b, "| `%s` | %s |\n", a.Name, a.Confidence)
	}
	b.WriteString("\n")
}

func writeCapabilitiesSection(b *strings.Builder, capabilities profile.CapabilityProfile, capabilitiesFile, securityContextFile string) {
	b.WriteString("## Capabilities\n\n")
	for _, generatedFile := range []string{capabilitiesFile, securityContextFile} {
		if generatedFile != "" {
			fmt.Fprintf(b, "See [`%s`](%s).\n\n", generatedFile, generatedFile)
		}
	}
	if len(capabilities.Accesses) == 0 {
		b.WriteString("No capability checks observed. Capability checks cluster heavily at " +
			"container startup (privilege drop, binding a privileged port) — if this container " +
			"was already running before this trace started, there may be nothing left to observe " +
			"— see `docs/e2e-demo.md` Finding 5 and re-run with `--restart`.\n\n")
		return
	}
	b.WriteString("| Capability | Confidence |\n|---|---|\n")
	for _, a := range capabilities.Accesses {
		fmt.Fprintf(b, "| `%s` | %s |\n", a.Name, a.Confidence)
	}
	b.WriteString("\n")
}

func writeChecklist(b *strings.Builder, meta Meta, behavior profile.BehaviorProfile) {
	b.WriteString("## Review checklist\n\n")
	if !meta.HistoryUsed {
		b.WriteString("- [ ] Re-run with `--history` a few times before trusting any `low`/`medium` " +
			"entry above — a single run only measures what happened *within* that run, not across " +
			"separate runs (see `docs/policy-synthesis.md`).\n")
	}
	if len(behavior.Capabilities.Accesses) == 0 || len(behavior.Syscalls.Accesses) == 0 {
		b.WriteString("- [ ] Re-run with `--restart` — capabilities and/or syscalls came back empty, " +
			"which usually means the container was already running and startup-only activity was " +
			"missed (see `docs/e2e-demo.md` Findings 2 and 5).\n")
	}
	b.WriteString("- [ ] Review every `low`/`medium` confidence entry above before enforcing anything.\n")
	b.WriteString("- [ ] Seccomp/capabilities are fatal if wrong (a missing syscall or capability " +
		"breaks the container outright) — review those two sections with extra care.\n")
	b.WriteString("- [ ] See `docs/threat-model.md` for the full recommended validation methodology.\n")
}
