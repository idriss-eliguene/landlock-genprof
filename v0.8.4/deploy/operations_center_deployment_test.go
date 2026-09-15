package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestOperationsCenterDeploymentIsSingleReplicaAndProxyBound(t *testing.T) {
	b, err := os.ReadFile("helm/landlock-genprof/templates/operations-center.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, required := range []string{
		"kind: Deployment",
		"replicas: 1",
		"type: Recreate",
		"LANDLOCK_GENPROF_DEPLOYMENT_MODE",
		"value: production",
		"LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET",
		"secretKeyRef:",
		"serviceAccountName:",
		"containerPort:",
		"LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_KUBECONFIG",
		"LANDLOCK_GENPROF_LOG_LEVEL",
		"LANDLOCK_GENPROF_METRICS_ENABLED",
		"LANDLOCK_GENPROF_METRICS_PORT",
		"type: ClusterIP",
		"terminationGracePeriodSeconds: 10",
		"startupProbe:",
		"exec:",
		"/healthz/startup",
		"livenessProbe:",
		"/healthz/live",
		"readinessProbe:",
		"/healthz/ready",
		"failureThreshold: 15",
		"failureThreshold: 3",
		"failureThreshold: 2",
		"terminationGracePeriodSeconds: 10",
	} {
		if !strings.Contains(s, required) {
			t.Fatalf("Operations Center template missing %q", required)
		}
	}
	for _, forbidden := range []string{"kind: Ingress", "type: NodePort", "type: LoadBalancer", "replicas: {{"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("Operations Center template contains forbidden or unconstrained %q", forbidden)
		}
	}
	if strings.Contains(s, "httpGet:") || strings.Contains(s, "preStop:") {
		t.Fatal("Operations Center lifecycle probes must not add network probe ingress or an arbitrary preStop delay")
	}
}

func TestOperationsCenterHardeningTemplateIsExplicit(t *testing.T) {
	deployment, err := os.ReadFile("helm/landlock-genprof/templates/operations-center.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(deployment)
	for _, required := range []string{
		"automountServiceAccountToken: true",
		"hostNetwork: false",
		"hostPID: false",
		"hostIPC: false",
		"type: RuntimeDefault",
		"runAsNonRoot: true",
		"allowPrivilegeEscalation: false",
		"readOnlyRootFilesystem: true",
		"- ALL",
		"readOnly: true",
	} {
		if !strings.Contains(s, required) {
			t.Fatalf("Operations Center hardening missing %q", required)
		}
	}
	for _, forbidden := range []string{"privileged: true", "hostPath:", "hostPort:"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("Operations Center template contains forbidden %q", forbidden)
		}
	}
}

func TestOperationsCenterNetworkPolicyIsProxyOnlyAndBounded(t *testing.T) {
	b, err := os.ReadFile("helm/landlock-genprof/templates/operations-center-networkpolicy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, required := range []string{
		"kind: NetworkPolicy",
		"policyTypes:",
		"- Ingress",
		"- Egress",
		"trustedProxy.namespaceSelector",
		"trustedProxy.podSelector",
		"kubernetesApiCIDRs",
		"kubernetesApiPorts",
		"namespaceSelector:",
		"podSelector:",
		"port: {{ $port }}",
		"metrics.networkPolicy.monitoring.namespaceSelector",
		"metrics.networkPolicy.monitoring.podSelector",
		"port: 53",
	} {
		if !strings.Contains(s, required) {
			t.Fatalf("NetworkPolicy template missing %q", required)
		}
	}
	for _, forbidden := range []string{"from: [{}]", "cidr: \"0.0.0.0/0\"", "cidr: \"::/0\"", "type: NodePort", "type: LoadBalancer"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("NetworkPolicy template contains forbidden %q", forbidden)
		}
	}
}

func TestOperationsCenterCiliumPolicyIsAPIEntityOnly(t *testing.T) {
	b, err := os.ReadFile("helm/landlock-genprof/templates/operations-center-cilium-networkpolicy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, required := range []string{"kind: CiliumNetworkPolicy", "endpointSelector:", "toEntities:", "kube-apiserver", "toPorts:"} {
		if !strings.Contains(s, required) {
			t.Fatalf("Cilium API policy missing %q", required)
		}
	}
	for _, forbidden := range []string{"toEntities:\n        - world", "0.0.0.0/0", "toFQDNs:", "toEndpoints:"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("Cilium API policy contains forbidden broad or speculative access %q", forbidden)
		}
	}
}

func TestObservationExecutorDeploymentIsDedicatedAndBounded(t *testing.T) {
	deployment, err := os.ReadFile("helm/landlock-genprof/templates/observation-executor.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(deployment)
	for _, required := range []string{
		"kind: Deployment",
		"name: landlock-genprof-observation-executor",
		"replicas: 1",
		"type: Recreate",
		"serviceAccountName: {{ .Values.observationExecutor.serviceAccount.name }}",
		"- executor",
		"runAsNonRoot: true",
		"allowPrivilegeEscalation: false",
		"readOnlyRootFilesystem: true",
		"type: RuntimeDefault",
		"- ALL",
		"resources:",
	} {
		if !strings.Contains(s, required) {
			t.Fatalf("executor deployment missing %q", required)
		}
	}
	for _, forbidden := range []string{"privileged: true", "hostNetwork: true", "hostPID: true", "hostIPC: true", "hostPath:", "hostPort:"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("executor deployment contains forbidden %q", forbidden)
		}
	}
}

func TestObservationExecutorNetworkPolicyHasNoIngressAndBoundedEgress(t *testing.T) {
	policy, err := os.ReadFile("helm/landlock-genprof/templates/observation-executor-networkpolicy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(policy)
	for _, required := range []string{"kind: NetworkPolicy", "ingress:", "kubernetesApiCIDRs", "port: 53", "kubernetesApiPorts", "metrics.networkPolicy.monitoring.namespaceSelector", "metrics.networkPolicy.monitoring.podSelector"} {
		if !strings.Contains(s, required) {
			t.Fatalf("executor network policy missing %q", required)
		}
	}
	for _, forbidden := range []string{"from: [{}]", "cidr: \"0.0.0.0/0\"", "cidr: \"::/0\"", "toFQDNs:", "toEntities:\n        - world"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("executor network policy contains forbidden %q", forbidden)
		}
	}
}
