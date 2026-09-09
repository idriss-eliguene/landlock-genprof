package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const trustedProxyHMACSecretEnv = "LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET"
const workbenchDeploymentModeEnv = "LANDLOCK_GENPROF_DEPLOYMENT_MODE"
const workbenchAllowedHostEnv = "LANDLOCK_GENPROF_ALLOWED_HOST"
const observationExecutorKubeconfigEnv = "LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_KUBECONFIG"
const observationExecutorContextEnv = "LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_CONTEXT"
const operationsCenterAllowedUsersEnv = "LANDLOCK_GENPROF_ALLOWED_USERS"
const operationsCenterAllowedGroupsEnv = "LANDLOCK_GENPROF_ALLOWED_GROUPS"

type workbenchRequestContext struct {
	identity        authn.Identity
	reads           k8s.WorkbenchReadCapability
	dynamic         dynamic.Interface
	discoverCaps    workbenchCapabilityDiscovery
	observations    *observationAPI
	clusterIdentity string
	authenticated   bool
}

type workbenchCapabilityDiscovery func(context.Context, string) (map[authz.Capability]bool, error)

// enableWorkbenchAuthorization is an explicit deployment opt-in. When set,
// every request must carry a signed external identity and receives a fresh
// impersonated Kubernetes client. The legacy local Workbench mode remains
// available for existing CLI/envtest workflows until the deployment is
// fronted by its trusted proxy.
func enableWorkbenchAuthorization(ctx context.Context, base *rest.Config, namespace string) (func(*http.Request) (workbenchRequestContext, error), error) {
	return enableWorkbenchAuthorizationWithResolver(ctx, base, namespace, k8s.ResolveClusterIdentity)
}

type clusterIdentityResolver func(context.Context, kubernetes.Interface) (observationdomain.ClusterIdentity, error)

func enableWorkbenchAuthorizationWithResolver(ctx context.Context, base *rest.Config, namespace string, resolve clusterIdentityResolver) (func(*http.Request) (workbenchRequestContext, error), error) {
	if err := validateWorkbenchDeploymentConfig(namespace); err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(os.Getenv(trustedProxyHMACSecretEnv))
	if secret == "" {
		return nil, nil
	}
	policy, err := configuredIdentityPolicy()
	if err != nil {
		return nil, err
	}
	verifier, err := authn.NewVerifierWithPolicy([]byte(secret), policy)
	if err != nil {
		return nil, fmt.Errorf("configuring Workbench trusted-proxy identity: %w", err)
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("authorized Workbench requires a namespace")
	}
	executorPath := strings.TrimSpace(os.Getenv(observationExecutorKubeconfigEnv))
	if executorPath == "" {
		return nil, fmt.Errorf("authenticated Observation execution requires %s", observationExecutorKubeconfigEnv)
	}
	executorClients, err := authz.NewConfiguredClients(executorPath, strings.TrimSpace(os.Getenv(observationExecutorContextEnv)))
	if err != nil {
		return nil, fmt.Errorf("configuring Observation executor: %w", err)
	}
	baseCore, err := kubernetes.NewForConfig(base)
	if err != nil {
		return nil, fmt.Errorf("constructing base Kubernetes client: %w", err)
	}
	baseCluster, err := resolve(ctx, baseCore)
	if err != nil {
		return nil, fmt.Errorf("resolving base cluster identity: %w", err)
	}
	executorCluster, err := resolve(ctx, executorClients.Core)
	if err != nil {
		return nil, fmt.Errorf("resolving executor cluster identity: %w", err)
	}
	if baseCluster != executorCluster {
		return nil, fmt.Errorf("executor Kubernetes config targets a different cluster")
	}
	return func(r *http.Request) (workbenchRequestContext, error) {
		identity, err := verifier.FromRequest(r)
		if err != nil {
			return workbenchRequestContext{}, fmt.Errorf("unauthenticated request: %w", err)
		}
		clients, err := authz.NewImpersonatedClients(base, identity)
		if err != nil {
			return workbenchRequestContext{}, err
		}
		reads, err := k8s.NewReadSessionForClients(clients.Core, clients.Dynamic, clients.Discovery, namespace)
		if err != nil {
			return workbenchRequestContext{}, err
		}
		humanObservationAPI, err := newObservationAPI(clients.Core, clients.Dynamic, namespace)
		if err != nil {
			return workbenchRequestContext{}, err
		}
		humanObservationAPI = humanObservationAPI.withExecutor(func() (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
			return executorClients.Core, executorClients.Dynamic, executorClients.Config, nil
		})
		return workbenchRequestContext{identity: identity, reads: reads, dynamic: clients.Dynamic, observations: humanObservationAPI, clusterIdentity: baseCluster.NamespaceUID, discoverCaps: func(ctx context.Context, namespace string) (map[authz.Capability]bool, error) {
			return authz.DiscoverCapabilities(ctx, clients.Authorization, namespace)
		}}, nil
	}, nil
}

