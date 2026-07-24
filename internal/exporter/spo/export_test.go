// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package spo

import (
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"

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

func TestToSeccompProfile_MirrorsFieldForField(t *testing.T) {
	p := mockNginxProfile()
	meta := Meta{Name: "nginx-demo", Namespace: "default"}

	cr := ToSeccompProfile(meta, p)

	if cr.APIVersion != apiVersion || cr.Kind != "SeccompProfile" {
		t.Errorf("TypeMeta = {%q %q}, want {%q SeccompProfile}", cr.APIVersion, cr.Kind, apiVersion)
	}
	if cr.Metadata.Name != "nginx-demo" || cr.Metadata.Namespace != "default" {
		t.Errorf("Metadata = %+v, want {nginx-demo default}", cr.Metadata)
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

func TestToYAML_ProducesApplyableManifest(t *testing.T) {
	cr := ToSeccompProfile(Meta{Name: "nginx-demo", Namespace: "default"}, mockNginxProfile())

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

func TestLocalhostProfilePath_MatchesSPOConvention(t *testing.T) {
	got := LocalhostProfilePath(Meta{Name: "nginx-demo", Namespace: "default"})
	want := "operator/nginx-demo.json"
	if got != want {
		t.Errorf("LocalhostProfilePath() = %q, want %q", got, want)
	}
}
