// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
)

// ReadState is the stable semantic result class for a bounded Kubernetes
// read. EMPTY is represented by an empty successful result, not an error.
type ReadState string

const (
	ReadNotFound            ReadState = "NOT_FOUND"
	ReadPermissionDenied    ReadState = "PERMISSION_DENIED"
	ReadBackendNotInstalled ReadState = "BACKEND_NOT_INSTALLED"
	ReadTimeout             ReadState = "TIMEOUT"
	ReadUnsupported         ReadState = "UNSUPPORTED"
	ReadUnknown             ReadState = "UNKNOWN"
)

// ReadError preserves the API cause while exposing a semantic state to a
// future Workbench consumer. It never maps permission or backend failures to
// an empty result.
type ReadError struct {
	State    ReadState
	Resource string
	Err      error
}

func (e *ReadError) Error() string {
	if e.Resource == "" {
		return fmt.Sprintf("kubernetes read %s: %v", e.State, e.Err)
	}
	return fmt.Sprintf("kubernetes read %s for %s: %v", e.State, e.Resource, e.Err)
}

func (e *ReadError) Unwrap() error { return e.Err }

// ReadSessionIdentity records the effective configuration selected at session
// construction. The clients are created once from a copied rest.Config and
// are never rebuilt from a mutable kubeconfig during a session.
type ReadSessionIdentity struct {
	KubeconfigSource string
	Context          string
	ClusterServer    string
	Namespace        string
}

// WorkbenchReadCapability is the only Kubernetes surface intended for
// future Workbench read consumers. It deliberately has no generic client,
// arbitrary GVR, watch, or mutation method.
type WorkbenchReadCapability interface {
	SessionIdentity() ReadSessionIdentity
	GetPod(context.Context, string) (*corev1.Pod, error)
	ListPods(context.Context) ([]corev1.Pod, error)
	GetDeployment(context.Context, string) (*unstructured.Unstructured, error)
	GetStatefulSet(context.Context, string) (*unstructured.Unstructured, error)
	GetDaemonSet(context.Context, string) (*unstructured.Unstructured, error)
	GetReplicaSet(context.Context, string) (*unstructured.Unstructured, error)
	GetProposal(context.Context, string) (*unstructured.Unstructured, error)
	ListProposals(context.Context) (*unstructured.UnstructuredList, error)
	GetTrainingHistory(context.Context, string) (*unstructured.Unstructured, error)
	GetPodLock(context.Context, string) (*unstructured.Unstructured, error)
	GetSPOProfile(context.Context, string) (*unstructured.Unstructured, error)
	ListNetworkPolicies(context.Context) (*unstructured.UnstructuredList, error)
}

// ReadSession owns private Kubernetes clients for one pinned read session.
// Existing CLI mutation paths continue to construct their own broad clients.
type ReadSession struct {
	core      kubernetes.Interface
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
	identity  ReadSessionIdentity
}

