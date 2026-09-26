package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/seccompverification"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/workload"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

type seccompVerificationRequest struct {
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
	TargetPodName           string `json:"targetPodName"`
	TargetPodUID            string `json:"targetPodUID"`
}

func (s *workbenchServer) handleSeccompVerification(w http.ResponseWriter, r *http.Request, name string) {
	if !s.authenticated || s.dynamic == nil || s.requestIdentity.Username == "" {
		writeGovernanceError(w, 401, "AUTHENTICATION_REQUIRED", "authenticated verification is required")
		return
	}
	if !s.capabilityAllowed(r.Context(), authz.ProposalVerify) {
		writeGovernanceError(w, 403, "AUTHORIZATION_DENIED", "the identity lacks proposal.verify")
		return
	}
	if s.verifierPods == nil || s.verifierNamespace == "" || s.verifierImage == "" {
		writeGovernanceError(w, 503, "VERIFIER_UNAVAILABLE", "dedicated verifier identity is not configured")
		return
	}
	if s.clusterIdentity == "" || s.clusterIdentity != s.verifierClusterIdentity {
		writeGovernanceError(w, 503, "VERIFIER_CLUSTER_MISMATCH", "verifier identity targets a different Kubernetes cluster")
		return
	}
	var request seccompVerificationRequest
	if err := decodeJSON(w, r, &request); err != nil || request.ExpectedResourceVersion == "" || request.TargetPodName == "" || request.TargetPodUID == "" {
		writeGovernanceError(w, 400, "INVALID_REQUEST", "resourceVersion, targetPodName, and targetPodUID are required")
		return
	}
	namespace := s.reads.SessionIdentity().Namespace
	spec, proposalUID, rv, err := proposal.GetWithIdentityAndResourceVersion(r.Context(), s.dynamic, namespace, name)
	if err != nil {
		writeGovernanceApplicationError(w, err)
		return
	}
	status, statusRV, err := proposal.GetStatusWithResourceVersion(r.Context(), s.dynamic, namespace, name)
	if err != nil {
		writeGovernanceApplicationError(w, err)
		return
	}
	if rv != request.ExpectedResourceVersion || statusRV != request.ExpectedResourceVersion {
		writeGovernanceError(w, 409, "CONFLICT", "proposal changed before verification")
		return
	}
	if status == nil || proposal.ValidateApprovedCandidate(spec, status) != nil {
		writeGovernanceError(w, 409, "APPROVAL_REQUIRED", "verification requires the currently approved candidate")
		return
	}
	candidateDigest, err := candidateDigestForVerification(spec)
	if err != nil || candidateDigest != status.ApprovedCandidateDigest {
		writeGovernanceError(w, 409, "APPROVAL_DRIFT", "approved candidate digest does not match current proposal")
		return
	}
	if spec.SPOSeccompProfile == "" {
		writeGovernanceError(w, 422, "UNSUPPORTED_BACKEND", "proposal has no SPO SeccompProfile artifact")
		return
	}
	profileName, desiredSpec, err := desiredSPOProfile(spec.SPOSeccompProfile)
	if err != nil {
		writeGovernanceError(w, 422, "INVALID_PROFILE", err.Error())
		return
	}
	target, err := s.reads.GetPod(r.Context(), request.TargetPodName)
	if err != nil || string(target.UID) != request.TargetPodUID {
		writeGovernanceError(w, 409, "TARGET_DRIFT", "selected target Pod identity is unavailable or changed")
		return
	}
	containerName := spec.Container
	if spec.Subject != nil && spec.Subject.Container != "" {
		containerName = spec.Subject.Container
	}
	workloadUID, containerID, imageID, err := validateTargetPod(target, containerName, profileName, spec)
	if err != nil {
		writeGovernanceError(w, 409, "TARGET_NOT_ELIGIBLE", err.Error())
		return
	}
	logicalWorkload, err := resolveTargetWorkload(r.Context(), s.discovery, target, spec)
	if err != nil {
		writeGovernanceError(w, 409, "TARGET_NOT_ELIGIBLE", err.Error())
		return
	}
	workloadUID = logicalWorkload.UID
	profile, profileDigest, err := readExactSPOProfile(r.Context(), s.profileDynamic, profileName, desiredSpec)
	if err != nil {
		writeGovernanceError(w, 409, "PROFILE_NOT_MATERIALIZED", err.Error())
		return
	}
	profilePath := spobackend.LocalhostProfilePath(profileName)
	if s.verificationSema == nil {
		writeGovernanceError(w, 503, "VERIFIER_BUSY", "bounded verification is not available in this server context")
		return
	}
	opCtx, cancel := context.WithTimeout(r.Context(), workbenchVerificationTimeout)
	defer cancel()
	select {
	case s.verificationSema <- struct{}{}:
		defer func() { <-s.verificationSema }()
	case <-opCtx.Done():
		writeGovernanceError(w, 504, "VERIFIER_TIMEOUT", "verification request expired while waiting for the verifier")
		return
	}
	attempt, err := seccompverification.NewAttemptID()
	if err != nil {
		writeGovernanceError(w, 500, "VERIFIER_ERROR", "could not allocate verification attempt")
		return
	}
	twinName, controlName := "seccomp-twin-"+attempt[:10], "seccomp-control-"+attempt[:10]
	twin, err := seccompverification.BuildPod(s.verifierNamespace, twinName, s.verifierImage, target.Spec.NodeName, podRuntimeClass(target), profilePath, false)
	if err != nil {
		writeGovernanceError(w, 422, "VERIFIER_CONFIGURATION", err.Error())
		return
	}
	control, err := seccompverification.BuildPod(s.verifierNamespace, controlName, s.verifierImage, target.Spec.NodeName, podRuntimeClass(target), profilePath, true)
	if err != nil {
		writeGovernanceError(w, 422, "VERIFIER_CONFIGURATION", err.Error())
		return
	}
	started := time.Now().UTC()
	experiment, runErr := seccompverification.Run(opCtx, s.verifierPods, s.verifierNamespace, twin, control, 90*time.Second)
	result := experiment.Result
	reason := experiment.Reason
	// Re-read every authority-bearing input after the run. A transient or
	// incomplete read is UNKNOWN and cannot overwrite materialization/activation.
	currentTarget, targetErr := s.reads.GetPod(opCtx, request.TargetPodName)
	currentSpec, currentUID, currentRV, specErr := proposal.GetWithIdentityAndResourceVersion(opCtx, s.dynamic, namespace, name)
	currentStatus, statusRV2, statusErr := proposal.GetStatusWithResourceVersion(opCtx, s.dynamic, namespace, name)
	currentProfile, currentProfileDigest, profileErr := readExactSPOProfile(opCtx, s.profileDynamic, profileName, desiredSpec)
	currentWorkload, workloadErr := resolveTargetWorkload(opCtx, s.discovery, currentTarget, spec)
	currentWorkloadUID, currentContainerID, currentImageID, currentTargetErr := validateTargetPod(currentTarget, containerName, profileName, spec)
	if runErr != nil || targetErr != nil || currentTargetErr != nil || specErr != nil || statusErr != nil || profileErr != nil || workloadErr != nil || currentUID != proposalUID || currentRV != rv || statusRV2 != rv || !targetPodIdentityStable(target, currentTarget, containerName, profileName, spec, containerID, imageID) || currentContainerID == "" || currentImageID == "" || currentWorkloadUID == "" || currentWorkload.UID != workloadUID || candidateDigestForVerificationMust(currentSpec) != candidateDigest || currentStatus == nil || currentStatus.ApprovedCandidateDigest != candidateDigest || currentStatus.ApprovalState != proposal.ApprovalApproved || currentProfileDigest != profileDigest || profile == nil || currentProfile == nil || profile.GetUID() == "" || currentProfile.GetUID() != profile.GetUID() {
		result = seccompverification.Unknown
		reason = "identity, approval, or profile changed or became unavailable during experiment"
	}
	if experiment.CompletedAt.IsZero() {
		experiment.CompletedAt = time.Now().UTC()
	}
	targetID := seccompverification.Identity{ProposalUID: proposalUID, CandidateDigest: candidateDigest, Namespace: namespace, ProfileName: profileName, ProfileUID: string(profile.GetUID()), ProfileDigest: profileDigest, WorkloadUID: workloadUID, Workload: targetWorkloadName(spec), PodName: target.Name, PodUID: string(target.UID), Container: containerName, ContainerID: containerID, ImageID: imageID, Node: target.Spec.NodeName, Runtime: runtimeFromContainerID(containerID), SeccompMode: "Localhost"}
	twinID := verifierIdentity(targetID, experiment.Twin, "Localhost", s.verifierNamespace, profileDigest)
	controlID := verifierIdentity(targetID, experiment.Control, "RuntimeDefault", s.verifierNamespace, profileDigest)
	targetOutput, controlOutput := probeOutputForExit(experiment.Twin.ExitCode, true), probeOutputForExit(experiment.Control.ExitCode, false)
	fact := seccompverification.Evaluate(targetOutput, controlOutput, twinID, controlID, started)
	fact.AttemptID = attempt
	fact.RequestedBy = s.requestIdentity.Username
	fact.VerifierIdentity = "system:serviceaccount:" + s.verifierNamespace + ":" + seccompverification.VerifierAPIServiceAccount
	fact.Target = targetID
	fact.Materialization = seccompverification.ProfileMaterialization{State: "MATERIALIZED", Source: "SPO.status.localhostProfile", ProfileUID: string(profile.GetUID()), ContentDigest: profileDigest, LocalhostPath: profilePath, Node: target.Spec.NodeName, ObservedAt: started, Limitation: "SPO status reports the path; the node-local file content digest was not independently measured"}
	fact.TargetConfiguration = seccompverification.TargetConfiguration{State: "CONFIGURED_NOT_RUNTIME_VERIFIED", PodUID: string(target.UID), ContainerID: containerID, Node: target.Spec.NodeName, LocalhostPath: profilePath, ObservedAt: started, Limitation: "Pod declaration and running container are recorded; this does not establish the application's effective syscall filter"}
	fact.Experiment = experiment
	fact.VerifierImage = s.verifierImage
	fact.Result = result
	fact.Reason = reason
	fact.Revocation = "UNKNOWN"
	if result == seccompverification.Unknown {
		if fact.Reason == "" {
			fact.Reason = "probe outcome or post-run evidence unavailable"
		}
	}
	fact.ValidUntil = started.Add(5 * time.Minute)
	// A probe/API timeout still produces a durable UNKNOWN when proposal
	// authority is readable and unchanged. The operation context may already
	// have expired, so persistence receives a short independent bound; the
	// authority/resourceVersion checks still reject stale evidence.
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer persistCancel()
	if err := proposal.AppendSeccompVerification(persistCtx, s.dynamic, namespace, name, rv, fact); err != nil {
		writeGovernanceError(w, 409, "PERSISTENCE_CONFLICT", "verification evidence was not persisted because proposal authority changed")
		return
	}
	writeGovernanceJSON(w, governanceResponse{Operation: "verify-seccomp", Namespace: namespace, Resource: "SecurityProfileProposal/" + name, ResultingState: string(fact.Result), Actor: s.requestIdentity.Username, Success: true, Message: "bounded twin-Pod experiment recorded; this is not evidence of live workload activation"})
}