// validateWorkbenchDeploymentConfig keeps the local CLI workflow compatible
// while making the production deployment contract explicit. Production mode
// must never silently select the legacy unauthenticated/local path.
func validateWorkbenchDeploymentConfig(namespace string) error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(workbenchDeploymentModeEnv)))
	switch mode {
	case "", "local", "development":
		return nil
	case "production":
		secret := strings.TrimSpace(os.Getenv(trustedProxyHMACSecretEnv))
		if secret == "" {
			return fmt.Errorf("production Operations Center requires %s", trustedProxyHMACSecretEnv)
		}
		if _, err := authn.NewVerifierWithPolicy([]byte(secret), authn.Policy{}); err != nil {
			return fmt.Errorf("invalid production Operations Center trust secret")
		}
		if _, err := configuredIdentityPolicy(); err != nil {
			return err
		}
		if strings.TrimSpace(namespace) == "" {
			return fmt.Errorf("production Operations Center requires a namespace")
		}
		if strings.TrimSpace(os.Getenv(observationExecutorKubeconfigEnv)) == "" {
			return fmt.Errorf("production Operations Center requires %s", observationExecutorKubeconfigEnv)
		}
		if strings.TrimSpace(os.Getenv(workbenchAllowedHostEnv)) == "" {
			return fmt.Errorf("production Operations Center requires %s", workbenchAllowedHostEnv)
		}
		return nil
	default:
		return fmt.Errorf("unsupported %s value %q", workbenchDeploymentModeEnv, mode)
	}
}

func configuredIdentityPolicy() (authn.Policy, error) {
	users := splitConfiguredPrincipals(os.Getenv(operationsCenterAllowedUsersEnv))
	groups := splitConfiguredPrincipals(os.Getenv(operationsCenterAllowedGroupsEnv))
	policy, err := authn.NewPolicy(users, groups)
	if err != nil {
		return authn.Policy{}, fmt.Errorf("configuring Operations Center identity allowlist: %w", err)
	}
	return policy, nil
}

func splitConfiguredPrincipals(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func (s *workbenchServer) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if s.requestContext == nil || s.requestIdentity.Username == "" || s.discoverCaps == nil {
		http.Error(w, "capability discovery requires authenticated request mode", http.StatusNotImplemented)
		return
	}
	capabilities, err := s.discoverCaps(r.Context(), s.reads.SessionIdentity().Namespace)
	if err != nil {
		http.Error(w, "capability discovery failed", http.StatusBadGateway)
		return
	}
	response := struct {
		Identity     authn.Identity            `json:"identity"`
		Namespace    string                    `json:"namespace"`
		Capabilities map[authz.Capability]bool `json:"capabilities"`
	}{Identity: s.requestIdentity, Namespace: s.reads.SessionIdentity().Namespace, Capabilities: capabilities}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
