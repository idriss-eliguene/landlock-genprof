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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/projection"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/workload"
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

type workbenchClusterView struct {
	Namespace string
	Proposal  workbenchView
	Workloads []workbenchNavigationWorkload
	Selected  *workbenchSelectedTarget
	NextSteps []string
}

type workbenchNavigationWorkload struct {
	Target     k8s.WorkloadRef
	Owner      string
	OwnerNote  string
	Containers []workbenchNavigationContainer
}

type workbenchNavigationContainer struct {
	Name         string
	Category     string
	Supported    bool
	RuntimeState string
	Target       *k8s.GovernedTarget
	Link         string
}

type workbenchSelectedTarget struct {
	Target          k8s.GovernedTarget
	RuntimeSubjects []k8s.RuntimeSubject
	Projection      dtoProjection
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
		Short: "Serves the local read-only Workbench HTTP boundary",
		Long: "Serves the local, read-only Workbench: the given SecurityProfileProposal at " +
			"\"/\", plus live workload/security-projection reads under \"/api\". Every read " +
			"goes through the bounded G0.5 read capability; there is no approval, rejection, " +
			"apply, or other mutation control." + kubectlPrefixNote,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkbench(cmd.Context(), cmd.OutOrStdout(), opts, args[0])
		},
	}
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().IntVar(&opts.port, "port", 8080, "Loopback HTTP port")
	return cmd
}

// newWorkbenchReadSession is a test seam, same pattern as this package's
// other newDynamicClientFor* seams. Unlike those, its return type is the
// bounded k8s.WorkbenchReadCapability, not a write-capable client: the
// Workbench HTTP server must not be able to hold one.
var newWorkbenchReadSession = func(namespace string) (k8s.WorkbenchReadCapability, error) {
	config, err := k8s.RestConfig()
	if err != nil {
		return nil, err
	}
	return k8s.NewReadSession(config, namespace)
}

// workbenchShutdownTimeout bounds how long a graceful shutdown waits for
// in-flight requests to drain before the process exits regardless.
const workbenchShutdownTimeout = 5 * time.Second

func runWorkbench(ctx context.Context, stdout io.Writer, opts workbenchOptions, proposalName string) error {
	reads, err := newWorkbenchReadSession(opts.namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	handler, err := newWorkbenchServer(reads, proposalName, opts.port)
	if err != nil {
		return fmt.Errorf("constructing Workbench server: %w", err)
	}

	addr := workbenchListenAddress(opts.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding Workbench listener on %s: %w", addr, err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: workbenchReadHeaderTimeout,
		ReadTimeout:       workbenchReadTimeout,
		WriteTimeout:      workbenchWriteTimeout,
		IdleTimeout:       workbenchIdleTimeout,
		MaxHeaderBytes:    workbenchMaxHeaderBytes,
	}
	fmt.Fprintf(stdout, "Local Workbench: http://%s\n", addr)
	fmt.Fprintln(stdout, "Read-only: approval and application remain CLI operations.")

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("Workbench listener: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), workbenchShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down Workbench: %w", err)
		}
		<-serveErr
		return ctx.Err()
	}
}

