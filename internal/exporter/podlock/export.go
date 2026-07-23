// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package podlock converts a Behavior IR (internal/profile) into the
// PodLock format (pkg/podlock) and serializes it to YAML.
//
// This is the only package that depends on both internal/profile and
// pkg/podlock — the dependency runs exporter -> IR, never the other way
// (see docs/architecture.md): internal/profile itself has no notion that
// PodLock exists, so this package is free to change or be joined by
// siblings (a Kubernetes NetworkPolicy exporter, Cilium, ...) without
// internal/profile or internal/policy ever needing to know about any of
// them.
package podlock

import (
	"bytes"
	"fmt"

	yamlv3 "gopkg.in/yaml.v3"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

// ProfileMeta identifies the pod/container/binary a BehaviorProfile
// applies to. Binary (the observed entry point's path) is specific to
// how PodLock indexes its rules — a future exporter (e.g. a Kubernetes
// NetworkPolicy one) would key its own output differently (podSelector,
// labels, ...), so this stays local to this package rather than becoming
// a shared, premature abstraction across exporters that don't exist yet.
type ProfileMeta struct {
	Name      string // name of the generated LandlockProfile (usually the pod's name)
	Namespace string
	Container string
	Binary    string // path of the container's main binary (observed entry point)
}

// ToProfile converts a BehaviorProfile's filesystem observations into a
// LandlockProfile ready to be serialized, in the format consumed by the
// PodLock operator. Only one profilesByContainer entry is produced
// (meta.Container -> meta.Binary), since a training run targets a single
// container at a time. Domains PodLock doesn't support (today: network)
// are simply never read from the IR — see docs/policy-synthesis.md.
func ToProfile(meta ProfileMeta, fs profile.FilesystemProfile) *podlock.LandlockProfile {
	var bp podlock.Profile
	for _, access := range fs.Accesses {
		switch categoryFor(access) {
		case "readExec":
			bp.ReadExec = append(bp.ReadExec, access.Path)
		case "readOnly":
			bp.ReadOnly = append(bp.ReadOnly, access.Path)
		case "readWrite":
			bp.ReadWrite = append(bp.ReadWrite, access.Path)
		case "readWriteExec":
			bp.ReadWriteExec = append(bp.ReadWriteExec, access.Path)
		}
	}

	return &podlock.LandlockProfile{
		APIVersion: "podlock.kubewarden.io/v1alpha1",
		Kind:       "LandlockProfile",
		Metadata: podlock.Metadata{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		},
		Spec: podlock.LandlockProfileSpec{
			ProfilesByContainer: map[string]podlock.ProfileByBinary{
				meta.Container: {
					meta.Binary: bp,
				},
			},
		},
	}
}

// categoryFor maps an IR permission set to exactly one of PodLock's four
// access categories (see pkg/podlock.Profile) — not a combination of
// several. A path that's both executed and written to is
// "readWriteExec", a distinct category of its own, not "readExec" and
// "readWrite" reported side by side: that mismatch was caught by
// checking PodLock's real schema (github.com/flavio/podlock), which
// mirrors what Landlock itself groups as one enforcement decision. Every
// named category also implies read access — there's no "execute but not
// read" bucket in PodLock's schema, matching the practical reality that
// executing or writing a file requires reading it first.
func categoryFor(access profile.FileAccess) string {
	switch {
	case access.HasPermission(profile.PermissionExecute) && access.HasPermission(profile.PermissionWrite):
		return "readWriteExec"
	case access.HasPermission(profile.PermissionExecute):
		return "readExec"
	case access.HasPermission(profile.PermissionWrite):
		return "readWrite"
	case access.HasPermission(profile.PermissionRead):
		return "readOnly"
	default:
		return ""
	}
}

// ToYAML serializes a LandlockProfile to YAML, as written to profile.yaml
// by the CLI (see cmd/landlock-genprof), with a trailing `# confidence:
// ...` comment on each path — fs is the same value ToProfile converted,
// carrying the per-path Confidence that conversion doesn't otherwise
// preserve (pkg/podlock.Profile mirrors PodLock's real schema, which has
// no field for it at all, see the package doc).
//
// Comments are invisible to kubectl apply (stripped at YAML-parse time)
// and to internal/exporter/podlock's own round-trip test (struct
// unmarshaling doesn't see them either) — this is purely for the human
// doing the mandatory review (docs/threat-model.md).
func ToYAML(p *podlock.LandlockProfile, fs profile.FilesystemProfile) ([]byte, error) {
	raw, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}

	// Re-parsed rather than built directly from p via yaml.v3: yaml.v3
	// has no notion of the `json` tags pkg/podlock's structs use for
	// their exact camelCase keys (apiVersion, readOnly, ...) — encoding
	// p directly would silently guess lowercase field names instead
	// (apiversion). Parsing sigs.k8s.io/yaml's own output back keeps
	// those keys exactly as already generated and tested.
	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("re-parsing generated YAML to attach confidence comments: %w", err)
	}
	annotateConfidence(&doc, confidenceByPath(fs))

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

// confidenceByPath indexes fs by path for annotateConfidence's lookups.
func confidenceByPath(fs profile.FilesystemProfile) map[string]profile.Confidence {
	m := make(map[string]profile.Confidence, len(fs.Accesses))
	for _, a := range fs.Accesses {
		m[a.Path] = a.Confidence
	}
	return m
}

// annotateConfidence walks a parsed LandlockProfile YAML tree and sets a
// LineComment on each path scalar found under a readOnly/readWrite/
// readExec/readWriteExec key, if confidenceByPath has a non-empty
// Confidence for it. Each path appears in exactly one category
// (categoryFor puts it there), so a flat path -> Confidence lookup is
// enough — no need to track which category a node is under while
// walking. A Confidence of "" (the zero value — e.g. a FileAccess built
// without setting it, as this package's own pre-existing test fixtures
// did) is left uncommented rather than printing a nonsensical
// "confidence: ".
func annotateConfidence(node *yamlv3.Node, confidenceByPath map[string]profile.Confidence) {
	switch node.Kind {
	case yamlv3.DocumentNode, yamlv3.SequenceNode:
		for _, child := range node.Content {
			annotateConfidence(child, confidenceByPath)
		}
	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			switch key.Value {
			case "readOnly", "readWrite", "readExec", "readWriteExec":
				for _, item := range value.Content {
					if c, ok := confidenceByPath[item.Value]; ok && c != "" {
						item.LineComment = "confidence: " + string(c)
					}
				}
			default:
				annotateConfidence(value, confidenceByPath)
			}
		}
	}
}
