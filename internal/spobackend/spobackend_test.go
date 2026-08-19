// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package spobackend

import (
	"strings"
	"testing"
)

// The governed name is normative identity semantics (docs/adr/0008), so
// these are golden-value assertions on purpose: a change here is a
// NameScheme bump, not a refactor, and this test is what makes an
// accidental change fail loudly.
func TestGovernedProfileName_IsFrozen(t *testing.T) {
	got := GovernedProfileName("default", "nginx-demo", "nginx")
	if !strings.HasPrefix(got, "lg-v1-nginx-demo-") {
		t.Errorf("GovernedProfileName() = %q, want the lg-v1-<pod>- prefix", got)
	}
	if len(got) != len("lg-v1-nginx-demo-")+16 {
		t.Errorf("GovernedProfileName() = %q (len %d), want a 16-character hex suffix", got, len(got))
	}
}

func TestGovernedProfileName_Deterministic(t *testing.T) {
	a := GovernedProfileName("default", "nginx-demo", "nginx")
	b := GovernedProfileName("default", "nginx-demo", "nginx")
	if a != b {
		t.Errorf("GovernedProfileName() = %q then %q, want determinism", a, b)
	}
}

// Every component is authority-relevant: two workloads differing in any
// one of them must not share a cluster-scoped object.
func TestGovernedProfileName_AllComponentsMatter(t *testing.T) {
	base := GovernedProfileName("default", "nginx-demo", "nginx")
	for _, tc := range []struct {
		name         string
		ns, pod, ctr string
	}{
		{"namespace", "staging", "nginx-demo", "nginx"},
		{"pod", "default", "nginx-other", "nginx"},
		{"container", "default", "nginx-demo", "tools"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := GovernedProfileName(tc.ns, tc.pod, tc.ctr); got == base {
				t.Errorf("changing the %s produced the same name %q", tc.name, got)
			}
		})
	}
}

// The encoding is length-prefixed rather than delimiter-joined precisely
// so these pairs cannot collide. "<namespace>-<pod>" would map both to
// "a-b-c", which in a cluster-wide name space is a cross-namespace
// authority collision.
func TestGovernedProfileName_InjectiveAcrossBoundaries(t *testing.T) {
	pairs := [][2][3]string{
		{{"a-b", "c", "x"}, {"a", "b-c", "x"}},
		{{"a", "b", "c-x"}, {"a", "b-c", "x"}},
		{{"ns", "pod", ""}, {"ns", "pod", ""}},
	}
	for _, pair := range pairs {
		left := GovernedProfileName(pair[0][0], pair[0][1], pair[0][2])
		right := GovernedProfileName(pair[1][0], pair[1][1], pair[1][2])
		same := pair[0] == pair[1]
		if (left == right) != same {
			t.Errorf("GovernedProfileName%v = %q and %v = %q; equal=%v, want %v",
				pair[0], left, pair[1], right, left == right, same)
		}
	}
}

// Kubernetes object names must be DNS-1123 subdomains; keeping inside the
// 63-character label limit also keeps the name usable anywhere a label
// value is expected.
func TestGovernedProfileName_LengthAndCharset(t *testing.T) {
	long := strings.Repeat("very-long-pod-name-segment", 20)
	for _, pod := range []string{"nginx-demo", long, "UPPER_Case.Pod", "", "---", "…unicode…"} {
		got := GovernedProfileName("some-namespace", pod, "container")
		if len(got) > 63 {
			t.Errorf("GovernedProfileName(pod=%q) = %q (len %d), want <= 63", pod, got, len(got))
		}
		if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Errorf("GovernedProfileName(pod=%q) = %q, want no leading/trailing dash", pod, got)
		}
		for _, r := range got {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
				t.Errorf("GovernedProfileName(pod=%q) = %q contains %q, want [a-z0-9-] only", pod, got, r)
			}
		}
	}
}

