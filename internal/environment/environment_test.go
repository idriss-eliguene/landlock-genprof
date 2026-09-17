// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package environment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func testClusterServer(t *testing.T, uid string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/kube-system" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"` + uid + `"}}`))
	}))
}

func testKubeconfig(t *testing.T, servers map[string]string, contexts map[string]string) string {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "context-a"
	cfg.AuthInfos["human"] = &clientcmdapi.AuthInfo{Token: "secret-token-must-not-escape"}
	for name, server := range servers {
		cfg.Clusters[name] = &clientcmdapi.Cluster{Server: server}
	}
	for name, cluster := range contexts {
		cfg.Contexts[name] = &clientcmdapi.Context{Cluster: cluster, AuthInfo: "human", Namespace: "payments"}
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLocalConnectorDiscoversSafeContextMetadata(t *testing.T) {
	server := testClusterServer(t, "cluster-x")
	defer server.Close()
	path := testKubeconfig(t, map[string]string{"cluster-x": server.URL}, map[string]string{"context-a": "cluster-x", "context-b": "cluster-x"})
	connector := NewLocalKubeconfigConnector(path, "installation-a")
	contexts, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 2 {
		t.Fatalf("contexts=%d, want 2", len(contexts))
	}
	if contexts[0].ClusterIdentity == "" || contexts[0].ClusterIdentity != contexts[1].ClusterIdentity {
		t.Fatalf("same cluster contexts did not share durable identity: %#v", contexts)
	}
	if contexts[0].ClusterDisplayName != "cluster-x" || contexts[1].ClusterDisplayName != "cluster-x" {
		t.Fatalf("cluster display name was not derived from safe locator metadata: %#v", contexts)
	}
	b, err := json.Marshal(contexts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret-token-must-not-escape") {
		t.Fatalf("credential leaked in discovery metadata: %s", b)
	}
}

func TestLocalConnectorSameClusterAndRenamedContextShareIdentity(t *testing.T) {
	server := testClusterServer(t, "cluster-x")
	defer server.Close()
	path := testKubeconfig(t, map[string]string{"cluster-x": server.URL}, map[string]string{"old-context": "cluster-x", "renamed-context": "cluster-x"})
	connector := NewLocalKubeconfigConnector(path, "installation-a")
	a, err := connector.Open(context.Background(), OpenRequest{ContextName: "old-context"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := connector.Open(context.Background(), OpenRequest{ContextName: "renamed-context"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Context().ClusterIdentity() != b.Context().ClusterIdentity() {
		t.Fatalf("identities differ: %#v %#v", a.Context().ClusterIdentity(), b.Context().ClusterIdentity())
	}
	if a.Context().ClusterLocator().Context != "old-context" || b.Context().ClusterLocator().Context != "renamed-context" {
		t.Fatal("context locator was not preserved")
	}
}

func TestLocalConnectorDifferentClustersAndConcurrentSessions(t *testing.T) {
	serverA := testClusterServer(t, "cluster-a")
	defer serverA.Close()
	serverB := testClusterServer(t, "cluster-b")
	defer serverB.Close()
	path := testKubeconfig(t, map[string]string{"a": serverA.URL, "b": serverB.URL}, map[string]string{"context-a": "a", "context-b": "b"})
	connector := NewLocalKubeconfigConnector(path, "installation-a")
	discovered, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]struct{}{}
	for _, item := range discovered {
		identities[item.ClusterIdentity] = struct{}{}
	}
	if len(identities) != 2 {
		t.Fatalf("discovery grouped %d durable clusters, want 2: %#v", len(identities), discovered)
	}
	a, err := connector.Open(context.Background(), OpenRequest{ContextName: "context-a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := connector.Open(context.Background(), OpenRequest{ContextName: "context-b"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Context().ClusterIdentity() == b.Context().ClusterIdentity() {
		t.Fatal("different API servers share cluster identity")
	}
	if a.Context().SessionID() == b.Context().SessionID() {
		t.Fatal("sessions are not independent")
	}
	if _, err := connector.Validate(a.Context().SessionID(), a.Context().ContextVersion(), b.Context().ClusterIdentity()); err != ErrStaleContext {
		t.Fatalf("cross-session validation error=%v, want stale context", err)
	}
}

func TestEnvironmentSessionIsImmutableAndClosable(t *testing.T) {
	server := testClusterServer(t, "cluster-x")
	defer server.Close()
	path := testKubeconfig(t, map[string]string{"cluster-x": server.URL}, map[string]string{"context-a": "cluster-x"})
	connector := NewLocalKubeconfigConnector(path, "installation-a")
	session, err := connector.Open(context.Background(), OpenRequest{ContextName: "context-a", Namespace: "payments"})
	if err != nil {
		t.Fatal(err)
	}
	before := session.Context()
	if before.ContextVersion() != 1 || before.Namespace() != "payments" {
		t.Fatalf("context=%#v", before)
	}
	if err := connector.Close(before.SessionID()); err != nil {
		t.Fatal(err)
	}
	if _, err := connector.Metadata(before.SessionID()); err != ErrSessionNotFound {
		t.Fatalf("metadata error=%v, want session not found", err)
	}
}

func TestLocalConnectorFailsClosedForExecPluginAndMissingContext(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "exec"
	cfg.Clusters["cluster"] = &clientcmdapi.Cluster{Server: "https://invalid.example"}
	cfg.AuthInfos["exec-user"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "definitely-not-authorized"}}
	cfg.Contexts["exec"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "exec-user"}
	path := filepath.Join(t.TempDir(), "config")
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatal(err)
	}
	connector := NewLocalKubeconfigConnector(path, "installation-a")
	if _, err := connector.Open(context.Background(), OpenRequest{ContextName: "exec"}); err == nil || !strings.Contains(err.Error(), "exec plugin requires") {
		t.Fatalf("exec error=%v", err)
	}
	if _, err := connector.Open(context.Background(), OpenRequest{ContextName: "missing"}); err == nil {
		t.Fatal("missing context unexpectedly opened")
	}
	if _, err := connector.Discover(context.Background()); err != nil {
		t.Fatal("discovery should inspect metadata without executing plugin: ", err)
	}
}
