// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package securitycontext composes a Behavior IR (internal/profile) and
// a reference to a separately-generated seccomp profile into a
// Kubernetes corev1.SecurityContext fragment, and serializes it to YAML.
//
// This exists instead of merging internal/exporter/seccomp and
// internal/exporter/capabilities into one backend: a seccomp profile
// still has to ship as its own file for the kubelet to load
// (corev1.SeccompProfile.LocalhostProfile only ever takes a path
// reference, never inline content — "Must be a descending path,
// relative to the kubelet's configured seccomp profile location", per
// its own doc comment in k8s.io/api/core/v1), so a true merge would
// still produce two files, just with more indirection. This package
// instead composes the *already-computed* capabilities fragment with a
// *reference* to the seccomp file (see ToSecurityContext), as a third,
// additional view — internal/exporter/seccomp and
// internal/exporter/capabilities are unchanged and still independently
// usable.
//
// Deliberately does not set corev1.SecurityContext's other fields
// (Privileged, RunAsUser, RunAsNonRoot, ReadOnlyRootFilesystem,
// AllowPrivilegeEscalation, ...): nothing in this codebase observes any
// of them today, and stamping in "safe defaults" regardless of what was
// actually seen would contradict this project's own positioning
// (observe, don't guess — see docs/roadmap.md's "Architecture decisions
// made"). RunAsUser might be legitimately derivable later from process
// credentials Inspektor Gadget's gadget_process struct already carries,
// but that's new tracer work, not this package's job.
//
// First exporter-to-exporter dependency in this codebase: this package
// imports internal/exporter/capabilities to reuse its ToProfile (same
// CAP_-prefix-stripping/sorting logic, not duplicated here). Every
// exporter before this one only ever depended on internal/profile — not
// a violation of the documented IR rule (internal/profile itself still
// never depends on an exporter, enforced by
// internal/profile/deps_test.go), just a new, deliberate exception to
// "exporters are siblings, never depend on each other" now that there's
// a real reason to (avoiding a second, driftable copy of the same
// conversion logic).
package securitycontext

import (
	"bytes"
	"fmt"

	yamlv3 "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/capabilities"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// ToSecurityContext composes capabilities and, if seccompLocalhostProfile
// is non-empty, a seccomp profile reference into a corev1.SecurityContext
// fragment ready to be serialized.
//
// seccompLocalhostProfile must be the exact filename a seccomp profile
// was actually written under this run (see
// cmd/landlock-genprof/trace.go's writeSecurityContext) — never a
// filename that wasn't genuinely produced, so this fragment never
// references a file that doesn't exist. Kubernetes' own
// LocalhostProfile field takes a path relative to the kubelet's
// configured seccomp root (typically /var/lib/kubelet/seccomp/), not a
// path on this machine — the caller is responsible for passing just the
// basename, and the operator applying this fragment is responsible for
// copying the seccomp file to that directory under that exact name.
func ToSecurityContext(capabilitiesProfile profile.CapabilityProfile, seccompLocalhostProfile string) *corev1.SecurityContext {
	sc := &corev1.SecurityContext{}

	if len(capabilitiesProfile.Accesses) > 0 {
		sc.Capabilities = capabilities.ToProfile(capabilitiesProfile)
	}

	if seccompLocalhostProfile != "" {
		localhostProfile := seccompLocalhostProfile
		sc.SeccompProfile = &corev1.SeccompProfile{
			Type:             corev1.SeccompProfileTypeLocalhost,
			LocalhostProfile: &localhostProfile,
		}
	}

	return sc
}

// ToYAML serializes a SecurityContext fragment to YAML, as written to
// <pod>-securitycontext.yaml by the CLI (see cmd/landlock-genprof), with
// a trailing `# confidence: ...` comment on each capabilities.add entry
// — capabilitiesProfile is the same value ToSecurityContext converted,
// carrying the per-capability Confidence that conversion doesn't
// otherwise preserve. Legal here (unlike internal/exporter/seccomp.
// ToJSON): this fragment is meant for a human to paste manually under a
// container's own securityContext: key, not loaded directly by the
// kubelet/runtime, so YAML comments don't break anything downstream —
// see internal/exporter/capabilities.ToYAML for the same reasoning.
func ToYAML(p *corev1.SecurityContext, capabilitiesProfile profile.CapabilityProfile) ([]byte, error) {
	raw, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}

	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("re-parsing generated YAML to attach confidence comments: %w", err)
	}
	annotateConfidence(&doc, confidenceByAddedName(capabilitiesProfile))

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("re-encoding YAML with confidence comments: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// confidenceByAddedName indexes capabilitiesProfile by the stripped name
// that appears under the serialized "capabilities.add" list, for
// annotateConfidence's lookups — same helper as
// internal/exporter/capabilities.confidenceByAddedName, not shared code
// (this package doesn't import that private function, only the public
// ToProfile).
func confidenceByAddedName(capabilitiesProfile profile.CapabilityProfile) map[string]profile.Confidence {
	m := make(map[string]profile.Confidence, len(capabilitiesProfile.Accesses))
	for _, a := range capabilitiesProfile.Accesses {
		m[capabilities.StripCapPrefix(a.Name)] = a.Confidence
	}
	return m
}

// annotateConfidence sets a LineComment on each scalar under
// capabilities.add, one level deeper than
// internal/exporter/capabilities.annotateConfidence since this output
// nests capabilities under its own key alongside seccompProfile, rather
// than being the bare add/drop fragment capabilities.yaml is.
func annotateConfidence(node *yamlv3.Node, confidenceByAddedName map[string]profile.Confidence) {
	if node.Kind == yamlv3.DocumentNode {
		for _, child := range node.Content {
			annotateConfidence(child, confidenceByAddedName)
		}
		return
	}
	if node.Kind != yamlv3.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Value == "capabilities" {
			annotateCapabilitiesAdd(value, confidenceByAddedName)
		}
	}
}

// annotateCapabilitiesAdd is the same walk
// internal/exporter/capabilities.annotateConfidence does over a bare
// {add, drop} mapping — factored out here since it's now one level
// deeper, not the document root.
func annotateCapabilitiesAdd(node *yamlv3.Node, confidenceByAddedName map[string]profile.Confidence) {
	if node.Kind != yamlv3.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Value != "add" {
			continue
		}
		for _, item := range value.Content {
			if c, ok := confidenceByAddedName[item.Value]; ok && c != "" {
				item.LineComment = "confidence: " + string(c)
			}
		}
	}
}
