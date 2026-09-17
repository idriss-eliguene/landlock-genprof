// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package environment owns the server-side boundary between an Operations
// Center environment selection and Kubernetes credentials.  It deliberately
// exposes metadata, never a REST config or credential-bearing client.
package environment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	ErrSessionNotFound      = errors.New("environment session not found")
	ErrStaleContext         = errors.New("stale environment context")
	ErrExecPluginNotAllowed = errors.New("kubeconfig exec plugin requires explicit local opt-in")
	ErrConnectorUnavailable = errors.New("environment connector unavailable")
)

// CredentialContext describes credential provenance without containing a
// token, certificate, key, kubeconfig payload, or exec-plugin output.
type CredentialContext struct {
	Source     string `json:"source,omitempty"`
	AuthMethod string `json:"authMethod,omitempty"`
}

// ProductInstallationIdentity distinguishes installations sharing a cluster.
type ProductInstallationIdentity string

// EnvironmentContext is immutable after construction. Namespace is optional
// during M1 environment discovery and is not an authorization grant.
type EnvironmentContext struct {
	cluster        observationdomain.ClusterIdentity
	locator        observationdomain.ClusterLocator
	credential     CredentialContext
	installation   ProductInstallationIdentity
	namespace      string
	sessionID      string
	contextVersion uint64
}

func (c EnvironmentContext) ClusterIdentity() observationdomain.ClusterIdentity { return c.cluster }
func (c EnvironmentContext) ClusterLocator() observationdomain.ClusterLocator   { return c.locator }
func (c EnvironmentContext) CredentialContext() CredentialContext               { return c.credential }
func (c EnvironmentContext) ProductInstallation() ProductInstallationIdentity   { return c.installation }
func (c EnvironmentContext) Namespace() string                                  { return c.namespace }
func (c EnvironmentContext) SessionID() string                                  { return c.sessionID }
func (c EnvironmentContext) ContextVersion() uint64                             { return c.contextVersion }

// Metadata is the only representation returned to an HTTP/UI boundary.
type Metadata struct {
	ContextName      string            `json:"contextName"`
	ClusterServer    string            `json:"clusterServer,omitempty"`
	DefaultNamespace string            `json:"defaultNamespace,omitempty"`
	ClusterIdentity  string            `json:"clusterIdentity"`
	Credential       CredentialContext `json:"credential"`
	SessionID        string            `json:"sessionID,omitempty"`
	ContextVersion   uint64            `json:"contextVersion,omitempty"`
}

// DiscoveredContext contains safe kubeconfig locator metadata and, when it can
// be resolved without an exec plugin, the durable cluster identity. Discovery
// never returns credential material or executes an exec plugin.
type DiscoveredContext struct {
	ContextName        string            `json:"contextName"`
	ClusterName        string            `json:"clusterName,omitempty"`
	ClusterServer      string            `json:"clusterServer,omitempty"`
	ClusterIdentity    string            `json:"clusterIdentity,omitempty"`
	ClusterDisplayName string            `json:"clusterDisplayName,omitempty"`
	DefaultNamespace   string            `json:"defaultNamespace,omitempty"`
	Credential         CredentialContext `json:"credential"`
}

type OpenRequest struct {
	ContextName         string
	Namespace           string
	ProductInstallation ProductInstallationIdentity
	AllowExecPlugins    bool
	ExecPluginTimeout   time.Duration
}

