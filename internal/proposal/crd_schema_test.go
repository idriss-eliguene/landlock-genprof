package proposal

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestSecurityProfileProposalStatusSchemaParity(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	paths := []string{
		filepath.Join(repoRoot, "deploy/crd-securityprofileproposal.yaml"),
		filepath.Join(repoRoot, "deploy/helm/landlock-genprof/crds/crd-securityprofileproposal.yaml"),
	}

	var schemas []map[string]interface{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var document map[string]interface{}
		if err := yaml.Unmarshal(data, &document); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		schemas = append(schemas, statusSchema(t, document, path))
	}

	statusFields := map[string]bool{}
	typ := reflect.TypeOf(Status{})
	for i := 0; i < typ.NumField(); i++ {
		jsonName := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			continue
		}
		statusFields[jsonName] = true
	}

	for i, properties := range schemas {
		if len(properties) != len(statusFields) {
			t.Fatalf("schema %d status fields = %v, runtime fields = %v", i, mapKeys(properties), mapKeysBool(statusFields))
		}
		for field := range statusFields {
			if _, ok := properties[field]; !ok {
				t.Fatalf("schema %d missing runtime status field %q", i, field)
			}
		}
	}

	if !reflect.DeepEqual(schemas[0], schemas[1]) {
		t.Fatalf("root and Helm SecurityProfileProposal status schemas differ:\nroot=%v\nhelm=%v", schemas[0], schemas[1])
	}
}

func statusSchema(t *testing.T, document map[string]interface{}, path string) map[string]interface{} {
	t.Helper()
	spec, ok := document["spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: CRD spec missing", path)
	}
	versions, ok := spec["versions"].([]interface{})
	if !ok || len(versions) == 0 {
		t.Fatalf("%s: CRD versions missing", path)
	}
	version, ok := versions[0].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: malformed CRD version", path)
	}
	if version["name"] != "v1alpha1" {
		t.Fatalf("%s: unexpected CRD version %v", path, version["name"])
	}
	subresources, ok := version["subresources"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: subresources missing", path)
	}
	if _, ok := subresources["status"]; !ok {
		t.Fatalf("%s: status subresource missing", path)
	}
	schema, ok := version["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: OpenAPI schema missing", path)
	}
	openAPI, ok := schema["openAPIV3Schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: openAPIV3Schema missing", path)
	}
	properties, ok := openAPI["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: root properties missing", path)
	}
	status, ok := properties["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: status schema missing", path)
	}
	statusProperties, ok := status["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: status properties missing", path)
	}
	return statusProperties
}

func mapKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func mapKeysBool(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
