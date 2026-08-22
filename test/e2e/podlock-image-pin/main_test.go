package main

import (
	"strings"
	"testing"
)

const validManifest = `apiVersion: apps/v1
kind: DaemonSet
metadata: {name: podlock-nri-plugin}
spec:
  template:
    spec:
      initContainers:
        - name: detect-landlock-support
          image: ghcr.io/flavio/podlock/nri:v0.1.0
      containers:
        - name: nri
          image: ghcr.io/flavio/podlock/nri:v0.1.0
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: podlock-controller}
spec:
  template:
    spec:
      containers:
        - name: controller
          image: ghcr.io/flavio/podlock/controller:v0.1.0
        - name: third-party
          image: example.com/third-party:1
`

func runManifest(t *testing.T, input string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := run(strings.NewReader(input), &out)
	return out.String(), err
}

func TestTransformOfficialShape(t *testing.T) {
	out, err := runManifest(t, validManifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{controllerPin, nriPin} {
		if !strings.Contains(out, image) {
			t.Fatalf("missing pinned image %q in output:\n%s", image, out)
		}
	}
	if strings.Count(out, controllerPin) != 1 || strings.Count(out, nriPin) != 2 {
		t.Fatalf("unexpected pin counts: controller=%d nri=%d", strings.Count(out, controllerPin), strings.Count(out, nriPin))
	}
	if strings.Contains(out, controllerTag) || strings.Contains(out, nriTag) || !strings.Contains(out, "example.com/third-party:1") {
		t.Fatalf("unexpected image mutation:\n%s", out)
	}
	second, err := runManifest(t, validManifest)
	if err != nil || second != out {
		t.Fatalf("post-rendering is not deterministic: err=%v", err)
	}
}

func TestTransformRejects(t *testing.T) {
	tests := map[string]string{
		"controller source changed":         strings.Replace(validManifest, controllerTag, "ghcr.io/flavio/podlock/controller:v9", 1),
		"nri source changed":                strings.Replace(validManifest, nriTag, "ghcr.io/flavio/podlock/nri:v9", 1),
		"controller workload absent":        strings.Replace(validManifest, "name: podlock-controller", "name: other", 1),
		"nri workload absent":               strings.Replace(validManifest, "name: podlock-nri-plugin", "name: other", 1),
		"container absent":                  strings.Replace(validManifest, "name: controller", "name: other", 1),
		"duplicate container":               strings.Replace(validManifest, "- name: third-party\n          image: example.com/third-party:1", "- name: controller\n          image: ghcr.io/flavio/podlock/controller:v0.1.0\n        - name: third-party\n          image: example.com/third-party:1", 1),
		"unexpected mutable workload image": validManifest + "\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: extra}\nspec:\n  template:\n    spec:\n      containers:\n        - name: extra\n          image: ghcr.io/flavio/podlock/nri:v0.1.0\n",
		"malformed YAML":                    "kind: [",
		"unexpected digest":                 strings.Replace(validManifest, controllerTag, "ghcr.io/flavio/podlock/controller@sha256:bad", 1),
		"wrong resource digest":             strings.Replace(validManifest, "name: third-party\n          image: example.com/third-party:1", "name: third-party\n          image: "+controllerPin, 1),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := runManifest(t, input); err == nil {
				t.Fatal("expected fail-closed error")
			}
		})
	}
}

func TestTransformIgnoresNonImageConfiguration(t *testing.T) {
	input := validManifest + "\n---\nkind: ConfigMap\nmetadata: {name: extra}\ndata: {image: ghcr.io/flavio/podlock/nri:v0.1.0}\n"
	out, err := runManifest(t, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "'"+nriTag+"'") {
		t.Fatal("non-image configuration value was unexpectedly rewritten")
	}
}
