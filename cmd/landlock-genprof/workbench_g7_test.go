package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestG7WorkbenchScriptPreservesCertifiedBoundaries(t *testing.T) {
	w := httptest.NewRecorder()
	handleWorkbenchScript(w, httptest.NewRequest(http.MethodGet, "/workbench.js", nil))
	script := w.Body.String()
	for _, required := range []string{
		"/api/v08/operations-context", "/api/workloads", "/api/observations", "/api/proposals", "/api/v08/environment", "/api/v08/history",
		"capabilitiesLoaded", "expectedResourceVersion", "proposal.review", "proposal.approve", "proposal.apply",
		"State changed since this decision was loaded", "The previous decision was not applied", "NOT_ELIGIBLE", "Unknown / Unbound",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("G7 script missing %q", required)
		}
	}
	for _, forbidden := range []string{"PATCH", "/status", "innerHTML", "kubeconfig", "Authorization:"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("G7 script contains forbidden authority/UI construct %q", forbidden)
		}
	}
}

func TestG7ClusterTemplatePrimaryNavigationAndContext(t *testing.T) {
	var body bytes.Buffer
	view := workbenchClusterView{Namespace: "team-a"}
	if err := workbenchClusterPageTemplate.Execute(&body, view); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, required := range []string{
		`aria-label="Primary navigation"`, `data-view="overview"`, `data-view="workloads"`, `data-view="observations"`,
		`data-view="proposals"`, `data-view="history"`, `data-view="attention"`, `id="operations-context"`,
		`data-view="governance"`, `id="refresh-operations-context"`, `id="cluster-selector"`, `id="namespace-selector"`, `data-namespace="team-a"`, "Observations / Evidence",
	} {
		if !strings.Contains(page, required) {
			t.Errorf("cluster template missing G7 surface %q", required)
		}
	}
}