// EnvironmentSession is an immutable, server-side binding. The Kubernetes
// client and REST configuration are intentionally private to this package.
type EnvironmentSession struct {
	context   EnvironmentContext
	core      kubernetes.Interface
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

func (s *EnvironmentSession) Context() EnvironmentContext { return s.context }
func (s *EnvironmentSession) Metadata() Metadata          { return metadataFor(s.context) }

// WorkbenchClients returns server-side clients for a request-scoped,
// namespace-pinned binding. These clients never cross the HTTP boundary.
func (s *EnvironmentSession) WorkbenchClients(namespace string) (*k8s.ReadSession, kubernetes.Interface, dynamic.Interface, error) {
	if s == nil || s.core == nil || s.dynamic == nil || s.discovery == nil {
		return nil, nil, nil, fmt.Errorf("environment session has no Kubernetes clients")
	}
	locator := s.context.ClusterLocator()
	reads, err := k8s.NewReadSessionForClientsWithIdentity(s.core, s.dynamic, s.discovery, k8s.ReadSessionIdentity{
		KubeconfigSource: locator.KubeconfigSource,
		Context:          locator.Context,
		ClusterServer:    locator.APIURL,
		Namespace:        namespace,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return reads, s.core, s.dynamic, nil
}

// ClusterConnector is deliberately small. Implementations own credential
// resolution and server-side clients; callers receive safe metadata/session
// handles only.
type ClusterConnector interface {
	Discover(context.Context) ([]DiscoveredContext, error)
	Open(context.Context, OpenRequest) (*EnvironmentSession, error)
	Metadata(string) (Metadata, error)
	Session(string) (*EnvironmentSession, error)
	Validate(string, uint64, observationdomain.ClusterIdentity) (*EnvironmentSession, error)
	Close(string) error
}

type LocalKubeconfigConnector struct {
	path      string
	install   ProductInstallationIdentity
	mu        sync.RWMutex
	sessions  map[string]*EnvironmentSession
	loadRules *clientcmd.ClientConfigLoadingRules
}

func NewLocalKubeconfigConnector(path string, installation ProductInstallationIdentity) *LocalKubeconfigConnector {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		rules.ExplicitPath = path
	}
	return &LocalKubeconfigConnector{path: path, install: installation, sessions: make(map[string]*EnvironmentSession), loadRules: rules}
}

func (c *LocalKubeconfigConnector) load() (*clientcmdapi.Config, error) {
	raw, err := c.loadRules.Load()
	if err != nil {
		return nil, fmt.Errorf("%w: loading kubeconfig: %v", ErrConnectorUnavailable, err)
	}
	return raw, nil
}

func (c *LocalKubeconfigConnector) Discover(ctx context.Context) ([]DiscoveredContext, error) {
	raw, err := c.load()
	if err != nil {
		return nil, err
	}
	result := make([]DiscoveredContext, 0, len(raw.Contexts))
	for name, selected := range raw.Contexts {
		item := DiscoveredContext{ContextName: name, Credential: credentialMetadata(raw, selected)}
		item.ClusterName = selected.Cluster
		item.ClusterDisplayName = selected.Cluster
		if cluster, ok := raw.Clusters[selected.Cluster]; ok {
			item.ClusterServer = cluster.Server
		}
		item.DefaultNamespace = selected.Namespace
		// Discovery may resolve durable identity for ordinary static
		// credentials, but must never execute an arbitrary exec plugin merely
		// to populate a selector. Such contexts remain selectable and will
		// report the normal explicit connector error when opened.
		if authInfo, ok := raw.AuthInfos[selected.AuthInfo]; !ok || authInfo.Exec == nil {
			identity, identityErr := c.discoverIdentity(ctx, name)
			if identityErr != nil {
				// A malformed or unreachable context must not poison discovery of
				// otherwise usable contexts. Keep its safe locator metadata, but
				// leave identity empty so callers cannot group or open it as a
				// verified environment.
				result = append(result, item)
				continue
			}
			item.ClusterIdentity = string(identity.NamespaceUID)
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ContextName < result[j].ContextName })
	return result, nil
}

func (c *LocalKubeconfigConnector) discoverIdentity(ctx context.Context, name string) (observationdomain.ClusterIdentity, error) {
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(c.loadRules, &clientcmd.ConfigOverrides{CurrentContext: name})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return observationdomain.ClusterIdentity{}, fmt.Errorf("%w: selecting context %q: %v", ErrConnectorUnavailable, name, err)
	}
	core, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return observationdomain.ClusterIdentity{}, fmt.Errorf("%w: constructing discovery client: %v", ErrConnectorUnavailable, err)
	}
	return k8s.ResolveClusterIdentity(ctx, core)
}

