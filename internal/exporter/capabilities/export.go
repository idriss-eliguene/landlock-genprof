// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package capabilities converts a Behavior IR (internal/profile) into a
// Linux capabilities fragment and serializes it to YAML.
//
// This is a sibling of internal/exporter/podlock/networkpolicy/seccomp,
// but with a real structural difference: unlike a LandlockProfile,
// NetworkPolicy, or seccomp profile, Linux capabilities aren't a
// standalone Kubernetes object — they only ever live inside a
// container's own securityContext.capabilities field. So this package's
// output isn't a complete, `kubectl apply`-able (or kubelet-loadable)
// artifact the way the other three are: it's a bare corev1.Capabilities
// fragment (add/drop lists) for a human to paste directly under their
// container's securityContext.capabilities: key — confirmed as the right
// shape with the project owner, the alternative considered being a
// ready-to-run kubectl patch command instead.
//
// Reuses the already-vendored k8s.io/api/core/v1.Capabilities type
// rather than hand-rolling one in pkg/ — same reasoning
// internal/exporter/networkpolicy used for k8s.io/api/networking/v1.
package capabilities

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// capPrefix is how Inspektor Gadget's trace_capabilities gadget names
// every capability (e.g. "CAP_NET_BIND_SERVICE") — confirmed via
// gadgets/trace_capabilities/gadget.yaml's own "cap" field
// documentation. Kubernetes' own convention for
// corev1.Capabilities.Add/Drop omits it (e.g. "NET_BIND_SERVICE"), so
// ToProfile strips it.
const capPrefix = "CAP_"

// dropAll is always dropped: this exporter's whole point is a minimal
// capability set, the same "deny by default, only grant what was
// observed" philosophy internal/exporter/seccomp's defaultAction:
// SCMP_ACT_ERRNO already follows for syscalls.
const dropAll = corev1.Capability("ALL")

// ToProfile converts a BehaviorProfile's capability observations into a
// corev1.Capabilities fragment ready to be serialized. Drop always
// contains exactly "ALL" — Add lists every capability observed, sorted,
// with the gadget's own "CAP_" prefix stripped to match Kubernetes'
// convention.
func ToProfile(capabilities profile.CapabilityProfile) *corev1.Capabilities {
	names := make([]string, len(capabilities.Accesses))
	for i, access := range capabilities.Accesses {
		names[i] = StripCapPrefix(access.Name)
	}
	sort.Strings(names)

	var add []corev1.Capability
	if len(names) > 0 {
		add = make([]corev1.Capability, len(names))
		for i, name := range names {
			add[i] = corev1.Capability(name)
		}
	}

	return &corev1.Capabilities{
		Add:  add,
		Drop: []corev1.Capability{dropAll},
	}
}

// StripCapPrefix removes the gadget's "CAP_" prefix from a capability
// name, matching Kubernetes' own naming convention. Exported for
// internal/exporter/securitycontext, which needs the same stripped name
// to index confidence by — the one deliberate exporter-to-exporter
// dependency in this codebase, see that package's own doc comment.
func StripCapPrefix(name string) string {
	return strings.TrimPrefix(name, capPrefix)
}

// ToYAML serializes a capabilities fragment to YAML, as written to
// <pod>-capabilities.yaml by the CLI (see cmd/landlock-genprof), with a
// trailing `# confidence: ...` comment on each entry in add — capabilities
// is the same value ToProfile converted, carrying the per-capability
// Confidence that conversion doesn't otherwise preserve. Legal here
// (unlike internal/exporter/seccomp.ToJSON): this fragment is meant for
// a human to paste manually, not loaded directly by the kubelet/runtime,
// so YAML comments don't break anything downstream — see the package
// doc.
func ToYAML(p *corev1.Capabilities, capabilities profile.CapabilityProfile) ([]byte, error) {
	raw, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}

	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("re-parsing generated YAML to attach confidence comments: %w", err)
	}
	annotateConfidence(&doc, confidenceByAddedName(capabilities))

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

// confidenceByAddedName indexes capabilities by the stripped name that
// appears under the serialized "add" list, for annotateConfidence's
// lookups.
func confidenceByAddedName(capabilities profile.CapabilityProfile) map[string]profile.Confidence {
	m := make(map[string]profile.Confidence, len(capabilities.Accesses))
	for _, a := range capabilities.Accesses {
		m[StripCapPrefix(a.Name)] = a.Confidence
	}
	return m
}

// annotateConfidence sets a LineComment on each scalar under the top-level
// "add" key, if confidenceByAddedName has a non-empty Confidence for it —
// no recursion needed the way podlock/networkpolicy's walkers require:
// corev1.Capabilities is flat (just add/drop, both plain string lists),
// so only the "add" sequence's direct children are ever candidates. A
// Confidence of "" (the zero value) is left uncommented, same reasoning
// as the other two YAML exporters.
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
