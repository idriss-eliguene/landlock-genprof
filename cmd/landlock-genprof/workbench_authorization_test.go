package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestWorkbenchAuthorizationRejectsUnsignedRequest(t *testing.T) {
	t.Setenv(trustedProxyHMACSecretEnv, "01234567890123456789012345678901")
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	setExecutorKubeconfig(t)
	factory, err := enableWorkbenchAuthorizationWithResolver(context.Background(), &rest.Config{Host: "https://cluster.example"}, "team-a", sameTestCluster)
	if err != nil {
		t.Fatal(err)
	}
	if factory == nil {
		t.Fatal("authorization was not enabled")
	}
	server := &workbenchServer{requestContext: factory, allowedHost: "127.0.0.1:8080", allowedOrigin: "http://127.0.0.1:8080", sema: make(chan struct{}, 1)}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/workbench.js", nil)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned request status = %d, want 401", res.Code)
	}
}

func TestWorkbenchDeploymentConfigProductionFailsClosedWithoutTrustConfig(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "production")
	t.Setenv(trustedProxyHMACSecretEnv, "")
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	t.Setenv(observationExecutorKubeconfigEnv, "")
	if err := validateWorkbenchDeploymentConfig("team-a"); err == nil {
		t.Fatal("production mode accepted missing trust configuration")
	}
}

func TestWorkbenchDeploymentConfigProductionRejectsInvalidTrustConfig(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "production")
	t.Setenv(trustedProxyHMACSecretEnv, "too-short")
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	setExecutorKubeconfig(t)
	if err := validateWorkbenchDeploymentConfig("team-a"); err == nil {
		t.Fatal("production mode accepted a short trust secret")
	}
}

func TestWorkbenchDeploymentConfigProductionRequiresAllowlistAndExecutor(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "production")
	t.Setenv(trustedProxyHMACSecretEnv, "01234567890123456789012345678901")
	t.Setenv(operationsCenterAllowedGroupsEnv, "")
	t.Setenv(operationsCenterAllowedUsersEnv, "")
	t.Setenv(observationExecutorKubeconfigEnv, "")
	if err := validateWorkbenchDeploymentConfig("team-a"); err == nil {
		t.Fatal("production mode accepted empty authority and executor configuration")
	}
}

func TestWorkbenchDeploymentConfigProductionAcceptsValidConfig(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "production")
	t.Setenv(trustedProxyHMACSecretEnv, "01234567890123456789012345678901")
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	t.Setenv(workbenchAllowedHostEnv, "operations-center.example:8080")
	setExecutorKubeconfig(t)
	if err := validateWorkbenchDeploymentConfig("team-a"); err != nil {
		t.Fatalf("valid production configuration rejected: %v", err)
	}
}

func TestWorkbenchDeploymentConfigLocalModeRemainsCompatible(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "local")
	t.Setenv(trustedProxyHMACSecretEnv, "")
	t.Setenv(operationsCenterAllowedGroupsEnv, "")
	t.Setenv(observationExecutorKubeconfigEnv, "")
	if err := validateWorkbenchDeploymentConfig("team-a"); err != nil {
		t.Fatalf("local configuration rejected: %v", err)
	}
}

func TestWorkbenchAuthorizationFailsClosedWithoutExecutorConfiguration(t *testing.T) {
	t.Setenv(trustedProxyHMACSecretEnv, "01234567890123456789012345678901")
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	t.Setenv(observationExecutorKubeconfigEnv, "")
	if _, err := enableWorkbenchAuthorization(context.Background(), &rest.Config{Host: "https://cluster.example"}, "team-a"); err == nil {
		t.Fatal("authenticated Observation execution must fail closed without executor configuration")
	}
}

func TestWorkbenchAuthorizationAcceptsSignedIdentityForRequestSetup(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	t.Setenv(trustedProxyHMACSecretEnv, string(secret))
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	setExecutorKubeconfig(t)
	factory, err := enableWorkbenchAuthorizationWithResolver(context.Background(), &rest.Config{Host: "https://cluster.example"}, "team-a", sameTestCluster)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/workbench.js", nil)
	identity := authn.Identity{Username: "alice@company", Groups: []string{"team-a"}}
	normalized, err := authn.Normalize(identity)
	if err != nil {
		t.Fatal(err)
	}
	h := hmac.New(sha256.New, secret)
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = h.Write([]byte(timestamp + "\n" + normalized.Username + "\n" + normalized.Groups[0]))
	request.Header.Set(authn.UserHeader, identity.Username)
	request.Header.Set(authn.GroupsHeader, identity.Groups[0])
	request.Header.Set(authn.ProxyHeader, "true")
	request.Header.Set(authn.TimestampHeader, timestamp)
	request.Header.Set(authn.SignatureHeader, hex.EncodeToString(h.Sum(nil)))
	if _, err := factory(request); err != nil {
		t.Fatal(err)
	}
}

func sameTestCluster(context.Context, kubernetes.Interface) (observationdomain.ClusterIdentity, error) {
	return observationdomain.NewClusterIdentity("test-cluster")
}

func TestWorkbenchAuthorizationRejectsDifferentExecutorCluster(t *testing.T) {
	t.Setenv(trustedProxyHMACSecretEnv, "01234567890123456789012345678901")
	t.Setenv(operationsCenterAllowedGroupsEnv, "team-a")
	setExecutorKubeconfig(t)
	called := 0
	_, err := enableWorkbenchAuthorizationWithResolver(context.Background(), &rest.Config{Host: "https://cluster.example"}, "team-a", func(context.Context, kubernetes.Interface) (observationdomain.ClusterIdentity, error) {
		called++
		if called == 1 {
			return observationdomain.NewClusterIdentity("base-cluster")
		}
		return observationdomain.NewClusterIdentity("different-cluster")
	})
	if err == nil {
		t.Fatal("expected executor cluster mismatch to fail closed")
	}
}

func setExecutorKubeconfig(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "executor.kubeconfig")
	contents := []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: https://cluster.example\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: executor\ncurrent-context: test\nusers:\n- name: executor\n  user: {}\n")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(observationExecutorKubeconfigEnv, path)
}