func targetPodIdentityStable(before, after *corev1.Pod, containerName, profileName string, spec *proposal.Spec, expectedContainerID, expectedImageID string) bool {
	if before == nil || after == nil || before.UID == "" || before.UID != after.UID || before.Spec.NodeName != after.Spec.NodeName {
		return false
	}
	_, containerID, imageID, err := validateTargetPod(after, containerName, profileName, spec)
	return err == nil && containerID == expectedContainerID && imageID == expectedImageID
}

func candidateDigestForVerification(s *proposal.Spec) (string, error) {
	if s == nil {
		return "", fmt.Errorf("proposal missing")
	}
	if s.CandidateVersion == proposal.CandidateVersionV2 {
		c, e := s.CandidateV2()
		if e != nil {
			return "", e
		}
		return proposal.CandidateDigestV2(c)
	}
	return proposal.CandidateDigest(*s)
}
func candidateDigestForVerificationMust(s *proposal.Spec) string {
	v, _ := candidateDigestForVerification(s)
	return v
}
func desiredSPOProfile(content string) (string, map[string]interface{}, error) {
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &obj); err != nil {
		return "", nil, err
	}
	meta, _ := obj["metadata"].(map[string]interface{})
	name, _ := meta["name"].(string)
	spec, _ := obj["spec"].(map[string]interface{})
	if name == "" || len(spec) == 0 {
		return "", nil, fmt.Errorf("profile name or spec missing")
	}
	return name, spec, nil
}
func readExactSPOProfile(ctx context.Context, dyn dynamic.Interface, name string, want map[string]interface{}) (*unstructured.Unstructured, string, error) {
	if dyn == nil {
		return nil, "", fmt.Errorf("SPO profile reader unavailable")
	}
	obj, err := dyn.Resource(spobackend.SeccompProfileGVR()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}
	got, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil, "", fmt.Errorf("live profile spec unreadable")
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		return nil, "", fmt.Errorf("live SPO profile content differs from approved artifact")
	}
	localhostPath, found, err := unstructured.NestedString(obj.Object, "status", "localhostProfile")
	if err != nil || !found || localhostPath != spobackend.LocalhostProfilePath(name) {
		return nil, "", fmt.Errorf("SPO has not reported the exact expected localhostProfile materialization path")
	}
	sum := sha256.Sum256(a)
	return obj, "sha256:" + hex.EncodeToString(sum[:]), nil
}
func validateTargetPod(p *corev1.Pod, container, profile string, spec *proposal.Spec) (string, string, string, error) {
	if p == nil || p.Status.Phase != corev1.PodRunning || p.Spec.NodeName == "" {
		return "", "", "", fmt.Errorf("target Pod is not running on a node")
	}
	expected := spobackend.LocalhostProfilePath(profile)
	if p.Spec.SecurityContext == nil || p.Spec.SecurityContext.SeccompProfile == nil || p.Spec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeLocalhost || p.Spec.SecurityContext.SeccompProfile.LocalhostProfile == nil || *p.Spec.SecurityContext.SeccompProfile.LocalhostProfile != expected {
		return "", "", "", fmt.Errorf("target Pod does not declare the approved Localhost profile")
	}
	for _, c := range p.Spec.Containers {
		if c.Name == container && c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
			sp := c.SecurityContext.SeccompProfile
			if sp.Type != corev1.SeccompProfileTypeLocalhost || sp.LocalhostProfile == nil || *sp.LocalhostProfile != expected {
				return "", "", "", fmt.Errorf("selected container overrides the approved Localhost profile")
			}
		}
	}
	var workloadUID string
	for _, o := range p.OwnerReferences {
		if o.UID != "" {
			workloadUID = string(o.UID)
			break
		}
	}
	if workloadUID == "" {
		return "", "", "", fmt.Errorf("target workload owner identity unavailable")
	}
	for _, c := range p.Status.ContainerStatuses {
		if c.Name == container && c.Ready && c.ContainerID != "" && c.ImageID != "" {
			if spec.Subject != nil && spec.Subject.ImageIdentity != "" && !imageIdentityMatches(c.ImageID, spec.Subject.ImageIdentity) {
				return "", "", "", fmt.Errorf("target image identity differs from candidate")
			}
			return workloadUID, c.ContainerID, c.ImageID, nil
		}
	}
	return "", "", "", fmt.Errorf("selected target container is not ready")
}