func TestLocalhostProfilePath_RoundTrips(t *testing.T) {
	name := GovernedProfileName("default", "nginx-demo", "nginx")
	path := LocalhostProfilePath(name)

	if want := "operator/" + name + ".json"; path != want {
		t.Errorf("LocalhostProfilePath() = %q, want %q", path, want)
	}
	got, ok := ParseLocalhostProfilePath(path)
	if !ok || got != name {
		t.Errorf("ParseLocalhostProfilePath(%q) = (%q, %v), want (%q, true)", path, got, ok, name)
	}
}

// Malformed and obsolete paths must be rejected rather than reinterpreted.
// The namespaced form in particular belonged to SPO v0.8.4; silently
// accepting it would bind a workload to a profile whose readiness was
// never established.
func TestParseLocalhostProfilePath_RejectsMalformed(t *testing.T) {
	for _, path := range []string{
		"operator/default/nginx-demo.json", // obsolete namespaced form
		"operator/.json",
		"operator/nginx-demo",
		"nginx-demo.json",
		"runtime/default.json",
		"",
		"operator/",
		"operator/a/b/c.json",
	} {
		if name, ok := ParseLocalhostProfilePath(path); ok {
			t.Errorf("ParseLocalhostProfilePath(%q) = (%q, true), want rejection", path, name)
		}
	}
}

func TestClassifyOwnership(t *testing.T) {
	owned := OwnershipAnnotations("default", "nginx-demo", "nginx")

	if got := ClassifyOwnership(owned, "default", "nginx-demo", "nginx"); got != OwnedSameTarget {
		t.Errorf("ClassifyOwnership(ours, same target) = %v, want OwnedSameTarget", got)
	}
	if got := ClassifyOwnership(nil, "default", "nginx-demo", "nginx"); got != NotOwned {
		t.Errorf("ClassifyOwnership(no annotations) = %v, want NotOwned", got)
	}
	if got := ClassifyOwnership(map[string]string{ManagedByAnnotation: "someone-else"},
		"default", "nginx-demo", "nginx"); got != NotOwned {
		t.Errorf("ClassifyOwnership(foreign marker) = %v, want NotOwned", got)
	}

	// The collision case the identity tuple exists to catch: ours, same
	// name, different workload.
	for _, tc := range []struct{ name, ns, pod, ctr string }{
		{"different namespace", "staging", "nginx-demo", "nginx"},
		{"different pod", "default", "other", "nginx"},
		{"different container", "default", "nginx-demo", "tools"},
	} {
		if got := ClassifyOwnership(owned, tc.ns, tc.pod, tc.ctr); got != OwnedDifferentTarget {
			t.Errorf("ClassifyOwnership(ours, %s) = %v, want OwnedDifferentTarget", tc.name, got)
		}
	}

	// A future name scheme must not be treated as this one.
	stale := OwnershipAnnotations("default", "nginx-demo", "nginx")
	stale[NameSchemeAnnotation] = "v2"
	if got := ClassifyOwnership(stale, "default", "nginx-demo", "nginx"); got != OwnedDifferentTarget {
		t.Errorf("ClassifyOwnership(other name scheme) = %v, want OwnedDifferentTarget", got)
	}
}

func TestSPOAPIContract(t *testing.T) {
	if APIVersion != "security-profiles-operator.x-k8s.io/v1" {
		t.Errorf("APIVersion = %q, want the v1 API", APIVersion)
	}
	if !SeccompProfileClusterScoped() {
		t.Error("SeccompProfileClusterScoped() = false; SeccompProfile is cluster-scoped from SPO v0.9.0 on")
	}
	if ProfileRecordingClusterScoped() {
		t.Error("ProfileRecordingClusterScoped() = true; ProfileRecording remained namespaced")
	}
	if gvr := SeccompProfileGVR(); gvr.Resource != "seccompprofiles" || gvr.Version != "v1" {
		t.Errorf("SeccompProfileGVR() = %v, want v1 seccompprofiles", gvr)
	}
}
