// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package securitycontext

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// mockNginxCapabilityProfile mirrors
// internal/exporter/capabilities/export_test.go's own fixture.
func mockNginxCapabilityProfile() profile.CapabilityProfile {
	return profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{
			{Name: "CAP_NET_BIND_SERVICE", Confidence: profile.ConfidenceHigh, SeenCount: 3},
			{Name: "CAP_SYS_NICE", Confidence: profile.ConfidenceLow, SeenCount: 1},
		},
	}
}

func TestToSecurityContext_CapabilitiesAndSeccomp(t *testing.T) {
	result := ToSecurityContext(mockNginxCapabilityProfile(), "nginx-demo-seccomp.json")

	if result.Capabilities == nil {
		t.Fatalf("Capabilities = nil, want set")
	}
	wantAdd := []corev1.Capability{"NET_BIND_SERVICE", "SYS_NICE"}
	if !reflect.DeepEqual(result.Capabilities.Add, wantAdd) {
		t.Errorf("Capabilities.Add = %v, want %v", result.Capabilities.Add, wantAdd)
	}
	if !reflect.DeepEqual(result.Capabilities.Drop, []corev1.Capability{"ALL"}) {
		t.Errorf("Capabilities.Drop = %v, want [ALL]", result.Capabilities.Drop)
	}

	if result.SeccompProfile == nil {
		t.Fatalf("SeccompProfile = nil, want set")
	}
	if result.SeccompProfile.Type != corev1.SeccompProfileTypeLocalhost {
		t.Errorf("SeccompProfile.Type = %q, want Localhost", result.SeccompProfile.Type)
	}
	if result.SeccompProfile.LocalhostProfile == nil || *result.SeccompProfile.LocalhostProfile != "nginx-demo-seccomp.json" {
		t.Errorf("SeccompProfile.LocalhostProfile = %v, want nginx-demo-seccomp.json", result.SeccompProfile.LocalhostProfile)
	}

	// Deliberately never set: nothing in this codebase observes any of
	// these, so stamping in "safe defaults" would misrepresent what was
	// actually seen — see the package doc.
	if result.Privileged != nil || result.RunAsNonRoot != nil || result.RunAsUser != nil ||
		result.ReadOnlyRootFilesystem != nil || result.AllowPrivilegeEscalation != nil {
		t.Errorf("expected all unobserved fields to stay nil, got %+v", result)
	}
}

// TestToSecurityContext_CapabilitiesOnly checks that an empty
// seccompLocalhostProfile (no seccomp file was written this run) leaves
// SeccompProfile nil rather than a dangling/empty reference.
func TestToSecurityContext_CapabilitiesOnly(t *testing.T) {
	result := ToSecurityContext(mockNginxCapabilityProfile(), "")

	if result.Capabilities == nil {
		t.Fatalf("Capabilities = nil, want set")
	}
	if result.SeccompProfile != nil {
		t.Errorf("SeccompProfile = %+v, want nil (no seccompLocalhostProfile given)", result.SeccompProfile)
	}
}

// TestToSecurityContext_Empty checks that nothing observed and no
// seccomp reference produces a valid, empty-fielded SecurityContext, not
// a nil pointer or an error.
func TestToSecurityContext_Empty(t *testing.T) {
	result := ToSecurityContext(profile.CapabilityProfile{}, "")

	if result == nil {
		t.Fatal("ToSecurityContext() = nil, want a non-nil (but empty) *corev1.SecurityContext")
	}
	if result.Capabilities != nil {
		t.Errorf("Capabilities = %+v, want nil (nothing observed)", result.Capabilities)
	}
	if result.SeccompProfile != nil {
		t.Errorf("SeccompProfile = %+v, want nil (no seccompLocalhostProfile given)", result.SeccompProfile)
	}
}

func TestToYAML_RoundTrips(t *testing.T) {
	capabilitiesProfile := mockNginxCapabilityProfile()
	result := ToSecurityContext(capabilitiesProfile, "nginx-demo-seccomp.json")

	out, err := ToYAML(result, capabilitiesProfile)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	text := string(out)
	for _, want := range []string{"capabilities:", "add:", "drop:", "seccompProfile:", "localhostProfile: nginx-demo-seccomp.json", "type: Localhost"} {
		if !strings.Contains(text, want) {
			t.Errorf("YAML output missing expected content %q:\n%s", want, text)
		}
	}

	var roundTripped corev1.SecurityContext
	if err := yaml.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&roundTripped, result) {
		t.Errorf("round-tripped securityContext = %+v, want %+v", roundTripped, *result)
	}
}

// TestToYAML_AnnotatesConfidenceUnderCapabilitiesAdd checks that the
// confidence comment lands one level deeper than
// internal/exporter/capabilities.ToYAML does (capabilities.add, not a
// bare top-level add), and that seccompProfile's own lines stay
// uncommented.
func TestToYAML_AnnotatesConfidenceUnderCapabilitiesAdd(t *testing.T) {
	capabilitiesProfile := profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{
			{Name: "CAP_NET_BIND_SERVICE", Confidence: profile.ConfidenceHigh},
		},
	}
	result := ToSecurityContext(capabilitiesProfile, "nginx-demo-seccomp.json")

	out, err := ToYAML(result, capabilitiesProfile)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	lines := strings.Split(string(out), "\n")
	var addLine, seccompTypeLine string
	for _, line := range lines {
		if strings.Contains(line, "NET_BIND_SERVICE") {
			addLine = line
		}
		if strings.Contains(line, "type: Localhost") {
			seccompTypeLine = line
		}
	}
	if !strings.Contains(addLine, "confidence: high") {
		t.Errorf("capabilities.add entry line = %q, want a confidence: high comment", addLine)
	}
	if strings.Contains(seccompTypeLine, "confidence:") {
		t.Errorf("seccompProfile.type line = %q, want no confidence comment", seccompTypeLine)
	}
}

// TestToYAML_NoCommentForUnsetConfidence mirrors
// internal/exporter/capabilities's own test of the same name.
func TestToYAML_NoCommentForUnsetConfidence(t *testing.T) {
	capabilitiesProfile := profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{{Name: "CAP_SYS_NICE"}},
	}
	result := ToSecurityContext(capabilitiesProfile, "")

	out, err := ToYAML(result, capabilitiesProfile)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	if strings.Contains(string(out), "confidence:") {
		t.Errorf("expected no confidence comment for an unset Confidence, got:\n%s", out)
	}
}
