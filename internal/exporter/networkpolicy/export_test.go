// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package networkpolicy

import (
	"reflect"
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// mockNginxNetworkProfile mirrors the shape internal/policy.Synthesize
// would produce for a typical nginx training run that both dials an
// upstream on 443 (egress) and listens on 8080 (ingress). Built directly
// as an IR fixture, not derived by calling Synthesize — this package
// tests the IR -> NetworkPolicy conversion in isolation, the same way
// internal/exporter/podlock's tests do for the filesystem half.
func mockNginxNetworkProfile() profile.NetworkProfile {
	return profile.NetworkProfile{
		Accesses: []profile.NetworkAccess{
			{Port: 443, Direction: profile.DirectionEgress, Confidence: profile.ConfidenceHigh, SeenCount: 5},
			{Port: 8080, Direction: profile.DirectionIngress, Confidence: profile.ConfidenceHigh, SeenCount: 5},
		},
	}
}

func TestToPolicy_MockNginxNetworkProfile(t *testing.T) {
	meta := PolicyMeta{
		Name:      "nginx-demo",
		Namespace: "default",
		PodLabels: map[string]string{"app": "nginx"},
	}
	result := ToPolicy(meta, mockNginxNetworkProfile())

	if result.APIVersion != "networking.k8s.io/v1" {
		t.Errorf("APIVersion = %q, want networking.k8s.io/v1", result.APIVersion)
	}
	if result.Kind != "NetworkPolicy" {
		t.Errorf("Kind = %q, want NetworkPolicy", result.Kind)
	}
	if result.Name != "nginx-demo" || result.Namespace != "default" {
		t.Errorf("ObjectMeta = {%q %q}, want {nginx-demo default}", result.Name, result.Namespace)
	}
	if !reflect.DeepEqual(result.Spec.PodSelector.MatchLabels, map[string]string{"app": "nginx"}) {
		t.Errorf("PodSelector.MatchLabels = %v, want {app: nginx}", result.Spec.PodSelector.MatchLabels)
	}

	wantTypes := []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
	if !reflect.DeepEqual(result.Spec.PolicyTypes, wantTypes) {
		t.Errorf("PolicyTypes = %v, want %v", result.Spec.PolicyTypes, wantTypes)
	}

	if len(result.Spec.Ingress) != 1 || len(result.Spec.Ingress[0].Ports) != 1 {
		t.Fatalf("Ingress = %+v, want one rule with one port", result.Spec.Ingress)
	}
	if got := result.Spec.Ingress[0].Ports[0].Port.IntValue(); got != 8080 {
		t.Errorf("Ingress port = %d, want 8080", got)
	}
	if result.Spec.Ingress[0].From != nil {
		t.Errorf("Ingress[0].From = %v, want nil (no peer restriction)", result.Spec.Ingress[0].From)
	}

	if len(result.Spec.Egress) != 1 || len(result.Spec.Egress[0].Ports) != 1 {
		t.Fatalf("Egress = %+v, want one rule with one port", result.Spec.Egress)
	}
	if got := result.Spec.Egress[0].Ports[0].Port.IntValue(); got != 443 {
		t.Errorf("Egress port = %d, want 443", got)
	}
	if result.Spec.Egress[0].To != nil {
		t.Errorf("Egress[0].To = %v, want nil (no peer restriction)", result.Spec.Egress[0].To)
	}
}

// TestToPolicy_OnlyPopulatesObservedDirections checks that a NetworkProfile
// with only egress accesses produces a policy with PolicyTypes: [Egress]
// and no Ingress rule — an empty Ingress rule set (as opposed to an absent
// one) means "deny all ingress" per the NetworkPolicy spec, which must
// never be asserted just because no ingress activity happened to be
// observed during a training run.
func TestToPolicy_OnlyPopulatesObservedDirections(t *testing.T) {
	net := profile.NetworkProfile{
		Accesses: []profile.NetworkAccess{
			{Port: 443, Direction: profile.DirectionEgress},
		},
	}

	result := ToPolicy(PolicyMeta{Name: "egress-only", Namespace: "default"}, net)

	if len(result.Spec.Ingress) != 0 {
		t.Errorf("Ingress = %+v, want empty", result.Spec.Ingress)
	}
	wantTypes := []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}
	if !reflect.DeepEqual(result.Spec.PolicyTypes, wantTypes) {
		t.Errorf("PolicyTypes = %v, want %v", result.Spec.PolicyTypes, wantTypes)
	}
}