// loadWorkbenchView performs one bounded live read of the named proposal
// through the read capability. It is called per request, not once at
// startup: G3 intentionally supersedes the v0.4 fixed-startup-snapshot
// contract with request-triggered bounded live reads (#185).
func loadWorkbenchView(ctx context.Context, reads k8s.WorkbenchReadCapability, name string) (workbenchView, error) {
	namespace := reads.SessionIdentity().Namespace
	obj, err := reads.GetProposal(ctx, name)
	if err != nil {
		return workbenchView{}, err
	}
	spec, err := decodeProposalSpec(obj)
	if err != nil {
		return workbenchView{}, err
	}
	status, err := decodeProposalStatus(obj)
	if err != nil {
		return workbenchView{}, err
	}
	digest, err := proposal.CandidateDigest(spec)
	if err != nil {
		return workbenchView{}, fmt.Errorf("computing candidate digest for Workbench: %w", err)
	}

	approval := "UNKNOWN — status unavailable"
	reason := ""
	approvedDigest := "NOT_AVAILABLE — no approved candidate is recorded"
	approvalVersion := "NOT_AVAILABLE"
	approvalUpdated := "NOT_AVAILABLE"
	approvalBinding := workbenchApprovalBinding(&spec, status)
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

	domains := summarizeWorkbenchDomains(spec)
	provenance := workbenchProvenance(spec)
	return workbenchView{
		Namespace:       namespace,
		Proposal:        name,
		Container:       spec.Container,
		Binary:          spec.Binary,
		GeneratedAt:     spec.GeneratedAt,
		CandidateDigest: digest,
		Lifecycle:       "PROPOSAL — structured candidate",
		Approval:        approval,
		ApprovalReason:  reason,
		ApprovedDigest:  approvedDigest,
		ApprovalVersion: approvalVersion,
		ApprovalUpdated: approvalUpdated,
		ApprovalBinding: approvalBinding,
		ReadAt:          time.Now().UTC().Format(time.RFC3339),
		Domains:         domains,
		Provenance:      append(provenance, workbenchSeccompProvenance(spec)...),
		Application:     "NOT_AVAILABLE — application outcome is not persisted in SecurityProfileProposal",
		Verification:    "NOT_AVAILABLE — behavioral verification is not persisted in SecurityProfileProposal",
		Boundaries: []string{
			"This is a live read, performed for this request; it is not cached and does not require a restart to reflect cluster changes.",
			"The candidate view is not a current-to-proposed comparison: live current configuration is NOT_AVAILABLE here.",
			"SPO-sourced SeccompProfile content is DERIVED POLICY, not direct landlock-genprof syscall evidence.",
			"Coverage is informational, not confidence or authorization.",
			"Observed is not automatically legitimate; not observed is not unnecessary.",
			"API application is not enforcement evidence or behavioral verification.",
			"This local page does not establish universal compatibility.",
		},
	}, nil
}

func workbenchClusterPage(ctx context.Context, reads k8s.WorkbenchReadCapability, proposalName string, selector *targetSelector) (workbenchClusterView, error) {
	proposalView, err := loadWorkbenchView(ctx, reads, proposalName)
	if err != nil {
		return workbenchClusterView{}, err
	}
	discovery, err := workload.NewService(reads)
	if err != nil {
		return workbenchClusterView{}, err
	}
	result, err := discovery.Discover(ctx)
	if err != nil {
		return workbenchClusterView{}, err
	}
	page := workbenchClusterView{Namespace: result.Namespace, Proposal: proposalView}
	for _, item := range result.Workloads {
		navigation := workbenchNavigationWorkload{Target: item.Target, Owner: string(item.Owner), OwnerNote: item.OwnerNote}
		for _, pod := range item.Pods {
			for _, container := range pod.Containers {
				entry := workbenchNavigationContainer{Name: container.Name, Category: string(container.Category), Supported: container.SupportedTarget, RuntimeState: string(container.RuntimeState), Target: container.Target}
				if container.Target != nil && container.SupportedTarget {
					entry.Link = workbenchTargetLink(*container.Target)
				}
				navigation.Containers = append(navigation.Containers, entry)
			}
		}
		page.Workloads = append(page.Workloads, navigation)
	}
	if selector == nil {
		return page, nil
	}
	target, item, subjects, found := resolveGovernedTarget(result, *selector)
	if !found {
		return workbenchClusterView{}, &workbenchTargetNotFoundError{target: *selector}
	}
	projector, err := projection.NewService(reads)
	if err != nil {
		return workbenchClusterView{}, err
	}
	projected, err := projector.Project(ctx, target, item, projection.Inputs{
		RuntimeSubjects: subjects,
	})
	if err != nil {
		return workbenchClusterView{}, err
	}
	page.Selected = &workbenchSelectedTarget{Target: target, RuntimeSubjects: subjects, Projection: dtoFromProjection(projected)}
	page.NextSteps = workbenchNextSteps(proposalName, result.Namespace, proposalView.CandidateDigest, proposalView.ApprovalBinding)
	return page, nil
}

func workbenchTargetLink(target k8s.GovernedTarget) string {
	query := url.Values{}
	query.Set("group", target.Workload.Group)
	query.Set("kind", target.Workload.Kind)
	query.Set("name", target.Workload.Name)
	query.Set("container", target.Container)
	return "?" + query.Encode()
}

// shellQuote renders one arbitrary value as exactly one POSIX shell word.
// The single-quote encoding leaves every byte literal; an embedded single
// quote closes the quoted word, emits a literal quote, and reopens it.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func workbenchNextSteps(proposalName, namespace, digest, binding string) []string {
	steps := []string{}
	if !strings.HasPrefix(binding, "BOUND —") {
		steps = append(steps, fmt.Sprintf("kubectl landlock-genprof approve %s -n %s --expected-digest %s", shellQuote(proposalName), shellQuote(namespace), shellQuote(digest)))
	}
	steps = append(steps, fmt.Sprintf("kubectl landlock-genprof apply-proposal %s -n %s", shellQuote(proposalName), shellQuote(namespace)))
	return steps
}

// decodeProposalSpec and decodeProposalStatus decode the same way
// internal/projection does: unstructured.NestedMap plus
// runtime.DefaultUnstructuredConverter.FromUnstructured. That keeps
// workbench.go on the bounded k8s.WorkbenchReadCapability instead of
// internal/proposal's dynamic.Interface-based Get/GetStatus, which the
// Workbench HTTP server must not hold.
func decodeProposalSpec(obj *unstructured.Unstructured) (proposal.Spec, error) {
	value, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return proposal.Spec{}, fmt.Errorf("reading spec from SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if !found {
		return proposal.Spec{}, nil
	}
	var spec proposal.Spec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, &spec); err != nil {
		return proposal.Spec{}, fmt.Errorf("converting spec from SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return spec, nil
}

func decodeProposalStatus(obj *unstructured.Unstructured) (*proposal.Status, error) {
	value, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil {
		return nil, fmt.Errorf("reading status from SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if !found {
		return nil, nil
	}
	var status proposal.Status
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, &status); err != nil {
		return nil, fmt.Errorf("converting status from SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return &status, nil
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
<div class="notice">This page reflects a live read performed at {{.ReadAt}}; reload to read the cluster again. It is a presentation surface only: approval and application remain explicit CLI operations. Application and behavioral verification are not recorded in the canonical proposal state.</div>
<h2>Exact candidate identity</h2><div class="digest"><strong>Candidate digest</strong><br>{{.CandidateDigest}}</div>
<h2>Lifecycle</h2><div class="state-grid"><div class="card"><strong>Proposal</strong><span class="state">{{.Lifecycle}}</span></div><div class="card"><strong>Application</strong><span class="state unknown">{{.Application}}</span></div><div class="card"><strong>Enforcement evidence</strong><span class="state unknown">NOT_AVAILABLE — no enforcement evidence is persisted</span></div><div class="card"><strong>Behavioral verification</strong><span class="state unknown">{{.Verification}}</span></div></div>
<h2>Candidate authority / policy</h2><p class="boundary">This is the structured candidate contained in the proposal. It is not a current-to-proposed delta because live current configuration is not available here.</p><table><thead><tr><th>Domain</th><th>Candidate</th><th>Availability</th><th>Provenance</th><th>Reviewer action</th></tr></thead><tbody>{{range .Domains}}<tr><td><strong>{{.Name}}</strong></td><td>{{.Candidate}}</td><td class="unknown">{{.Availability}}</td><td>{{.Provenance}}</td><td>{{.ReviewState}}</td></tr>{{end}}</tbody></table>
<h2>Evidence & provenance</h2>{{range .Provenance}}<div class="boundary">{{.}}</div>{{end}}
<h2>Authorization</h2><div class="state-grid"><div class="card"><strong>Approval state</strong><span class="state">{{.Approval}}</span>{{if .ApprovalReason}}<br><span>{{.ApprovalReason}}</span>{{end}}</div><div class="card"><strong>Approval binding</strong><span class="state unknown">{{.ApprovalBinding}}</span></div><div class="card"><strong>Approved candidate digest</strong><code>{{.ApprovedDigest}}</code></div><div class="card"><strong>Approval mechanism</strong><span>{{.ApprovalVersion}}</span></div><div class="card"><strong>Approval updated</strong><span>{{.ApprovalUpdated}}</span></div></div>
<h2>Unsupported / unknown boundaries</h2>{{range .Boundaries}}<div class="boundary">{{.}}</div>{{end}}
</main></body></html>`))

var workbenchClusterPageTemplate = template.Must(template.New("cluster-workbench").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cluster Workbench — {{.Namespace}}</title><style>
:root{color-scheme:light;--ink:#1f2933;--muted:#52606d;--line:#d9e2ec;--panel:#f5f7fa;--accent:#245b75;--warn:#8a5a00}
body{margin:0;background:#fff;color:var(--ink);font:16px/1.45 system-ui,-apple-system,sans-serif}main{max-width:1200px;margin:0 auto;padding:28px 22px 56px}h1{margin:0 0 4px;font-size:30px}h2{margin:28px 0 12px;font-size:20px;color:var(--accent)}h3{margin:18px 0 8px}.eyebrow{color:var(--muted);font-size:13px;letter-spacing:.08em;text-transform:uppercase}.meta{display:flex;flex-wrap:wrap;gap:8px 22px;color:var(--muted);margin:8px 0 20px}.panel{border:1px solid var(--line);border-radius:7px;padding:14px;background:#fff;margin:12px 0}.state{font-weight:650}.unknown{color:var(--warn)}.muted{color:var(--muted)}.workloads{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:12px}.workload{border:1px solid var(--line);border-radius:7px;padding:14px;background:var(--panel)}.workload ul{margin:8px 0 0;padding-left:20px}.workload a{color:var(--accent)}table{border-collapse:collapse;width:100%;font-size:14px}th,td{text-align:left;vertical-align:top;border-bottom:1px solid var(--line);padding:10px 8px}th{background:var(--panel)}code{font-family:ui-monospace,monospace;overflow-wrap:anywhere}.notice{border-left:4px solid var(--warn);background:#fff8e1;padding:12px 14px}.section-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:12px}.boundary{margin:6px 0;color:var(--muted)}@media(max-width:650px){main{padding:20px 14px}table{display:block;overflow-x:auto}}
</style></head><body><main>
<div class="eyebrow">Experimental · Local / Read-only</div><h1>Cluster Workbench</h1>
<div class="meta"><span><strong>Namespace:</strong> {{.Namespace}}</span><span><strong>Initial proposal:</strong> {{.Namespace}}/{{.Proposal.Proposal}}</span></div>
<div class="notice">This page reflects a live read performed at {{.Proposal.ReadAt}}; reload to read the cluster again. It presents bounded live reads and is a presentation surface only: browser actions do not approve, apply, reject, revoke, or otherwise mutate the cluster.</div>
<h2>Workload navigation</h2>
{{if .Workloads}}<div class="workloads">{{range .Workloads}}<section class="workload"><strong>{{.Target.Group}}/{{.Target.Kind}}/{{.Target.Name}}</strong>{{if .OwnerNote}}<div class="muted">{{.OwnerNote}}</div>{{end}}<ul>{{range .Containers}}<li>{{if .Supported}}<a href="{{.Link}}">{{.Name}}</a>{{else}}<span>{{.Name}}</span>{{end}} — {{.Category}} — <span class="state">{{.RuntimeState}}</span></li>{{end}}</ul></section>{{end}}</div>{{else}}<div class="panel state unknown">EMPTY — no supported workloads were discovered</div>{{end}}
{{if .Selected}}<h2>Selected canonical target</h2><div class="panel"><div><strong>Namespace:</strong> {{.Selected.Target.Namespace}}</div><div><strong>Workload:</strong> {{.Selected.Target.Workload.Group}}/{{.Selected.Target.Workload.Kind}}/{{.Selected.Target.Workload.Name}}</div><div><strong>Container:</strong> {{.Selected.Target.Container}}</div></div>
<h2>Runtime subject / provenance</h2>{{if .Selected.RuntimeSubjects}}<div class="panel"><table><thead><tr><th>Pod UID</th><th>Image ID</th><th>Binary path</th></tr></thead><tbody>{{range .Selected.RuntimeSubjects}}<tr><td><code>{{if .PodUID}}{{.PodUID}}{{else}}NOT_AVAILABLE{{end}}</code></td><td><code>{{if .ImageID}}{{.ImageID}}{{else}}NOT_AVAILABLE{{end}}</code></td><td><code>{{if .BinaryPath}}{{.BinaryPath}}{{else}}NOT_AVAILABLE{{end}}</code></td></tr>{{end}}</tbody></table></div>{{else}}<div class="panel state unknown">NOT_AVAILABLE — no current runtime incarnation was discovered for this target</div>{{end}}
<h2>Security state</h2><div class="section-grid">
<section class="panel"><h3>Declared configuration</h3><div class="state">{{.Selected.Projection.Declared.State}}</div><div>{{.Selected.Projection.Declared.Reason}}</div></section>
<section class="panel"><h3>Materialized policy</h3><div class="state">{{.Selected.Projection.Materialized.State}}</div><div>{{.Selected.Projection.Materialized.Reason}}</div><div>PodLock: {{.Selected.Projection.Materialized.PodLockState}}</div><div>SPO: {{.Selected.Projection.Materialized.SPOState}}</div></section>
<section class="panel"><h3>Binding evidence</h3><div class="state">{{.Selected.Projection.Binding.State}}</div><div>{{.Selected.Projection.Binding.Reason}}</div></section>
<section class="panel"><h3>Enforcement evidence</h3><div class="state unknown">{{.Selected.Projection.Enforcement.State}}</div><div>{{.Selected.Projection.Enforcement.Reason}}</div></section>
<section class="panel"><h3>Behavioral verification</h3><div class="state unknown">{{.Selected.Projection.BehavioralVerification.State}}</div><div>{{.Selected.Projection.BehavioralVerification.Reason}}</div></section>
<section class="panel"><h3>Observed/runtime evidence</h3><div class="state">{{.Selected.Projection.Runtime.State}}</div><div>{{.Selected.Projection.Runtime.Reason}}</div>{{range .Selected.Projection.Runtime.Evidence}}<div class="boundary">Association: {{.Association.State}} — {{.Association.Reason}}</div>{{end}}{{range .Selected.Projection.Runtime.Excluded}}<div class="boundary">Excluded: {{.Association.State}} — {{.Association.Reason}}</div>{{end}}</section>
<section class="panel"><h3>Derived policy</h3><div class="state">{{.Selected.Projection.Derived.State}}</div><div>{{.Selected.Projection.Derived.Reason}}</div></section>
<section class="panel"><h3>Proposal/governance</h3><div class="state">{{.Selected.Projection.Governance.State}}</div><div>{{.Selected.Projection.Governance.Reason}}</div>{{range .Selected.Projection.Governance.Proposals}}<div class="boundary">Approval: {{.ApprovalState}} — binding valid: {{.ApprovalBindingValid}} — application: {{.Applied}}</div>{{end}}{{range .Selected.Projection.Governance.Excluded}}<div class="boundary">Excluded proposal: {{.Exclusion}} — {{.Reason}}</div>{{end}}</section>
</div><h2>Exact CLI next steps</h2><div class="panel"><p class="muted">These are advisory text only; the browser does not execute them.</p>{{range .NextSteps}}<div><code>{{.}}</code></div>{{end}}</div>{{else}}<h2>Initial proposal context</h2><div class="panel"><strong>{{.Proposal.Proposal}}</strong><div>{{.Proposal.Lifecycle}}</div><div>Candidate digest: <code>{{.Proposal.CandidateDigest}}</code></div><div>Approval: {{.Proposal.Approval}}</div><div>Approval binding: {{.Proposal.ApprovalBinding}}</div><div>Application: {{.Proposal.Application}}</div><div>Enforcement evidence: NOT_AVAILABLE — no enforcement evidence is persisted</div><div>Behavioral verification: {{.Proposal.Verification}}</div><div class="muted">This is not a current-to-proposed comparison.</div>{{range .Proposal.Provenance}}<div class="muted">{{.}}</div>{{end}}</div><p class="muted">Select a supported workload container above to inspect its independent security projection.</p>{{end}}
</main></body></html>`))

func newWorkbenchClusterHandler(view workbenchClusterView) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Method != http.MethodGet {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = workbenchClusterPageTemplate.Execute(w, view)
	})
}

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