func (c *LocalKubeconfigConnector) Open(ctx context.Context, request OpenRequest) (*EnvironmentSession, error) {
	raw, err := c.load()
	if err != nil {
		return nil, err
	}
	name := request.ContextName
	if name == "" {
		name = raw.CurrentContext
	}
	selected, ok := raw.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("%w: context %q not found", ErrConnectorUnavailable, name)
	}
	if selected == nil {
		return nil, fmt.Errorf("%w: context %q is empty", ErrConnectorUnavailable, name)
	}
	if selectedUser, ok := raw.AuthInfos[selected.AuthInfo]; ok && selectedUser.Exec != nil && !request.AllowExecPlugins {
		return nil, fmt.Errorf("%w: context %q", ErrExecPluginNotAllowed, name)
	}
	if request.AllowExecPlugins {
		if request.ExecPluginTimeout <= 0 {
			request.ExecPluginTimeout = 30 * time.Second
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.ExecPluginTimeout)
		defer cancel()
	}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(c.loadRules, &clientcmd.ConfigOverrides{CurrentContext: name})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("%w: selecting context %q: %v", ErrConnectorUnavailable, name, err)
	}
	core, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: constructing Kubernetes client: %v", ErrConnectorUnavailable, err)
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: constructing dynamic client: %v", ErrConnectorUnavailable, err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: constructing discovery client: %v", ErrConnectorUnavailable, err)
	}
	identity, err := k8s.ResolveClusterIdentity(ctx, core)
	if err != nil {
		return nil, err
	}
	clusterName := selected.Cluster
	clusterServer := ""
	if cluster, ok := raw.Clusters[clusterName]; ok {
		clusterServer = cluster.Server
	}
	namespace := request.Namespace
	if namespace == "" {
		namespace = selected.Namespace
	}
	install := request.ProductInstallation
	if install == "" {
		install = c.install
	}
	sessionID, err := newSessionID()
	if err != nil {
		return nil, fmt.Errorf("creating environment session: %w", err)
	}
	session := &EnvironmentSession{core: core, dynamic: dynamicClient, discovery: discoveryClient, context: EnvironmentContext{
		cluster:    identity,
		locator:    observationdomain.ClusterLocator{KubeconfigSource: c.loadRules.GetLoadingPrecedence()[0], Context: name, APIURL: clusterServer},
		credential: credentialMetadata(raw, selected), installation: install,
		namespace: namespace, sessionID: sessionID, contextVersion: 1,
	}}
	c.mu.Lock()
	c.sessions[sessionID] = session
	c.mu.Unlock()
	return session, nil
}

func (c *LocalKubeconfigConnector) Metadata(sessionID string) (Metadata, error) {
	c.mu.RLock()
	session, ok := c.sessions[sessionID]
	c.mu.RUnlock()
	if !ok {
		return Metadata{}, ErrSessionNotFound
	}
	return session.Metadata(), nil
}

// Session is an internal server-side lookup. Callers must not serialize the
// returned session or expose its client; HTTP boundaries should use Metadata.
func (c *LocalKubeconfigConnector) Session(sessionID string) (*EnvironmentSession, error) {
	c.mu.RLock()
	session, ok := c.sessions[sessionID]
	c.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (c *LocalKubeconfigConnector) Validate(sessionID string, version uint64, identity observationdomain.ClusterIdentity) (*EnvironmentSession, error) {
	c.mu.RLock()
	session, ok := c.sessions[sessionID]
	c.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotFound
	}
	if session.context.contextVersion != version || session.context.cluster != identity {
		return nil, ErrStaleContext
	}
	return session, nil
}

func (c *LocalKubeconfigConnector) Close(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}
	delete(c.sessions, sessionID)
	return nil
}

func credentialMetadata(raw *clientcmdapi.Config, selected *clientcmdapi.Context) CredentialContext {
	method := "unknown"
	if selected != nil {
		if user, ok := raw.AuthInfos[selected.AuthInfo]; ok {
			switch {
			case user.Exec != nil:
				method = "exec-plugin"
			case user.AuthProvider != nil:
				method = "auth-provider"
			case user.Token != "" || user.TokenFile != "":
				method = "bearer-token"
			case user.ClientCertificate != "" || user.ClientKey != "":
				method = "client-certificate"
			default:
				method = "credential-reference"
			}
		}
	}
	return CredentialContext{Source: "local-kubeconfig", AuthMethod: method}
}

func metadataFor(c EnvironmentContext) Metadata {
	return Metadata{ContextName: c.locator.Context, ClusterServer: c.locator.APIURL, DefaultNamespace: c.namespace, ClusterIdentity: c.cluster.NamespaceUID, Credential: c.credential, SessionID: c.sessionID, ContextVersion: c.contextVersion}
}

func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
