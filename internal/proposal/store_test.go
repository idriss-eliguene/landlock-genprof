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

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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

const examplePodLockYAML = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  profilesByContainer:
    nginx:
      /usr/sbin/nginx:
        readOnly:
          - /etc/nginx
`

const exampleNetworkPolicyYAML = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: nginx
  ingress:
    - ports:
        - port: 80
`

const exampleSeccompJSON = `{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64"],
  "syscalls": [{"names": ["openat", "read"], "action": "SCMP_ACT_ALLOW"}]
}
`

const examplePatchedManifestYAML = `apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:alpine
      securityContext:
        capabilities:
          add:
            - SETUID
          drop:
            - ALL
        seccompProfile:
          type: Localhost
          localhostProfile: nginx-demo-seccomp.json
`

// TestSave_ThenGet_RoundTrips exercises every field populated at once —
// plain rendered text, exactly what cmd/landlock-genprof/trace.go's
// publishProposal stores (see its own doc comment for why this isn't a
// structured sub-spec).
func TestSave_ThenGet_RoundTrips(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{
		Container:       "nginx",
		Binary:          "/usr/sbin/nginx",
		GeneratedAt:     "2026-07-24T10:00:00Z",
		HistoryUsed:     true,
		PodLock:         examplePodLockYAML,
		NetworkPolicy:   exampleNetworkPolicyYAML,
		Seccomp:         exampleSeccompJSON,
		PatchedManifest: examplePatchedManifestYAML,
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

// TestSave_ThenGet_EmptyFieldsRoundTrip checks that a field left empty
// (nothing observed for that domain this run) round-trips back as an
// empty string, not some non-empty placeholder — the plain-string
// equivalent of the nil-vs-empty-value gotcha already hit once building
// TrainingHistory's own round-trip test.
func TestSave_ThenGet_EmptyFieldsRoundTrip(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{
		Container:   "nginx",
		Binary:      "/usr/sbin/nginx",
		GeneratedAt: "2026-07-24T10:00:00Z",
		PodLock:     examplePodLockYAML,
		// NetworkPolicy/Seccomp/PatchedManifest deliberately left empty:
		// no network/syscall/capability activity was observed this run.
	}

	if err := Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.NetworkPolicy != "" {
		t.Errorf("NetworkPolicy = %q, want empty", got.NetworkPolicy)
	}
	if got.Seccomp != "" {
		t.Errorf("Seccomp = %q, want empty", got.Seccomp)
	}
	if got.PatchedManifest != "" {
		t.Errorf("PatchedManifest = %q, want empty", got.PatchedManifest)
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
