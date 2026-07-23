// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package capabilities

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// mockNginxCapabilityProfile mirrors the shape internal/policy.Synthesize
// would produce for a typical nginx training run. Built directly as an
// IR fixture, not derived by calling Synthesize — same pattern as the
// other exporters' own tests.
func mockNginxCapabilityProfile() profile.CapabilityProfile {
	return profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{
			{Name: "CAP_NET_BIND_SERVICE", Confidence: profile.ConfidenceHigh, SeenCount: 3},
			{Name: "CAP_SYS_NICE", Confidence: profile.ConfidenceLow, SeenCount: 1},
		},
	}
}

func TestToProfile_MockNginxCapabilityProfile(t *testing.T) {
	result := ToProfile(mockNginxCapabilityProfile())

	if !reflect.DeepEqual(result.Drop, []corev1.Capability{"ALL"}) {
		t.Errorf("Drop = %v, want [ALL]", result.Drop)
	}
	// Sorted, "CAP_" prefix stripped to match Kubernetes' own convention.
	want := []corev1.Capability{"NET_BIND_SERVICE", "SYS_NICE"}
	if !reflect.DeepEqual(result.Add, want) {
		t.Errorf("Add = %v, want %v (sorted, CAP_ prefix stripped)", result.Add, want)
	}
}

// TestToProfile_EmptyCapabilityProfile checks that Drop still contains
// ALL even with nothing observed — "deny everything by default" doesn't
// depend on having observed anything, same as
// internal/exporter/seccomp.ToProfile's DefaultAction.
func TestToProfile_EmptyCapabilityProfile(t *testing.T) {
	result := ToProfile(profile.CapabilityProfile{})

	if !reflect.DeepEqual(result.Drop, []corev1.Capability{"ALL"}) {
		t.Errorf("Drop = %v, want [ALL] even with nothing observed", result.Drop)
	}
	if len(result.Add) != 0 {
		t.Errorf("Add = %v, want empty", result.Add)
	}
}

func TestToYAML_RoundTrips(t *testing.T) {
	capabilities := mockNginxCapabilityProfile()
	result := ToProfile(capabilities)

	out, err := ToYAML(result, capabilities)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	text := string(out)
	for _, want := range []string{"add:", "drop:", "NET_BIND_SERVICE", "SYS_NICE", "ALL"} {
		if !strings.Contains(text, want) {
			t.Errorf("YAML output missing expected content %q:\n%s", want, text)
		}
	}

	var roundTripped corev1.Capabilities
	if err := yaml.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&roundTripped, result) {
		t.Errorf("round-tripped capabilities = %+v, want %+v", roundTripped, *result)
	}
}

// TestToYAML_AnnotatesConfidenceOnAddOnly checks that the confidence
// comment lands on entries under "add" and that "drop" (always just
// "ALL", never carrying a meaningful Confidence) stays uncommented.
func TestToYAML_AnnotatesConfidenceOnAddOnly(t *testing.T) {
	capabilities := profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{
			{Name: "CAP_NET_BIND_SERVICE", Confidence: profile.ConfidenceHigh},
		},
	}
	result := ToProfile(capabilities)

	out, err := ToYAML(result, capabilities)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	lines := strings.Split(string(out), "\n")
	var addLine, dropLine string
	for _, line := range lines {
		if strings.Contains(line, "NET_BIND_SERVICE") {
			addLine = line
		}
		if strings.Contains(line, "ALL") {
			dropLine = line
		}
	}
	if !strings.Contains(addLine, "confidence: high") {
		t.Errorf("add entry line = %q, want a confidence: high comment", addLine)
	}
	if strings.Contains(dropLine, "confidence:") {
		t.Errorf("drop entry line = %q, want no confidence comment", dropLine)
	}
}

// TestToYAML_NoCommentForUnsetConfidence checks that a CapabilityAccess
// built without setting Confidence (the zero value "") doesn't produce a
// nonsensical bare "# confidence: " comment.
func TestToYAML_NoCommentForUnsetConfidence(t *testing.T) {
	capabilities := profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{{Name: "CAP_SYS_NICE"}},
	}
	result := ToProfile(capabilities)

	out, err := ToYAML(result, capabilities)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	if strings.Contains(string(out), "confidence:") {
		t.Errorf("expected no confidence comment for an unset Confidence, got:\n%s", out)
	}
}
