package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/seccompverification"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestVerifySeccompRequiresDedicatedProposalVerifyCapability(t *testing.T) {
	dyn, reads := workbenchReadFixture(t, "work")
	server := &workbenchServer{
		reads: reads, dynamic: dyn, authenticated: true,
		requestIdentity: authn.Identity{Username: "alice"},
		discoverCaps: func(_ context.Context, _ string) (map[authz.Capability]bool, error) {
			return map[authz.Capability]bool{authz.ProposalVerify: false}, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/governance/proposals/example/verify-seccomp", strings.NewReader(`{"expectedResourceVersion":"1","targetPodName":"app","targetPodUID":"pod-uid"}`))
	response := httptest.NewRecorder()
	server.handleGovernanceProposal(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("verify without proposal.verify returned HTTP %d: %s", response.Code, response.Body.String())
	}
}

func TestTargetPodIdentityRevalidationRejectsReplacementAndProfileDrift(t *testing.T) {
	path := "operator/api.json"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-abc", Namespace: "work", UID: types.UID("pod-uid"), OwnerReferences: []metav1.OwnerReference{{UID: types.UID("owner-uid")}}},
		Spec:       corev1.PodSpec{NodeName: "node-a", SecurityContext: &corev1.PodSecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: &path}}, Containers: []corev1.Container{{Name: "app"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true, ContainerID: "containerd://container-a", ImageID: "sha256:image-a"}}},
	}
	spec := &proposal.Spec{Container: "app"}
	if !targetPodIdentityStable(pod, pod.DeepCopy(), "app", "api", spec, "containerd://container-a", "sha256:image-a") {
		t.Fatal("unchanged Pod identity was rejected")
	}
	replacement := pod.DeepCopy()
	replacement.UID = "replacement-uid"
	if targetPodIdentityStable(pod, replacement, "app", "api", spec, "containerd://container-a", "sha256:image-a") {
		t.Fatal("replacement Pod UID was accepted")
	}
	drift := pod.DeepCopy()
	drift.Status.ContainerStatuses[0].ContainerID = "containerd://container-b"
	if targetPodIdentityStable(pod, drift, "app", "api", spec, "containerd://container-a", "sha256:image-a") {
		t.Fatal("replacement container ID was accepted")
	}
	drift = pod.DeepCopy()
	other := "operator/other.json"
	drift.Spec.SecurityContext.SeccompProfile.LocalhostProfile = &other
	if targetPodIdentityStable(pod, drift, "app", "api", spec, "containerd://container-a", "sha256:image-a") {
		t.Fatal("profile drift was accepted")
	}
}

func TestProbeOutputForExitRejectsUnknownExitCodes(t *testing.T) {
	other := int32(137)
	if probeOutputForExit(&other, true) != nil {
		t.Fatal("unexpected process exit became a syscall observation")
	}
	denied := int32(seccompverification.ExitEPERM)
	if probeOutputForExit(&denied, true) == nil || probeOutputForExit(&denied, false) != nil {
		t.Fatal("EPERM exit mapped incorrectly for twin/control")
	}
}
