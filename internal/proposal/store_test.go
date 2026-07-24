// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package proposal

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
)

func TestGet_NotFoundReturnsNilNil(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Errorf("Get() = %+v, want nil (no proposal yet)", got)
	}
}

// TestSave_ThenGet_RoundTrips exercises every sub-spec populated at
// once, all built from real Kubernetes/PodLock/seccomp API types
// unchanged elsewhere in this codebase.
func TestSave_ThenGet_RoundTrips(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	localhostProfile := "nginx-demo-seccomp.json"
	spec := Spec{
		Container:   "nginx",
		Binary:      "/usr/sbin/nginx",
		GeneratedAt: "2026-07-24T10:00:00Z",
		HistoryUsed: true,
		PodLock: &podlock.LandlockProfileSpec{
			ProfilesByContainer: map[string]podlock.ProfileByBinary{
				"nginx": {
					"/usr/sbin/nginx": podlock.Profile{ReadOnly: []string{"/etc/nginx"}},
				},
			},
		},
		NetworkPolicy: &networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				Ports: []networkingv1.NetworkPolicyPort{{}},
			}},
		},
		Seccomp: &seccomp.Profile{
			DefaultAction: "SCMP_ACT_ERRNO",
			Architectures: []string{"SCMP_ARCH_X86_64"},
			Syscalls: []seccomp.SyscallRule{
				{Names: []string{"openat", "read"}, Action: "SCMP_ACT_ALLOW"},
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add:  []corev1.Capability{"SETUID"},
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type:             corev1.SeccompProfileTypeLocalhost,
				LocalhostProfile: &localhostProfile,
			},
		},
	}

	if err := Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(got, &spec) {
		t.Errorf("round-tripped spec = %+v, want %+v", got, spec)
	}
}

// TestSave_ThenGet_NilSubSpecsRoundTrip deliberately exercises the
// nil-vs-empty-value reflect.DeepEqual gotcha already hit once building
// internal/history's own round-trip test: a sub-spec left nil (nothing
// observed for that domain this run) must come back nil, not an empty
// non-nil value.
func TestSave_ThenGet_NilSubSpecsRoundTrip(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{
		Container:   "nginx",
		Binary:      "/usr/sbin/nginx",
		GeneratedAt: "2026-07-24T10:00:00Z",
		PodLock: &podlock.LandlockProfileSpec{
			ProfilesByContainer: map[string]podlock.ProfileByBinary{
				"nginx": {"/usr/sbin/nginx": podlock.Profile{ReadOnly: []string{"/etc/nginx"}}},
			},
		},
		// NetworkPolicy/Seccomp/SecurityContext deliberately left nil:
		// no network/syscall/capability activity was observed this run.
	}

	if err := Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.NetworkPolicy != nil {
		t.Errorf("NetworkPolicy = %+v, want nil", got.NetworkPolicy)
	}
	if got.Seccomp != nil {
		t.Errorf("Seccomp = %+v, want nil", got.Seccomp)
	}
	if got.SecurityContext != nil {
		t.Errorf("SecurityContext = %+v, want nil", got.SecurityContext)
	}
	if !reflect.DeepEqual(got, &spec) {
		t.Errorf("round-tripped spec = %+v, want %+v", got, spec)
	}
}

// TestSave_UpdatesExistingProposal checks the Create-vs-Update branch in
// Save: a second Save for the same name must overwrite (a proposal is
// the latest snapshot, not an accumulation — see Save's own doc
// comment), not fail on a missing/stale resourceVersion or create a
// duplicate.
func TestSave_UpdatesExistingProposal(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	first := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(context.Background(), client, "default", "nginx-demo", first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	second := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T11:00:00Z", HistoryUsed: true}
	if err := Save(context.Background(), client, "default", "nginx-demo", second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.GeneratedAt != "2026-07-24T11:00:00Z" || !got.HistoryUsed {
		t.Errorf("got = %+v, want the second Save's values (overwritten, not accumulated)", got)
	}
}