func TestToPolicy_EmptyNetworkProfile(t *testing.T) {
	result := ToPolicy(PolicyMeta{Name: "empty", Namespace: "default"}, profile.NetworkProfile{})

	if len(result.Spec.PolicyTypes) != 0 {
		t.Errorf("PolicyTypes = %v, want empty (no observed direction)", result.Spec.PolicyTypes)
	}
	if len(result.Spec.Ingress) != 0 || len(result.Spec.Egress) != 0 {
		t.Errorf("Ingress/Egress = %+v/%+v, want both empty", result.Spec.Ingress, result.Spec.Egress)
	}
}

func TestToYAML_RoundTrips(t *testing.T) {
	net := mockNginxNetworkProfile()
	result := ToPolicy(PolicyMeta{
		Name:      "nginx-demo",
		Namespace: "default",
		PodLabels: map[string]string{"app": "nginx"},
	}, net)

	out, err := ToYAML(result, net)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	// Keys must be camelCase (apiVersion, podSelector, ...), not the Go
	// field name — that's the guarantee that sigs.k8s.io/yaml reads `json`
	// tags, matching internal/exporter/podlock.ToYAML's own test.
	text := string(out)
	for _, want := range []string{"apiVersion:", "podSelector:", "policyTypes:", "ingress:", "egress:"} {
		if !strings.Contains(text, want) {
			t.Errorf("YAML output missing expected key %q:\n%s", want, text)
		}
	}

	var roundTripped networkingv1.NetworkPolicy
	if err := yaml.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&roundTripped, result) {
		t.Errorf("round-tripped policy = %+v, want %+v", roundTripped, *result)
	}
}

// TestToYAML_AnnotatesConfidenceByDirection checks the actual point of
// ToYAML's second parameter, and specifically that the same port number
// under different directions gets its own, independently correct
// comment — proving the walk tracks ingress/egress context, not just
// port value.
func TestToYAML_AnnotatesConfidenceByDirection(t *testing.T) {
	net := profile.NetworkProfile{
		Accesses: []profile.NetworkAccess{
			{Port: 443, Direction: profile.DirectionEgress, Confidence: profile.ConfidenceHigh},
			{Port: 443, Direction: profile.DirectionIngress, Confidence: profile.ConfidenceLow},
		},
	}
	result := ToPolicy(PolicyMeta{Name: "nginx-demo", Namespace: "default"}, net)

	out, err := ToYAML(result, net)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	lines := strings.Split(string(out), "\n")
	var ingressLine, egressLine string
	for i, line := range lines {
		if strings.Contains(line, "ingress:") {
			ingressLine = findPortLine(lines[i:])
		}
		if strings.Contains(line, "egress:") {
			egressLine = findPortLine(lines[i:])
		}
	}
	if !strings.Contains(ingressLine, "confidence: low") {
		t.Errorf("ingress port 443 line = %q, want a confidence: low comment", ingressLine)
	}
	if !strings.Contains(egressLine, "confidence: high") {
		t.Errorf("egress port 443 line = %q, want a confidence: high comment", egressLine)
	}
}

// findPortLine returns the first "port:" line in lines, for
// TestToYAML_AnnotatesConfidenceByDirection's per-section lookup.
func findPortLine(lines []string) string {
	for _, line := range lines {
		if strings.Contains(line, "port:") {
			return line
		}
	}
	return ""
}

// TestToYAML_NoCommentForUnsetConfidence checks that a NetworkAccess
// built without setting Confidence (the zero value "") doesn't produce
// a nonsensical bare "# confidence: " comment.
func TestToYAML_NoCommentForUnsetConfidence(t *testing.T) {
	net := profile.NetworkProfile{
		Accesses: []profile.NetworkAccess{{Port: 9090, Direction: profile.DirectionEgress}},
	}
	result := ToPolicy(PolicyMeta{Name: "app-demo", Namespace: "default"}, net)

	out, err := ToYAML(result, net)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	if strings.Contains(string(out), "confidence:") {
		t.Errorf("expected no confidence comment for an unset Confidence, got:\n%s", out)
	}
}
