// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import (
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

func TestToProfile_MockNginxEvents(t *testing.T) {
	rules, err := Synthesize(mockNginxEvents())
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	meta := ProfileMeta{
		Name:      "nginx-demo",
		Namespace: "default",
		Container: "nginx",
		Binary:    "/usr/sbin/nginx",
	}
	profile := ToProfile(meta, rules)

	if profile.APIVersion != "podlock.kubewarden.io/v1alpha1" {
		t.Errorf("APIVersion = %q, want podlock.kubewarden.io/v1alpha1", profile.APIVersion)
	}
	if profile.Kind != "LandlockProfile" {
		t.Errorf("Kind = %q, want LandlockProfile", profile.Kind)
	}
	if profile.Metadata.Name != "nginx-demo" || profile.Metadata.Namespace != "default" {
		t.Errorf("Metadata = %+v, want {nginx-demo default}", profile.Metadata)
	}

	bp, ok := profile.Spec.ProfilesByContainer["nginx"]["/usr/sbin/nginx"]
	if !ok {
		t.Fatalf("no BinaryProfile for nginx//usr/sbin/nginx, got: %+v", profile.Spec.ProfilesByContainer)
	}

	if !reflect.DeepEqual(bp.ReadExec, []string{"/usr/sbin"}) {
		t.Errorf("ReadExec = %v, want [/usr/sbin]", bp.ReadExec)
	}
	if !reflect.DeepEqual(bp.ReadOnly, []string{"/etc/nginx", "/usr/share/nginx"}) {
		t.Errorf("ReadOnly = %v, want [/etc/nginx /usr/share/nginx]", bp.ReadOnly)
	}
	if !reflect.DeepEqual(bp.ReadWrite, []string{"/tmp", "/var/log/nginx"}) {
		t.Errorf("ReadWrite = %v, want [/tmp /var/log/nginx]", bp.ReadWrite)
	}
}

func TestToYAML_RoundTrips(t *testing.T) {
	rules, err := Synthesize(mockNginxEvents())
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	profile := ToProfile(ProfileMeta{
		Name:      "nginx-demo",
		Namespace: "default",
		Container: "nginx",
		Binary:    "/usr/sbin/nginx",
	}, rules)

	out, err := ToYAML(profile)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	// Les clés doivent être en camelCase (apiVersion, readOnly, ...), pas
	// le nom du champ Go (APIVersion, ReadOnly, ...) — c'est la garantie
	// que sigs.k8s.io/yaml lit bien les tags `json`, pas des tags `yaml`.
	text := string(out)
	for _, want := range []string{"apiVersion:", "profilesByContainer:", "readOnly:", "readWrite:", "readExec:"} {
		if !strings.Contains(text, want) {
			t.Errorf("YAML output missing expected key %q:\n%s", want, text)
		}
	}

	var roundTripped podlock.LandlockProfile
	if err := yaml.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&roundTripped, profile) {
		t.Errorf("round-tripped profile = %+v, want %+v", roundTripped, *profile)
	}
}
