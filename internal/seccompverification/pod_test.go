package seccompverification

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

const testProbeImage = "registry.example.test/seccomp-verifier@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestBuildPodTwinAndControlAreIsolated(t *testing.T) {
	for _, tc := range []struct {
		name     string
		control  bool
		wantType corev1.SeccompProfileType
	}{
		{"twin", false, corev1.SeccompProfileTypeLocalhost}, {"control", true, corev1.SeccompProfileTypeRuntimeDefault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profilePath := "operator/api.json"
			pod, err := BuildPod("verifier", tc.name, testProbeImage, "node-a", "runc", profilePath, tc.control)
			if err != nil {
				t.Fatal(err)
			}
			if pod.Spec.NodeName != "node-a" || pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "runc" || pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
				t.Fatalf("pod scheduling/lifecycle: %+v", pod.Spec)
			}
			if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
				t.Fatal("service account token may be mounted")
			}
			if pod.Spec.ServiceAccountName != "seccomp-verifier" || len(pod.Spec.Volumes) != 0 || len(pod.Spec.Containers) != 1 {
				t.Fatal("pod includes unapproved identity or volumes")
			}
			if pod.Spec.SecurityContext.SeccompProfile.Type != tc.wantType {
				t.Fatalf("seccomp type=%s want %s", pod.Spec.SecurityContext.SeccompProfile.Type, tc.wantType)
			}
			if tc.control && pod.Spec.SecurityContext.SeccompProfile.LocalhostProfile != nil {
				t.Fatal("control inherits a Localhost profile")
			}
			if !tc.control && (pod.Spec.SecurityContext.SeccompProfile.LocalhostProfile == nil || *pod.Spec.SecurityContext.SeccompProfile.LocalhostProfile != profilePath) {
				t.Fatal("twin profile path mismatch")
			}
			c := pod.Spec.Containers[0]
			if c.Image != testProbeImage || len(c.Command) != 1 || c.Command[0] != "/seccomp-verifier-probe" || c.SecurityContext == nil || !*c.SecurityContext.ReadOnlyRootFilesystem || *c.SecurityContext.AllowPrivilegeEscalation || len(c.SecurityContext.Capabilities.Drop) != 1 || c.SecurityContext.Capabilities.Drop[0] != "ALL" {
				t.Fatalf("unsafe probe container: %+v", c)
			}
		})
	}
}

func TestBuildPodRequiresDigestImageAndProfile(t *testing.T) {
	if _, err := BuildPod("verifier", "twin", "repo/probe:latest", "node", "", "profile", false); err == nil {
		t.Fatal("accepted mutable image")
	}
	if _, err := BuildPod("verifier", "twin", testProbeImage, "node", "", "", false); err == nil {
		t.Fatal("accepted empty profile")
	}
}