func imageIdentityMatches(runtimeID, expected string) bool {
	if at := strings.LastIndex(expected, "@"); at >= 0 {
		expected = expected[at+1:]
	}
	return expected != "" && strings.HasSuffix(runtimeID, expected)
}

func resolveTargetWorkload(ctx context.Context, service *workload.Service, target *corev1.Pod, spec *proposal.Spec) (*workload.Workload, error) {
	if service == nil || target == nil || spec == nil || spec.Subject == nil {
		return nil, fmt.Errorf("logical workload identity is unavailable")
	}
	discovered, err := service.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("revalidating logical workload identity: %w", err)
	}
	for i := range discovered.Workloads {
		w := &discovered.Workloads[i]
		if w.UID == "" || w.Target.Kind+"/"+w.Target.Name != spec.Subject.Target {
			continue
		}
		for _, p := range w.Pods {
			if p.UID == string(target.UID) && p.Name == target.Name {
				for _, c := range p.Containers {
					if c.Name == spec.Subject.Container {
						return w, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("selected Pod is not a current replica of the approved logical workload")
}
func targetWorkloadName(s *proposal.Spec) string {
	if s.Subject != nil {
		return s.Subject.Target
	}
	return "UNKNOWN"
}
func podRuntimeClass(p *corev1.Pod) string {
	if p.Spec.RuntimeClassName == nil {
		return ""
	}
	return *p.Spec.RuntimeClassName
}
func containerRuntimeID(p *corev1.Pod, name string) string {
	if p == nil {
		return ""
	}
	for _, c := range p.Status.ContainerStatuses {
		if c.Name == name {
			return c.ContainerID
		}
	}
	return ""
}
func runtimeFromContainerID(id string) string { v, _, _ := strings.Cut(id, "://"); return v }
func verifierIdentity(target seccompverification.Identity, p seccompverification.PodResult, mode, namespace, profileDigest string) seccompverification.Identity {
	v := target
	v.Namespace = namespace
	v.PodName = p.Name
	v.PodUID = p.UID
	v.ContainerID = p.ContainerID
	v.ImageID = p.ImageID
	v.Node = p.Node
	v.Runtime = runtimeFromContainerID(p.ContainerID)
	v.SeccompMode = mode
	v.ProfileDigest = profileDigest
	return v
}
func probeOutputForExit(code *int32, twin bool) *seccompverification.ProbeOutput {
	if code == nil {
		return nil
	}
	if *code == seccompverification.ExitEPERM && twin {
		return &seccompverification.ProbeOutput{Syscall: seccompverification.ProbeSyscall, Result: -1, Errno: 1}
	}
	if *code == seccompverification.ExitSuccess {
		return &seccompverification.ProbeOutput{Syscall: seccompverification.ProbeSyscall, Result: 0, Errno: 0}
	}
	// Exit 20 is explicitly inconclusive; all other non-protocol exits
	// indicate startup/runtime failure and must not be treated as denial.
	return nil
}
