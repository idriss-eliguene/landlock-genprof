package seccompverification

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ProbeContainerName = "seccomp-probe"
	VerifierLabel      = "landlockgenprof.io/seccomp-verifier"
	ExitSuccess        = 0
	ExitEPERM          = 10
	ExitInconclusive   = 20
)

func ValidateProbeImage(image string) error {
	if !strings.Contains(image, "@sha256:") {
		return fmt.Errorf("probe image must be digest pinned")
	}
	parts := strings.Split(image, "@sha256:")
	if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 {
		return fmt.Errorf("probe image digest is malformed")
	}
	for _, c := range parts[1] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return fmt.Errorf("probe image digest is malformed")
		}
	}
	return nil
}

func BuildPod(namespace, name, image, node, runtimeClass, profileName string, control bool) (*corev1.Pod, error) {
	if errs := validation.IsDNS1123Label(name); len(errs) != 0 {
		return nil, fmt.Errorf("invalid verifier Pod name: %s", strings.Join(errs, ", "))
	}
	if namespace == "" || node == "" {
		return nil, fmt.Errorf("verifier Pod requires namespace and target node")
	}
	if err := ValidateProbeImage(image); err != nil {
		return nil, err
	}
	if !control && profileName == "" {
		return nil, fmt.Errorf("twin Pod requires an exact Localhost profile name")
	}
	profile := &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: &profileName}
	if control {
		profile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	}
	labels := map[string]string{VerifierLabel: "twin"}
	if control {
		labels[VerifierLabel] = "control"
	}
	var runtimeName *string
	if runtimeClass != "" {
		runtimeName = &runtimeClass
	}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}, Spec: corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever, NodeName: node, RuntimeClassName: runtimeName,
		ServiceAccountName: "seccomp-verifier", AutomountServiceAccountToken: func() *bool { v := false; return &v }(),
		SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true), RunAsUser: int64Ptr(65532), SeccompProfile: profile},
		Containers:      []corev1.Container{{Name: ProbeContainerName, Image: image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/seccomp-verifier-probe"}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: boolPtr(false), ReadOnlyRootFilesystem: boolPtr(true), RunAsNonRoot: boolPtr(true), RunAsUser: int64Ptr(65532), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}},
	}}, nil
}

func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }
