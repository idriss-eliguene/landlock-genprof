// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
	"github.com/idriss-eliguene/landlock-genprof/pkg/spo"
)

// workbenchView is a display projection only. It contains no authorization,
// digest, policy, or mutation logic of its own; those remain in proposal and
// the existing artifact packages.
type workbenchView struct {
	Namespace       string
	Proposal        string
	Container       string
	Binary          string
	GeneratedAt     string
	CandidateDigest string
	Lifecycle       string
	Approval        string
	ApprovalReason  string
	ApprovedDigest  string
	ApprovalVersion string
	ApprovalUpdated string
	ApprovalBinding string
	ReadAt          string
	Domains         []workbenchDomain
	Provenance      []string
	Application     string
	Verification    string
	Boundaries      []string
}

type workbenchDomain struct {
	Name         string
	Candidate    string
	Availability string
	Provenance   string
	ReviewState  string
	artifact     string
}

type workbenchOptions struct {
	namespace string
	port      int
}

const workbenchReadHeaderTimeout = 5 * time.Second

func newWorkbenchCmd() *cobra.Command {
	var opts workbenchOptions

	cmd := &cobra.Command{
		Use:   "ui <proposal>",
		Short: "Serves an experimental local read-only proposal review page",
		Long: "Serves one SecurityProfileProposal as an experimental, local, read-only " +
			"review page. It reads the proposal through the existing Kubernetes API path; " +
			"it has no approval, rejection, or apply controls." + kubectlPrefixNote,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkbench(cmd.Context(), cmd.OutOrStdout(), opts, args[0])
		},
	}
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().IntVar(&opts.port, "port", 8080, "Loopback HTTP port")
	return cmd
}

func runWorkbench(ctx context.Context, stdout io.Writer, opts workbenchOptions, proposalName string) error {
	client, err := newDynamicClientForWorkbench()
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	view, err := loadWorkbenchView(ctx, client, opts.namespace, proposalName)
	if err != nil {
		return err
	}

	addr := workbenchListenAddress(opts.port)
	server := &http.Server{
		Addr:              addr,
		Handler:           newWorkbenchHandler(view),
		ReadHeaderTimeout: workbenchReadHeaderTimeout,
	}
	fmt.Fprintf(stdout, "Experimental local Workbench: http://%s\n", addr)
	fmt.Fprintln(stdout, "Read-only: approval and application remain CLI operations.")
	return server.ListenAndServe()
}

func loadWorkbenchView(ctx context.Context, client dynamic.Interface, namespace, name string) (workbenchView, error) {
	spec, err := proposal.Get(ctx, client, namespace, name)
	if err != nil {
		return workbenchView{}, err
	}
	if spec == nil {
		return workbenchView{}, fmt.Errorf("securityprofileproposal %s/%s not found", namespace, name)
	}
	status, err := proposal.GetStatus(ctx, client, namespace, name)
	if err != nil {
		return workbenchView{}, err
	}
	digest, err := proposal.CandidateDigest(*spec)
	if err != nil {
		return workbenchView{}, fmt.Errorf("computing candidate digest for Workbench: %w", err)
	}

	approval := "UNKNOWN — status unavailable"
	reason := ""
	approvedDigest := "NOT_AVAILABLE — no approved candidate is recorded"
	approvalVersion := "NOT_AVAILABLE"
	approvalUpdated := "NOT_AVAILABLE"
	approvalBinding := workbenchApprovalBinding(spec, status)
	if status != nil {
		approval = string(status.ApprovalState)
		reason = status.Reason
		if status.ApprovedCandidateDigest != "" {
			approvedDigest = status.ApprovedCandidateDigest
		}
		if status.ApprovalMechanismVersion != "" {
			approvalVersion = status.ApprovalMechanismVersion
		}
		if status.UpdatedAt != "" {
			approvalUpdated = status.UpdatedAt
		}
	}

	domains := summarizeWorkbenchDomains(*spec)
	provenance := workbenchProvenance(*spec)
	return workbenchView{
		Namespace:       namespace,
		Proposal:        name,
		Container:       spec.Container,
		Binary:          spec.Binary,
		GeneratedAt:     spec.GeneratedAt,
		CandidateDigest: digest,
		Lifecycle:       "PROPOSAL — structured snapshot",
		Approval:        approval,
		ApprovalReason:  reason,
		ApprovedDigest:  approvedDigest,
		ApprovalVersion: approvalVersion,
		ApprovalUpdated: approvalUpdated,
		ApprovalBinding: approvalBinding,
		ReadAt:          time.Now().UTC().Format(time.RFC3339),
		Domains:         domains,
		Provenance:      append(provenance, workbenchSeccompProvenance(*spec)...),
		Application:     "NOT_AVAILABLE — application outcome is not persisted in SecurityProfileProposal",
		Verification:    "NOT_AVAILABLE — behavioral verification is not persisted in SecurityProfileProposal",
		Boundaries: []string{
			"The candidate view is not a current-to-proposed comparison: live current configuration is NOT_AVAILABLE here.",
			"SPO SeccompProfile content is DERIVED POLICY, not direct syscall evidence.",
			"Coverage is informational, not confidence or authorization.",
			"Observed is not automatically legitimate; not observed is not unnecessary.",
			"API application is not enforcement evidence or behavioral verification.",
			"This local page does not establish universal compatibility.",
		},
	}, nil
}

func workbenchApprovalBinding(spec *proposal.Spec, status *proposal.Status) string {
	err := proposal.ValidateApprovedCandidate(spec, status)
	if err == nil {
		return "BOUND — approved digest validates against the current candidate"
	}
	if status == nil {
		return "NOT BOUND / RE-APPROVAL REQUIRED — no approval is recorded"
	}
	return "NOT BOUND / RE-APPROVAL REQUIRED — " + err.Error()
}

func workbenchProvenance(spec proposal.Spec) []string {
	provenance := []string{
		"CONFIGURED STATE: NOT_AVAILABLE in the proposal snapshot",
		"DIRECT EVIDENCE: NOT_AVAILABLE in the proposal snapshot",
		"Generated artifact content: STRUCTURED candidate data",
	}
	return provenance
}

func workbenchSeccompProvenance(spec proposal.Spec) []string {
	if strings.TrimSpace(spec.SPOSeccompProfile) == "" {
		return nil
	}
	prov, ok := parseSeccompProvenance(spec.SPOSeccompProfile)
	if !ok {
		return []string{"SECCOMP PROVENANCE: UNKNOWN — source metadata is unavailable"}
	}
	lines := []string{"SECCOMP: " + seccompSourceClassification(prov.source)}
	for _, line := range seccompProvenanceLines(spec.SPOSeccompProfile) {
		if line == "Seccomp:" {
			continue
		}
		lines = append(lines, "  "+line)
	}
	return lines
}

func annotationOrUnknown(annotations map[string]string, key string) string {
	if value := annotations[key]; value != "" {
		return value
	}
	return "unknown"
}

func summarizeWorkbenchDomains(spec proposal.Spec) []workbenchDomain {
	domains := []workbenchDomain{
		{Name: "Filesystem / Landlock", artifact: spec.PodLock, Provenance: "DERIVED POLICY / candidate artifact"},
		{Name: "NetworkPolicy", artifact: spec.NetworkPolicy, Provenance: "DERIVED POLICY / candidate artifact"},
		{Name: "SPO SeccompProfile", artifact: spec.SPOSeccompProfile},
		{Name: "SecurityContext binding", artifact: spec.PatchedManifest, Provenance: "DERIVED POLICY / proposed binding artifact"},
	}

	for i := range domains {
		domains[i].Provenance = workbenchDomainProvenance(domains[i].Name, domains[i].artifact)
		domains[i].ReviewState = "REVIEW REQUIRED"
		if domains[i].artifact == "" {
			domains[i].Availability = "NOT_AVAILABLE — artifact not present"
			domains[i].Candidate = "NOT_AVAILABLE"
			continue
		}
		domains[i].Availability = "AVAILABLE — structured candidate artifact"
		domains[i].Candidate = summarizeWorkbenchArtifact(domains[i].Name, domains[i].artifact)
	}
	return domains
}

func workbenchDomainProvenance(name, content string) string {
	if name != "SPO SeccompProfile" {
		if content == "" {
			return "NOT_AVAILABLE"
		}
		return "DERIVED POLICY / candidate artifact"
	}
	if strings.TrimSpace(content) == "" {
		return "NOT_AVAILABLE"
	}
	prov, ok := parseSeccompProvenance(content)
	if !ok {
		return "UNKNOWN — seccomp source provenance unavailable"
	}
	return seccompSourceClassification(prov.source)
}

func summarizeWorkbenchArtifact(name, content string) string {
	switch name {
	case "Filesystem / Landlock":
		var profile podlock.LandlockProfile
		if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
			return "UNKNOWN — candidate artifact could not be parsed"
		}
		return "STRUCTURED candidate artifact"
	case "NetworkPolicy":
		var policy networkingv1.NetworkPolicy
		if err := yaml.Unmarshal([]byte(content), &policy); err != nil {
			return "UNKNOWN — candidate artifact could not be parsed"
		}
		return "STRUCTURED candidate artifact"
	case "SPO SeccompProfile":
		var profile spo.SeccompProfile
		if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
			return "UNKNOWN — candidate artifact could not be parsed"
		}
		prov, ok := parseSeccompProvenance(content)
		if !ok {
			return "STRUCTURED candidate artifact; source unknown"
		}
		if prov.source == spobackend.SeccompSourceSPO {
			return "STRUCTURED derived policy artifact"
		}
		if prov.source == spobackend.SeccompSourceInternal {
			return "STRUCTURED internal observation artifact"
		}
		return "STRUCTURED candidate artifact; source unknown"
	case "SecurityContext binding":
		var manifest unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(content), &manifest.Object); err != nil {
			return "UNKNOWN — candidate artifact could not be parsed"
		}
		if _, found, _ := unstructured.NestedMap(manifest.Object, "spec", "containers"); found {
			return "STRUCTURED candidate: proposed workload binding manifest present"
		}
		return "STRUCTURED candidate: binding artifact present; container details unavailable"
	default:
		return "STRUCTURED candidate artifact present"
	}
}

func workbenchListenAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

var workbenchPage = template.Must(template.New("workbench").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Experimental Workbench — {{.Proposal}}</title>
<style>
:root{color-scheme:light;--ink:#1f2933;--muted:#52606d;--line:#d9e2ec;--panel:#f5f7fa;--accent:#245b75;--warn:#8a5a00}
body{margin:0;background:#fff;color:var(--ink);font:16px/1.45 system-ui,-apple-system,sans-serif}
main{max-width:1100px;margin:0 auto;padding:28px 22px 56px}h1{margin:0 0 4px;font-size:30px}h2{margin:28px 0 12px;font-size:20px;color:var(--accent)}
.eyebrow{color:var(--muted);font-size:13px;letter-spacing:.08em;text-transform:uppercase}.meta{display:flex;flex-wrap:wrap;gap:8px 22px;color:var(--muted);margin:8px 0 20px}.digest{font-family:ui-monospace,monospace;overflow-wrap:anywhere;background:var(--panel);border:1px solid var(--line);padding:12px;border-radius:6px}
.state-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.card{border:1px solid var(--line);border-radius:7px;padding:14px;background:#fff}.card strong{display:block;margin-bottom:5px}.state{font-weight:650}.unknown{color:var(--warn)}
table{border-collapse:collapse;width:100%;font-size:14px}th,td{text-align:left;vertical-align:top;border-bottom:1px solid var(--line);padding:10px 8px}th{background:var(--panel)}code{font-family:ui-monospace,monospace}.notice{border-left:4px solid var(--warn);background:#fff8e1;padding:12px 14px}.boundary{margin:6px 0;color:var(--muted)}
@media(max-width:650px){main{padding:20px 14px}table{display:block;overflow-x:auto;white-space:normal}}
</style></head>
<body><main>
<div class="eyebrow">Experimental · Local / Read-only</div>
<h1>SecurityProfileProposal review</h1>
<div class="meta"><span><strong>Proposal:</strong> {{.Namespace}}/{{.Proposal}}</span><span><strong>Container:</strong> {{.Container}}</span><span><strong>Binary:</strong> {{.Binary}}</span><span><strong>Generated:</strong> {{.GeneratedAt}}</span></div>
<div class="notice">This page is a snapshot read at {{.ReadAt}}. Restart the command to refresh cluster state. It is a presentation surface only: approval and application remain explicit CLI operations. Application and behavioral verification are not recorded in the canonical proposal state.</div>
<h2>Exact candidate identity</h2><div class="digest"><strong>Candidate digest</strong><br>{{.CandidateDigest}}</div>
<h2>Lifecycle</h2><div class="state-grid"><div class="card"><strong>Proposal</strong><span class="state">{{.Lifecycle}}</span></div><div class="card"><strong>Application</strong><span class="state unknown">{{.Application}}</span></div><div class="card"><strong>Enforcement evidence</strong><span class="state unknown">NOT_AVAILABLE — no enforcement evidence is persisted</span></div><div class="card"><strong>Behavioral verification</strong><span class="state unknown">{{.Verification}}</span></div></div>
<h2>Candidate authority / policy</h2><p class="boundary">This is the structured candidate contained in the proposal. It is not a current-to-proposed delta because live current configuration is not available here.</p><table><thead><tr><th>Domain</th><th>Candidate</th><th>Availability</th><th>Provenance</th><th>Reviewer action</th></tr></thead><tbody>{{range .Domains}}<tr><td><strong>{{.Name}}</strong></td><td>{{.Candidate}}</td><td class="unknown">{{.Availability}}</td><td>{{.Provenance}}</td><td>{{.ReviewState}}</td></tr>{{end}}</tbody></table>
<h2>Evidence & provenance</h2>{{range .Provenance}}<div class="boundary">{{.}}</div>{{end}}
<h2>Authorization</h2><div class="state-grid"><div class="card"><strong>Approval state</strong><span class="state">{{.Approval}}</span>{{if .ApprovalReason}}<br><span>{{.ApprovalReason}}</span>{{end}}</div><div class="card"><strong>Approval binding</strong><span class="state unknown">{{.ApprovalBinding}}</span></div><div class="card"><strong>Approved candidate digest</strong><code>{{.ApprovedDigest}}</code></div><div class="card"><strong>Approval mechanism</strong><span>{{.ApprovalVersion}}</span></div><div class="card"><strong>Approval updated</strong><span>{{.ApprovalUpdated}}</span></div></div>
<h2>Unsupported / unknown boundaries</h2>{{range .Boundaries}}<div class="boundary">{{.}}</div>{{end}}
</main></body></html>`))

func newWorkbenchHandler(view workbenchView) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := workbenchPage.Execute(w, view); err != nil {
			// The template is parsed at package initialization. A write error
			// is reported to the server, never converted into a mutation.
			return
		}
	})
}

var newDynamicClientForWorkbench = newDynamicClient
