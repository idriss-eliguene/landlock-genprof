package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestG8CanonicalShellHasOneSurfaceAndSemanticTokens(t *testing.T) {
	var body bytes.Buffer
	if err := workbenchClusterPageTemplate.Execute(&body, workbenchClusterView{Namespace: "team-a"}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, token := range []string{
		"--surface:", "--surface-subtle:", "--surface-raised:", "--nav:", "--nav-hover:", "--nav-selected:",
		"--text:", "--text-muted:", "--text-inverse:", "--border:", "--border-strong:", "--primary:",
		"--primary-hover:", "--success:", "--success-surface:", "--warning:", "--warning-surface:",
		"--danger:", "--danger-surface:", "--unknown:", "--unknown-surface:",
	} {
		if !strings.Contains(page, token) {
			t.Errorf("canonical shell missing token %q", token)
		}
	}
	for _, responsive := range []string{
		"@media(max-width:1100px)",
		".attention-item{display:grid",
		"background:var(--surface);overflow-wrap:anywhere}",
		".attention-item{grid-template-columns:repeat(2,minmax(0,1fr))}",
		"@media(max-width:700px)",
		".attention-item{grid-template-columns:1fr}",
	} {
		if !strings.Contains(page, responsive) {
			t.Errorf("canonical shell missing responsive Attention rule %q", responsive)
		}
	}
	for _, required := range []string{
		"class=\"app-shell\"", "class=\"sidebar\"", "class=\"topbar\"", "class=\"content\"",
		"id=\"observation-workbench\"", "id=\"operations-context\"", "id=\"overview-view\"",
		"id=\"workloads-view\"", "id=\"observations-view\"", "id=\"proposals-view\"",
		"id=\"history-view\"", "id=\"attention-view\"",
		".technical{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}",
	} {
		if !strings.Contains(page, required) {
			t.Errorf("canonical shell missing %q", required)
		}
	}
	for _, legacy := range []string{"Workload navigation", "Runtime subject / provenance", "Exact CLI next steps", "Initial proposal context"} {
		if strings.Contains(page, legacy) {
			t.Errorf("legacy duplicate surface remains: %q", legacy)
		}
	}
}

func TestG8WorkbenchScriptHasNavigationStateAndVisibleSafetyFeedback(t *testing.T) {
	w := httptest.NewRecorder()
	handleWorkbenchScript(w, httptest.NewRequest(http.MethodGet, "/workbench.js", nil))
	script := w.Body.String()
	for _, required := range []string{
		"setAttribute(\"aria-current\", \"page\")", "contextChip(\"Cluster\"", "contextChip(\"Namespace\"",
		"contextChip(\"Platform\"", "contextChip(\"Projection\"", "contextChip(\"Read time\"",
		"authoritative = lastContext || body || {}", "document.querySelectorAll(\"[data-view]\")",
		"The previous decision was not applied", "NOT_AUTHORIZED", "NOT_SEMANTICALLY_ELIGIBLE",
		"STALE — canonical resourceVersion is unavailable", "Projection DEGRADED", "NOT_ELIGIBLE",
		"SUCCESS", "PARTIAL", "FAILED", "UNKNOWN", "History / custody", "BINDING_INVALID",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("G8 script missing %q", required)
		}
	}
	if strings.Contains(script, "app.querySelectorAll(\"[data-view]\")") {
		t.Fatal("navigation listeners must include the canonical sidebar outside the content app")
	}
	for _, forbidden := range []string{"innerHTML", "cluster-selector", "namespace-selector", "kubeconfig", "Authorization:", "Rollback</button>"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("G8 script contains forbidden construct %q", forbidden)
		}
	}
}

func TestG8PrimaryNavigationIsExact(t *testing.T) {
	var body bytes.Buffer
	if err := workbenchClusterPageTemplate.Execute(&body, workbenchClusterView{Namespace: "team-a"}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, item := range []string{"Overview", "Workloads", "Observations", "Proposals", "History", "Attention"} {
		if strings.Count(page, `data-view="`+strings.ToLower(item)+`"`) != 1 {
			t.Errorf("navigation item %q is not present exactly once", item)
		}
	}
	if strings.Count(page, `id="refresh-operations-context"`) != 1 || strings.Contains(page, `id="refresh-workbench"`) {
		t.Fatal("refresh controls are not consolidated into the canonical context refresh")
	}
}