// NewReadSession copies config and constructs all clients once. namespace is
// the application query scope; an empty namespace is rejected so callers do
// not accidentally obtain an all-namespaces LIST capability.
func NewReadSession(config *rest.Config, namespace string) (*ReadSession, error) {
	if config == nil {
		return nil, fmt.Errorf("read session requires a Kubernetes config")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("read session requires a namespace scope")
	}
	pinned := rest.CopyConfig(config)
	core, err := kubernetes.NewForConfig(pinned)
	if err != nil {
		return nil, fmt.Errorf("constructing read client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(pinned)
	if err != nil {
		return nil, fmt.Errorf("constructing dynamic read client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(pinned)
	if err != nil {
		return nil, fmt.Errorf("constructing discovery read client: %w", err)
	}
	return &ReadSession{
		core: core, dynamic: dyn, discovery: disc,
		identity: ReadSessionIdentity{ClusterServer: pinned.Host, Namespace: namespace},
	}, nil
}

// NewReadSessionFromKubeconfig resolves one kubeconfig/context at startup,
// then delegates to NewReadSession. Later current-context changes cannot
// affect the already-created session.
func NewReadSessionFromKubeconfig(kubeconfigPath, contextName, namespace string) (*ReadSession, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	if contextName == "" {
		contextName = raw.CurrentContext
	}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, configOverrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("selecting kubeconfig context %q: %w", contextName, err)
	}
	session, err := NewReadSession(restConfig, namespace)
	if err != nil {
		return nil, err
	}
	clusterName := ""
	if selected, ok := raw.Contexts[contextName]; ok {
		clusterName = selected.Cluster
	}
	session.identity.KubeconfigSource = rules.GetLoadingPrecedence()[0]
	session.identity.Context = contextName
	if cluster, ok := raw.Clusters[clusterName]; ok {
		session.identity.ClusterServer = cluster.Server
	}
	return session, nil
}

// NewReadSessionForClients is intended for deterministic unit tests and
// adapter composition. It still returns only the narrow read capability.
func NewReadSessionForClients(core kubernetes.Interface, dyn dynamic.Interface, disc discovery.DiscoveryInterface, namespace string) (*ReadSession, error) {
	if core == nil || dyn == nil || disc == nil {
		return nil, fmt.Errorf("read session requires core, dynamic, and discovery clients")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("read session requires a namespace scope")
	}
	return &ReadSession{core: core, dynamic: dyn, discovery: disc, identity: ReadSessionIdentity{Namespace: namespace}}, nil
}

func (s *ReadSession) SessionIdentity() ReadSessionIdentity { return s.identity }

func (s *ReadSession) GetPod(ctx context.Context, name string) (*corev1.Pod, error) {
	obj, err := s.core.CoreV1().Pods(s.identity.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, classifyReadError(err, "pods")
	}
	return obj, nil
}

func (s *ReadSession) ListPods(ctx context.Context) ([]corev1.Pod, error) {
	list, err := s.core.CoreV1().Pods(s.identity.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, classifyReadError(err, "pods")
	}
	return list.Items, nil
}

var (
	deploymentGVR    = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	statefulSetGVR   = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	daemonSetGVR     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	replicaSetGVR    = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	proposalGVR      = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
	historyGVR       = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "traininghistories"}
	podLockGVR       = schema.GroupVersionResource{Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles"}
	networkPolicyGVR = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
)

func (s *ReadSession) GetDeployment(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, deploymentGVR, name)
}
func (s *ReadSession) GetStatefulSet(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, statefulSetGVR, name)
}
func (s *ReadSession) GetDaemonSet(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, daemonSetGVR, name)
}
func (s *ReadSession) GetReplicaSet(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, replicaSetGVR, name)
}
func (s *ReadSession) GetProposal(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, proposalGVR, name)
}
func (s *ReadSession) ListProposals(ctx context.Context) (*unstructured.UnstructuredList, error) {
	return s.listOptional(ctx, proposalGVR)
}
func (s *ReadSession) GetTrainingHistory(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, historyGVR, name)
}
func (s *ReadSession) GetPodLock(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getOptional(ctx, podLockGVR, name)
}
func (s *ReadSession) GetSPOProfile(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return s.getClusterOptional(ctx, spobackend.SeccompProfileGVR(), name)
}
func (s *ReadSession) ListNetworkPolicies(ctx context.Context) (*unstructured.UnstructuredList, error) {
	return s.listOptional(ctx, networkPolicyGVR)
}

func (s *ReadSession) getOptional(ctx context.Context, gvr schema.GroupVersionResource, name string) (*unstructured.Unstructured, error) {
	if err := s.ensureResource(ctx, gvr); err != nil {
		return nil, err
	}
	obj, err := s.dynamic.Resource(gvr).Namespace(s.identity.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, classifyReadError(err, gvr.Resource)
	}
	return obj, nil
}

func (s *ReadSession) getClusterOptional(ctx context.Context, gvr schema.GroupVersionResource, name string) (*unstructured.Unstructured, error) {
	if err := s.ensureResource(ctx, gvr); err != nil {
		return nil, err
	}
	obj, err := s.dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, classifyReadError(err, gvr.Resource)
	}
	return obj, nil
}

func (s *ReadSession) listOptional(ctx context.Context, gvr schema.GroupVersionResource) (*unstructured.UnstructuredList, error) {
	if err := s.ensureResource(ctx, gvr); err != nil {
		return nil, err
	}
	list, err := s.dynamic.Resource(gvr).Namespace(s.identity.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, classifyReadError(err, gvr.Resource)
	}
	return list, nil
}

func (s *ReadSession) ensureResource(ctx context.Context, gvr schema.GroupVersionResource) error {
	resources, err := s.discovery.ServerResourcesForGroupVersion(gvr.Group + "/" + gvr.Version)
	if apierrors.IsNotFound(err) || (err == nil && !resourceInList(resources, gvr.Resource)) {
		return &ReadError{State: ReadBackendNotInstalled, Resource: gvr.Resource, Err: fmt.Errorf("resource %s is not served", gvr.Resource)}
	}
	if err != nil {
		return classifyReadError(err, gvr.Resource)
	}
	return nil
}

func resourceInList(resources *metav1.APIResourceList, resource string) bool {
	if resources == nil {
		return false
	}
	for _, item := range resources.APIResources {
		if item.Name == resource {
			return true
		}
	}
	return false
}

func classifyReadError(err error, resource string) error {
	if err == nil {
		return nil
	}
	state := ReadUnknown
	switch {
	case apierrors.IsNotFound(err):
		state = ReadNotFound
	case apierrors.IsForbidden(err):
		state = ReadPermissionDenied
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err), err == context.DeadlineExceeded:
		state = ReadTimeout
	}
	return &ReadError{State: state, Resource: resource, Err: err}
}
