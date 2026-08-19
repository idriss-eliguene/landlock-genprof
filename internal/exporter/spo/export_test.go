// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package spo

import (
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
	pkgspo "github.com/idriss-eliguene/landlock-genprof/pkg/spo"
)

func mockNginxProfile() *seccomp.Profile {
	return &seccomp.Profile{
		DefaultAction: "SCMP_ACT_ERRNO",
		Architectures: []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86"},
		Syscalls: []seccomp.SyscallRule{
			{Names: []string{"epoll_wait", "openat"}, Action: "SCMP_ACT_ALLOW"},
		},
	}
}

func testMeta() Meta {
	return Meta{Namespace: "default", Pod: "nginx-demo", Container: "nginx"}
}

func TestToSeccompProfile_MirrorsFieldForField(t *testing.T) {
	p := mockNginxProfile()
	cr := ToSeccompProfile(testMeta(), p)

	if cr.APIVersion != spobackend.APIVersion || cr.Kind != "SeccompProfile" {
		t.Errorf("TypeMeta = {%q %q}, want {%q SeccompProfile}", cr.APIVersion, cr.Kind, spobackend.APIVersion)
	}
	if cr.Spec.DefaultAction != p.DefaultAction {
		t.Errorf("Spec.DefaultAction = %q, want %q", cr.Spec.DefaultAction, p.DefaultAction)
	}
	if !reflect.DeepEqual(cr.Spec.Architectures, p.Architectures) {
		t.Errorf("Spec.Architectures = %v, want %v", cr.Spec.Architectures, p.Architectures)
	}
	if len(cr.Spec.Syscalls) != 1 || !reflect.DeepEqual(cr.Spec.Syscalls[0].Names, p.Syscalls[0].Names) ||
		cr.Spec.Syscalls[0].Action != p.Syscalls[0].Action {
		t.Errorf("Spec.Syscalls = %+v, want a field-for-field mirror of %+v", cr.Spec.Syscalls, p.Syscalls)
	}
}

// SeccompProfile is cluster-scoped on the targeted API, so the rendered
// manifest must carry the governed name and no namespace at all — a
// namespace on a cluster-scoped object is rejected by the API server.
func TestToSeccompProfile_ClusterScopedIdentity(t *testing.T) {
	meta := testMeta()
	cr := ToSeccompProfile(meta, mockNginxProfile())

	if cr.Metadata.Name != ProfileName(meta) {
		t.Errorf("Metadata.Name = %q, want the governed name %q", cr.Metadata.Name, ProfileName(meta))
	}

	out, err := ToYAML(cr)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	// Decode rather than substring-match: the ownership annotation key
	// itself ends in "target-namespace", so a naive text search finds a
	// namespace that isn't there.
	var doc struct {
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if _, found := doc.Metadata["namespace"]; found {
		t.Errorf("rendered SeccompProfile carries metadata.namespace, but the resource is cluster-scoped:\n%s", out)
	}
}

// The ownership annotations are what let the apply path tell "ours, same
// workload" from "ours, different workload that collided" — see
// docs/adr/0008.
func TestToSeccompProfile_CarriesOwnershipAnnotations(t *testing.T) {
	meta := testMeta()
	cr := ToSeccompProfile(meta, mockNginxProfile())

	want := map[string]string{
		spobackend.ManagedByAnnotation:       spobackend.ManagedByValue,
		spobackend.NameSchemeAnnotation:      spobackend.NameScheme,
		spobackend.TargetNamespaceAnnotation: "default",
		spobackend.TargetPodAnnotation:       "nginx-demo",
		spobackend.TargetContainerAnnotation: "nginx",
	}
	for key, value := range want {
		if got := cr.Metadata.Annotations[key]; got != value {
			t.Errorf("annotation %s = %q, want %q", key, got, value)
		}
	}
}

func TestToYAML_ProducesApplyableManifest(t *testing.T) {
	cr := ToSeccompProfile(testMeta(), mockNginxProfile())

	out, err := ToYAML(cr)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	var got pkgspo.SeccompProfile
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&got, cr) {
		t.Errorf("round-tripped = %+v, want %+v", got, *cr)
	}
}

func TestLocalhostProfilePath_HasNoNamespaceSegment(t *testing.T) {
	meta := testMeta()
	got := LocalhostProfilePath(meta)
	want := "operator/" + ProfileName(meta) + ".json"
	if got != want {
		t.Errorf("LocalhostProfilePath() = %q, want %q", got, want)
	}
	// "operator/<ns>/<name>.json" was the SPO v0.8.4 form; the targeted
	// API materializes profiles without a namespace segment.
	if strings.Count(got, "/") != 1 {
		t.Errorf("LocalhostProfilePath() = %q, want exactly one separator", got)
	}
}

// Two workloads that differ only by namespace must not collide on one
// cluster-scoped object — the property the old namespaced path used to
// provide for free.
func TestProfileName_DistinguishesNamespaces(t *testing.T) {
	a := ProfileName(Meta{Namespace: "default", Pod: "nginx-demo", Container: "nginx"})
	b := ProfileName(Meta{Namespace: "staging", Pod: "nginx-demo", Container: "nginx"})
	if a == b {
		t.Errorf("ProfileName() = %q for both namespaces, want distinct cluster-scoped names", a)
	}
}
