package seccompverification

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	coretyped "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	defaultRunTimeout = 90 * time.Second
	defaultPollPeriod = 500 * time.Millisecond
)

type PodResult struct {
	Name        string          `json:"name"`
	UID         string          `json:"uid,omitempty"`
	Node        string          `json:"node,omitempty"`
	ImageID     string          `json:"imageID,omitempty"`
	ContainerID string          `json:"containerID,omitempty"`
	Phase       corev1.PodPhase `json:"phase,omitempty"`
	ExitCode    *int32          `json:"exitCode,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

type Experiment struct {
	Twin          PodResult `json:"twin"`
	Control       PodResult `json:"control"`
	VerifierImage string    `json:"verifierImage"`
	Result        Result    `json:"result"`
	Reason        string    `json:"reason,omitempty"`
	StartedAt     time.Time `json:"startedAt"`
	CompletedAt   time.Time `json:"completedAt"`
}

type PodLifecycle interface {
	Create(context.Context, *corev1.Pod, metav1.CreateOptions) (*corev1.Pod, error)
	Get(context.Context, string, metav1.GetOptions) (*corev1.Pod, error)
	Delete(context.Context, string, metav1.DeleteOptions) error
}

type scopedPodLifecycle struct{ pods coretyped.PodInterface }

func (p scopedPodLifecycle) Create(ctx context.Context, pod *corev1.Pod, o metav1.CreateOptions) (*corev1.Pod, error) {
	return p.pods.Create(ctx, pod, o)
}
func (p scopedPodLifecycle) Get(ctx context.Context, name string, o metav1.GetOptions) (*corev1.Pod, error) {
	return p.pods.Get(ctx, name, o)
}
func (p scopedPodLifecycle) Delete(ctx context.Context, name string, o metav1.DeleteOptions) error {
	return p.pods.Delete(ctx, name, o)
}
func NewScopedPodLifecycle(client kubernetes.Interface, namespace string) (PodLifecycle, error) {
	if client == nil || namespace == "" {
		return nil, fmt.Errorf("verifier Pod client requires a namespace")
	}
	return scopedPodLifecycle{pods: client.CoreV1().Pods(namespace)}, nil
}

// Run creates only the two caller-prepared verifier Pods, waits for each fixed
// probe to terminate, captures Kubernetes-native exit identity, then deletes
// only those exact Pod UIDs. Any uncertain stage yields UNKNOWN.
func Run(ctx context.Context, client PodLifecycle, namespace string, twin, control *corev1.Pod, timeout time.Duration) (result Experiment, err error) {
	result.Result = Unknown
	result.StartedAt = time.Now().UTC()
	if client == nil || namespace == "" || twin == nil || control == nil || twin.Namespace != namespace || twin.Namespace != control.Namespace || twin.Name == control.Name {
		result.Reason = "invalid twin/control request"
		return result, fmt.Errorf("invalid twin/control request")
	}
	if err := validatePair(twin, control); err != nil {
		result.Reason = "invalid verifier Pod pair"
		return result, err
	}
	result.VerifierImage = twin.Spec.Containers[0].Image
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// PodLifecycle is scoped at construction to exactly this namespace.
	pods := client
	created := make([]types.UID, 0, 2)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, p := range []*corev1.Pod{twin, control} {
			uid := p.UID
			if uid == "" {
				continue
			}
			if deleteErr := deletePodAndWait(cleanupCtx, pods, p.Name, uid); deleteErr != nil && err == nil {
				err = fmt.Errorf("cleaning up verifier Pod %s: %w", p.Name, deleteErr)
				result.Result = Unknown
				result.Reason = "verifier Pod cleanup uncertain"
			}
		}
		result.CompletedAt = time.Now().UTC()
	}()
	for _, p := range []*corev1.Pod{twin, control} {
		createdPod, createErr := pods.Create(runCtx, p, metav1.CreateOptions{})
		if createErr != nil {
			result.Reason = "verifier Pod creation failed"
			return result, fmt.Errorf("creating verifier Pod: %w", createErr)
		}
		if createdPod.UID == "" {
			result.Reason = "created verifier Pod has no UID"
			return result, fmt.Errorf("created verifier Pod has no UID")
		}
		p.UID = createdPod.UID
		*p = *createdPod
		created = append(created, p.UID)
	}
	if len(created) != 2 {
		result.Reason = "incomplete verifier Pod creation"
		return result, fmt.Errorf("incomplete verifier Pod creation")
	}
	if err = validatePair(twin, control); err != nil {
		result.Reason = "admitted verifier Pod changed security configuration"
		return result, err
	}
	result.Twin, err = waitPod(runCtx, pods, twin.Name, twin.UID)
	if err != nil {
		result.Reason = errorReason(err)
		return result, err
	}
	result.Control, err = waitPod(runCtx, pods, control.Name, control.UID)
	if err != nil {
		result.Reason = errorReason(err)
		return result, err
	}
	if result.Twin.ExitCode == nil || result.Control.ExitCode == nil {
		result.Reason = "probe exit evidence unavailable"
		return result, nil
	}
	if result.Twin.Node != twin.Spec.NodeName || result.Control.Node != twin.Spec.NodeName || result.Twin.ImageID == "" || result.Twin.ImageID != result.Control.ImageID || result.Twin.ContainerID == "" || result.Control.ContainerID == "" || runtimePrefix(result.Twin.ContainerID) == "" || runtimePrefix(result.Twin.ContainerID) != runtimePrefix(result.Control.ContainerID) {
		result.Result = Unknown
		result.Reason = "twin/control runtime identities are incomplete or mismatched"
		return result, nil
	}
	switch {
	case *result.Twin.ExitCode == ExitEPERM && *result.Control.ExitCode == ExitSuccess:
		result.Result = Verified
	case *result.Twin.ExitCode == ExitSuccess && *result.Control.ExitCode == ExitSuccess:
		result.Result = NotVerified
		result.Reason = "twin probe succeeded instead of returning EPERM"
	case *result.Twin.ExitCode == ExitInconclusive:
		result.Result = Unknown
		result.Reason = "twin probe was inconclusive"
	case *result.Control.ExitCode != ExitSuccess:
		result.Result = Unknown
		result.Reason = "RuntimeDefault control failed or was inconclusive"
	default:
		result.Result = Unknown
		result.Reason = "probe exit status was not recognized"
	}
	return result, nil
}

func deletePodAndWait(ctx context.Context, pods PodLifecycle, name string, uid types.UID) error {
	policy := metav1.DeletePropagationForeground
	if err := pods.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}, PropagationPolicy: &policy}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	ticker := time.NewTicker(defaultPollPeriod)
	defer ticker.Stop()
	for {
		pod, err := pods.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if pod.UID != uid {
			return fmt.Errorf("Pod name was reused; refusing to affect replacement UID %s", pod.UID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func runtimePrefix(containerID string) string {
	prefix, _, ok := strings.Cut(containerID, "://")
	if !ok {
		return ""
	}
	return prefix
}

func validatePair(twin, control *corev1.Pod) error {
	if twin.Spec.NodeName == "" || twin.Spec.NodeName != control.Spec.NodeName || !sameStringPtr(twin.Spec.RuntimeClassName, control.Spec.RuntimeClassName) {
		return fmt.Errorf("twin and control must use the same node and runtime class")
	}
	if len(twin.Spec.Containers) != 1 || len(control.Spec.Containers) != 1 || twin.Spec.SecurityContext == nil || control.Spec.SecurityContext == nil {
		return fmt.Errorf("verifier Pod shape is invalid")
	}
	if err := ValidateProbeImage(twin.Spec.Containers[0].Image); err != nil {
		return err
	}
	if twin.Spec.Containers[0].Image != control.Spec.Containers[0].Image {
		return fmt.Errorf("twin and control probe images differ")
	}
	if twin.Labels[VerifierLabel] != "twin" || control.Labels[VerifierLabel] != "control" {
		return fmt.Errorf("verifier Pod roles are invalid")
	}
	for _, pod := range []*corev1.Pod{twin, control} {
		if pod.Spec.RestartPolicy != corev1.RestartPolicyNever || pod.Spec.ServiceAccountName != "seccomp-verifier" || pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken || len(pod.Spec.Volumes) != 0 || pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC || len(pod.Spec.Containers) != 1 || pod.Spec.SecurityContext.RunAsNonRoot == nil || !*pod.Spec.SecurityContext.RunAsNonRoot || pod.Spec.SecurityContext.RunAsUser == nil || *pod.Spec.SecurityContext.RunAsUser != 65532 {
			return fmt.Errorf("verifier Pod violates isolation constraints")
		}
		container := pod.Spec.Containers[0]
		if container.Name != ProbeContainerName || len(container.Command) != 1 || container.Command[0] != "/seccomp-verifier-probe" || len(container.Args) != 0 || container.ImagePullPolicy != corev1.PullIfNotPresent || container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation || container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem || container.SecurityContext.Capabilities == nil || len(container.SecurityContext.Capabilities.Add) != 0 || len(container.SecurityContext.Capabilities.Drop) != 1 || container.SecurityContext.Capabilities.Drop[0] != "ALL" {
			return fmt.Errorf("probe container violates fixed command or security constraints")
		}
	}
	twinProfile, controlProfile := twin.Spec.SecurityContext.SeccompProfile, control.Spec.SecurityContext.SeccompProfile
	if twinProfile == nil || twinProfile.Type != corev1.SeccompProfileTypeLocalhost || twinProfile.LocalhostProfile == nil || *twinProfile.LocalhostProfile == "" || controlProfile == nil || controlProfile.Type != corev1.SeccompProfileTypeRuntimeDefault || controlProfile.LocalhostProfile != nil {
		return fmt.Errorf("twin/control seccomp profiles are invalid")
	}
	return nil
}

func sameStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func waitPod(ctx context.Context, client interface {
	Get(context.Context, string, metav1.GetOptions) (*corev1.Pod, error)
}, name string, expectedUID types.UID) (PodResult, error) {
	ticker := time.NewTicker(defaultPollPeriod)
	defer ticker.Stop()
	for {
		pod, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return PodResult{Name: name}, fmt.Errorf("reading verifier Pod: %w", err)
		}
		if pod.UID != expectedUID {
			return PodResult{Name: name}, fmt.Errorf("verifier Pod identity changed")
		}
		out := PodResult{Name: pod.Name, UID: string(pod.UID), Node: pod.Spec.NodeName, Phase: pod.Status.Phase}
		for _, s := range pod.Status.ContainerStatuses {
			if s.Name == ProbeContainerName {
				out.ImageID = s.ImageID
				out.ContainerID = s.ContainerID
				if s.State.Terminated != nil {
					code := s.State.Terminated.ExitCode
					out.ExitCode = &code
					out.Reason = s.State.Terminated.Reason
				}
			}
		}
		if out.ExitCode != nil {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, fmt.Errorf("waiting for verifier Pod %s: %w", name, ctx.Err())
		case <-ticker.C:
		}
	}
}

func errorReason(err error) string {
	if err != nil {
		return err.Error()
	}
	return "unknown"
}
