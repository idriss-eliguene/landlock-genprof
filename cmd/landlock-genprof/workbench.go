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
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
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
	Attempts  workbenchAttemptVisibility
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
		Use:   "ui [proposal]",
		Short: "Serves the local read-only Workbench HTTP boundary",
		Long: "Serves the local Workbench: the given SecurityProfileProposal at " +
			"\"/\", plus bounded durable-object workload/security-projection reads under \"/api\". Every read " +
			"goes through the bounded G0.5 read capability; there is no approval, rejection, " +
			"apply, or other mutation control unless authenticated governance mode is enabled." + kubectlPrefixNote,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proposalName := ""
			if len(args) == 1 {
				proposalName = args[0]
			}
			return runWorkbench(cmd.Context(), cmd.OutOrStdout(), opts, proposalName)
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
	if err := validateWorkbenchDeploymentConfig(opts.namespace); err != nil {
		return err
	}
	logger, metrics, obsConfig, err := observability.NewFromEnv(stdout)
	if err != nil {
		return fmt.Errorf("configuring observability: %w", err)
	}
	reads, err := newWorkbenchReadSession(opts.namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	handler, err := newWorkbenchServer(reads, proposalName, opts.port)
	if err != nil {
		return fmt.Errorf("constructing Workbench server: %w", err)
	}
	handler.logger = logger
	handler.metrics = metrics
	config, err := k8s.RestConfig()
	if err != nil {
		return fmt.Errorf("connecting Workbench observation API: %w", err)
	}
	requestContext, err := enableWorkbenchAuthorization(ctx, config, opts.namespace)
	if err != nil {
		return err
	}
	handler.requestContext = requestContext
	if requestContext == nil {
		writeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("constructing Workbench observation client: %w", err)
		}
		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("constructing Workbench observation dynamic client: %w", err)
		}
		handler.observations, err = newObservationAPI(writeClient, dynamicClient, opts.namespace)
		if err != nil {
			return fmt.Errorf("constructing Workbench observation API: %w", err)
		}
		// Local demo/qualification mode may compose the same separately
		// deployed Linux executor used by production-like mode. The executor
		// path is startup configuration only; it is never request data and is
		// never exposed by the Workbench HTTP surface.
		if executorPath := strings.TrimSpace(os.Getenv(observationExecutorKubeconfigEnv)); executorPath != "" {
			executorClients, executorErr := authz.NewConfiguredClients(executorPath, strings.TrimSpace(os.Getenv(observationExecutorContextEnv)))
			if executorErr != nil {
				return fmt.Errorf("configuring local Observation executor: %w", executorErr)
			}
			handler.observations = handler.observations.withExecutor(func() (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
				return executorClients.Core, executorClients.Dynamic, executorClients.Config, nil
			})
		}
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
	logger.Info("listener_ready", map[string]interface{}{"component": "operations_center", "deployment_mode": os.Getenv(workbenchDeploymentModeEnv), "address": addr})
	fmt.Fprintf(stdout, "Local Workbench: http://%s\n", addr)
	fmt.Fprintln(stdout, "Governance mutations require authenticated Operations Center mode.")
	handler.lifecycle.markStarted()
	var metricsServer *http.Server
	var metricsListener net.Listener
	if obsConfig.Metrics {
		metricsListener, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", obsConfig.MetricsPort))
		if err != nil {
			_ = listener.Close()
			return fmt.Errorf("binding metrics listener: %w", err)
		}
		metricsServer = &http.Server{Handler: http.HandlerFunc(metrics.ServeHTTP), ReadHeaderTimeout: workbenchReadHeaderTimeout, ReadTimeout: workbenchReadTimeout, WriteTimeout: workbenchWriteTimeout}
		go func() { _ = metricsServer.Serve(metricsListener) }()
		logger.Info("metrics_listener_ready", map[string]interface{}{"component": "operations_center", "port": obsConfig.MetricsPort})
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("Workbench listener: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown_initiated", map[string]interface{}{"component": "operations_center"})
		handler.lifecycle.beginDrain()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), workbenchShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down Workbench: %w", err)
		}
		if metricsServer != nil {
			_ = metricsServer.Shutdown(context.Background())
		}
		<-serveErr
		logger.Info("shutdown_completed", map[string]interface{}{"component": "operations_center"})
		return nil
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
			"This is a bounded read assembled from durable Kubernetes objects for this request; it is not cached and does not require a restart to reflect newly persisted changes.",
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
	proposalView := workbenchView{Namespace: reads.SessionIdentity().Namespace, Proposal: proposalName, ReadAt: time.Now().UTC().Format(time.RFC3339), Lifecycle: "EXPLORER — workload-first discovery", Application: "NOT_AVAILABLE — no proposal selected", Verification: "NOT_AVAILABLE — no proposal selected", Approval: "NOT_AVAILABLE"}
	var err error
	if proposalName != "" {
		proposalView, err = loadWorkbenchView(ctx, reads, proposalName)
		if err != nil {
			return workbenchClusterView{}, err
		}
	}
	discovery, err := workload.NewService(reads)
	if err != nil {
		return workbenchClusterView{}, err
	}
	result, err := discovery.Discover(ctx)
	if err != nil {
		return workbenchClusterView{}, err
	}
	page := workbenchClusterView{Namespace: result.Namespace, Proposal: proposalView, Attempts: loadWorkbenchAttemptVisibility(ctx, reads)}
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
	if strings.EqualFold(strings.TrimSpace(os.Getenv(workbenchDeploymentModeEnv)), "production") {
		return fmt.Sprintf("0.0.0.0:%d", port)
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func workbenchAllowedHost(port int) string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(workbenchDeploymentModeEnv)), "production") {
		if host := strings.TrimSpace(os.Getenv(workbenchAllowedHostEnv)); host != "" {
			return host
		}
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

var workbenchPage = template.Must(template.New("workbench").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Experimental Workbench — {{.Proposal}}</title>
<style>
:root{color-scheme:light;--ink:#1f2933;--muted:#52606d;--line:#d9e2ec;--panel:#f5f7fa;--navy:#102a43;--accent:#245b75;--warn:#8a5a00;--good:#176b45;--danger:#a61b1b}
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
<div class="notice">This page reflects a best-effort read assembled from durable Kubernetes objects at {{.ReadAt}}; reload to read the objects again. It is a presentation surface only: approval and application remain explicit CLI operations. Application and behavioral verification are not recorded in the canonical proposal state.</div>
<h2>Exact candidate identity</h2><div class="digest"><strong>Candidate digest</strong><br>{{.CandidateDigest}}</div>
<h2>Lifecycle</h2><div class="state-grid"><div class="card"><strong>Proposal</strong><span class="state">{{.Lifecycle}}</span></div><div class="card"><strong>Application</strong><span class="state unknown">{{.Application}}</span></div><div class="card"><strong>Enforcement evidence</strong><span class="state unknown">NOT_AVAILABLE — no enforcement evidence is persisted</span></div><div class="card"><strong>Behavioral verification</strong><span class="state unknown">{{.Verification}}</span></div></div>
<h2>Candidate authority / policy</h2><p class="boundary">This is the structured candidate contained in the proposal. It is not a current-to-proposed delta because live current configuration is not available here.</p><table><thead><tr><th>Domain</th><th>Candidate</th><th>Availability</th><th>Provenance</th><th>Reviewer action</th></tr></thead><tbody>{{range .Domains}}<tr><td><strong>{{.Name}}</strong></td><td>{{.Candidate}}</td><td class="unknown">{{.Availability}}</td><td>{{.Provenance}}</td><td>{{.ReviewState}}</td></tr>{{end}}</tbody></table>
<h2>Evidence & provenance</h2>{{range .Provenance}}<div class="boundary">{{.}}</div>{{end}}
<h2>Authorization</h2><div class="state-grid"><div class="card"><strong>Approval state</strong><span class="state">{{.Approval}}</span>{{if .ApprovalReason}}<br><span>{{.ApprovalReason}}</span>{{end}}</div><div class="card"><strong>Approval binding</strong><span class="state unknown">{{.ApprovalBinding}}</span></div><div class="card"><strong>Approved candidate digest</strong><code>{{.ApprovedDigest}}</code></div><div class="card"><strong>Approval mechanism</strong><span>{{.ApprovalVersion}}</span></div><div class="card"><strong>Approval updated</strong><span>{{.ApprovalUpdated}}</span></div></div>
<h2>Unsupported / unknown boundaries</h2>{{range .Boundaries}}<div class="boundary">{{.}}</div>{{end}}
</main><h2>Governance custody</h2><div class="panel"><div class="state">{{.Attempts.State}}</div><div class="muted">{{.Attempts.Reason}}</div><div>Current custody epoch: <code>{{.Attempts.CustodyEpoch}}</code></div>{{if .Attempts.ApplyAttempts}}<h3>ApplyAttempt history</h3><table><thead><tr><th>Attempt</th><th>Proposal / target</th><th>State</th><th>Mutations</th></tr></thead><tbody>{{range .Attempts.ApplyAttempts}}<tr><td><code>{{.Namespace}}/{{.Name}}</code><br>UID <code>{{.UID}}</code></td><td>{{.Proposal}}<br>{{.Target}}<br><code>{{.Digest}}</code></td><td class="state">{{.State}}</td><td>{{range .Mutations}}<details><summary>{{.ID}} — {{.Operation}} — {{.Result}}</summary><div>{{.Kind}}/{{.Name}} RV <code>{{.AttributableAfterRV}}</code></div><pre>{{.Before}}</pre><pre>{{.IntendedAfter}}</pre><pre>{{.ObservedAfter}}</pre></details>{{end}}</td></tr>{{end}}</tbody></table>{{else}}<div class="muted">EMPTY — no ApplyAttempt records returned</div>{{end}}{{if .Attempts.RollbackAttempts}}<h3>RollbackAttempt history</h3><table><thead><tr><th>Attempt</th><th>Source / target</th><th>State</th><th>Inverse mutations</th></tr></thead><tbody>{{range .Attempts.RollbackAttempts}}<tr><td><code>{{.Namespace}}/{{.Name}}</code><br>UID <code>{{.UID}}</code></td><td>{{.Source}}<br>{{.Previous}}<br>{{.Target}}</td><td class="state">{{.State}}</td><td>{{range .Mutations}}<details><summary>{{.ID}} — {{.Operation}} — {{.Result}}</summary><div>Source mutation: {{.SourceMutationID}}</div><pre>{{.Before}}</pre><pre>{{.IntendedAfter}}</pre><pre>{{.ObservedAfter}}</pre></details>{{end}}</td></tr>{{end}}</tbody></table>{{else}}<div class="muted">EMPTY — no RollbackAttempt records returned</div>{{end}}</div></main></body></html>`))

var workbenchClusterPageTemplate = template.Must(template.New("cluster-workbench").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Operations Center — {{.Namespace}}</title><style>
:root{color-scheme:light;--surface:#fff;--surface-subtle:#f5f7fa;--surface-raised:#fff;--nav:#102a43;--nav-hover:#243b53;--nav-selected:#334e68;--text:#1f2933;--text-muted:#52606d;--text-inverse:#fff;--border:#d9e2ec;--border-strong:#9fb3c8;--primary:#245b75;--primary-hover:#17465a;--success:#176b45;--success-surface:#e8f5ee;--warning:#8a5a00;--warning-surface:#fff8e1;--danger:#a61b1b;--danger-surface:#fde8e8;--unknown:#52606d;--unknown-surface:#f1f3f5;--focus:#bfdbfe}
*{box-sizing:border-box}body{margin:0;background:var(--surface-subtle);color:var(--text);font:16px/1.45 system-ui,-apple-system,sans-serif}button,select{font:inherit}button{border:1px solid var(--border-strong);border-radius:6px;background:var(--surface);color:var(--text);padding:8px 12px;cursor:pointer}button:hover:not(:disabled){border-color:var(--primary);background:#f0f6f8}button:focus-visible,select:focus-visible{outline:3px solid var(--focus);outline-offset:2px}button:disabled{cursor:not-allowed;opacity:.62}.app-shell{min-height:100vh;display:grid;grid-template-columns:224px minmax(0,1fr);grid-template-rows:64px minmax(0,1fr)}.sidebar{grid-row:1/3;background:var(--nav);color:var(--text-inverse);padding:18px 12px}.brand{padding:0 10px 18px}.brand strong{display:block;font-size:18px}.brand span{color:#d9e2ec;font-size:12px}.primary-nav{display:flex;flex-direction:column;gap:4px}.primary-nav button{width:100%;min-height:40px;text-align:left;border:0;background:transparent;color:#d9e2ec}.primary-nav button:hover:not(:disabled){background:var(--nav-hover);color:var(--text-inverse)}.primary-nav button[aria-current=page]{background:var(--nav-selected);color:var(--text-inverse);box-shadow:inset 3px 0 var(--text-inverse);font-weight:700}.topbar{grid-column:2;min-width:0;display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 24px;background:var(--surface);border-bottom:1px solid var(--border);position:sticky;top:0;z-index:2}.topbar h1{margin:0;font-size:22px}.topbar .eyebrow{color:var(--text-muted);font-size:12px;text-transform:uppercase;letter-spacing:.08em}.content{grid-column:2;min-width:0;max-width:1280px;width:100%;padding:24px}.context-bar{display:flex;align-items:center;gap:8px 12px;flex-wrap:wrap;padding:12px 16px;background:var(--surface);border:1px solid var(--border);border-radius:8px;margin-bottom:20px}.context-chip{display:inline-flex;align-items:center;gap:5px;padding:4px 8px;border-radius:5px;background:var(--surface-subtle);border:1px solid var(--border);white-space:nowrap}.context-chip strong{font-size:12px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.04em}.context-chip.status{font-weight:700}.context-chip.status.success{color:var(--success);background:var(--success-surface)}.context-chip.status.warning{color:var(--warning);background:var(--warning-surface)}.context-chip.status.danger{color:var(--danger);background:var(--danger-surface)}.context-chip.status.unknown{color:var(--unknown);background:var(--unknown-surface)}.context-refresh{margin-left:auto}.view{display:block}.view[hidden]{display:none}.view-header{display:flex;align-items:end;justify-content:space-between;gap:16px;margin:0 0 16px}.view-header h2{margin:0;color:var(--text);font-size:24px}.view-header p{margin:4px 0 0;color:var(--text-muted)}.panel,.card{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:16px}.panel{margin:0 0 16px}.card{box-shadow:0 1px 2px rgba(16,42,67,.05)}.summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.summary-card h3{margin:0;color:var(--text-muted);font-size:13px;text-transform:uppercase;letter-spacing:.04em}.summary-card .metric{display:block;margin-top:8px;font-size:26px;font-weight:750}.status-line{display:flex;align-items:center;gap:8px}.status-badge,.outcome-badge{display:inline-flex;align-items:center;gap:5px;border:1px solid currentColor;border-radius:999px;padding:3px 8px;font-size:13px;font-weight:700}.healthy,.success{color:var(--success);background:var(--success-surface)}.degraded,.partial{color:var(--warning);background:var(--warning-surface)}.unavailable,.failed{color:var(--danger);background:var(--danger-surface)}.unknown,.not-eligible{color:var(--unknown);background:var(--unknown-surface)}.notice{border-left:4px solid var(--warning);background:var(--warning-surface);padding:12px 14px;margin:0 0 16px}.notice.error-state,.stale-state{border-left-color:var(--danger);background:var(--danger-surface)}.boundary{color:var(--text-muted);margin:8px 0}.muted{color:var(--text-muted)}.diagnostic-panel{border-left:4px solid var(--warning);background:var(--warning-surface);padding:12px 14px}.attention-list{display:grid;gap:10px;padding:0;margin:0;list-style:none}.attention-item{display:grid;grid-template-columns:minmax(150px,.7fr) minmax(220px,1.3fr) minmax(180px,1fr) auto;gap:12px;align-items:start;padding:12px;border:1px solid var(--border);border-left:4px solid var(--warning);border-radius:7px;background:var(--surface);overflow-wrap:anywhere}.attention-item strong{display:block}.technical{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}.data-table{width:100%;border-collapse:collapse;font-size:14px}.data-table th,.data-table td{text-align:left;vertical-align:top;border-bottom:1px solid var(--border);padding:10px 8px}.data-table th{background:var(--surface-subtle);font-size:13px;color:var(--text-muted)}.table-wrap{overflow-x:auto}.workload-row.selected{background:#edf6fa}.workload-row button,.proposal-row button{margin-right:6px}.workload-row .unknown,.proposal-row .unknown{background:transparent;border:0;padding:0}.section-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:12px}.state-list{display:grid;gap:8px}.state-item{display:flex;justify-content:space-between;gap:12px;border-bottom:1px solid var(--border);padding:7px 0}.state-item strong{font-size:14px}.state-item span{font-weight:650;text-align:right}.action-group{display:flex;flex-wrap:wrap;gap:8px;align-items:start}.action-group .disabled-reason{width:100%;font-size:13px;color:var(--text-muted)}.danger-action{color:var(--danger);border-color:var(--danger)}.history-table tr.discontinuity td{border-top:3px solid var(--warning);background:var(--warning-surface)}.empty-state,.unavailable-state,.degraded-state,.stale-state{padding:16px;border:1px solid var(--border);border-radius:8px;margin:8px 0}.empty-state{background:var(--surface-subtle)}.unavailable-state{background:var(--danger-surface);border-color:#e0a3a3}.degraded-state{background:var(--warning-surface);border-color:#e4bd69}.stale-state{background:var(--danger-surface);border-color:#e0a3a3}.no-wrap{white-space:nowrap}@media(max-width:1100px){.app-shell{grid-template-columns:200px minmax(0,1fr)}.summary-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.attention-item{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:700px){.app-shell{display:block}.sidebar{position:static;padding:12px}.primary-nav{display:grid;grid-template-columns:repeat(2,minmax(0,1fr))}.topbar{position:static;padding:12px 16px}.content{padding:16px}.summary-grid{grid-template-columns:1fr}.attention-item{grid-template-columns:1fr}.context-refresh{margin-left:0}}
</style><style>
.environment-panel{padding:20px;background:var(--surface);border:1px solid var(--border);border-radius:10px;margin-bottom:20px}.environment-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:20px;margin-bottom:18px}.environment-heading h2{margin:2px 0 2px;color:var(--text);font-size:22px}.environment-heading p{margin:0;color:var(--text-muted)}.environment-controls{display:grid;grid-template-columns:repeat(3,minmax(180px,1fr));gap:16px;margin-bottom:18px}.environment-field{display:flex;flex-direction:column;gap:7px;min-width:0}.environment-field label{font-weight:700;color:var(--text)}.environment-field select,.environment-field input{width:100%;min-height:40px}.field-help{font-size:13px;color:var(--text-muted);line-height:1.35}.explicit-namespace-controls{display:flex;gap:8px}.explicit-namespace-controls input{min-width:0}.context-values{display:flex;flex-wrap:wrap;gap:8px;padding-top:14px;border-top:1px solid var(--border)}.context-chip{display:inline-flex;align-items:center;gap:5px;padding:5px 9px;border-radius:5px;background:var(--surface-subtle);border:1px solid var(--border);white-space:nowrap}
.environment-details{flex-basis:100%;padding-top:4px}.environment-details summary{display:inline-block;padding:6px 10px;color:var(--primary);font-weight:700;cursor:pointer}.environment-details[open]{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:8px}.environment-details[open] summary{grid-column:1/-1}.context-chip.secondary{font-size:13px;color:var(--text-muted);white-space:normal}.context-chip.secondary strong{font-size:11px}
@media(max-width:1100px){.environment-controls{grid-template-columns:repeat(3,minmax(150px,1fr))}}@media(max-width:700px){.environment-heading{display:block}.environment-heading .context-refresh{margin-top:12px}.environment-controls{grid-template-columns:1fr}}
</style></head><body><div class="app-shell">
<aside class="sidebar" aria-label="Operations Center navigation"><div class="brand"><strong>Operations Center</strong><span>Environment-aware security operations</span></div><nav class="primary-nav" aria-label="Primary navigation"><button type="button" data-view="overview">Overview</button><button type="button" data-view="workloads">Workloads</button><button type="button" data-view="observations">Observations</button><button type="button" data-view="proposals">Proposals</button><button type="button" data-view="history">History</button><button type="button" data-view="attention">Attention</button><button type="button" data-view="governance">Governance</button></nav></aside>
<header class="topbar"><div><div class="eyebrow">Operations Center V2 · bounded environment</div><h1>Operations Center</h1></div></header>
<main id="observation-workbench" class="content" data-namespace="{{.Namespace}}"><section id="operations-context" class="environment-panel" aria-labelledby="environment-heading"><div class="environment-heading"><div><div class="eyebrow">Operational scope</div><h2 id="environment-heading">Environment</h2><p>Choose the cluster, identity, and namespace for this session.</p></div><button id="refresh-operations-context" class="context-refresh" type="button">Refresh environments</button></div><div class="environment-controls"><div class="environment-field"><label for="cluster-selector">Cluster</label><select id="cluster-selector"><option value="">Loading clusters…</option></select></div><div class="environment-field"><label for="identity-selector">Identity / context</label><select id="identity-selector" disabled><option value="">Select a cluster first</option></select></div><div class="environment-field namespace-field"><label for="namespace-selector">Namespace</label><select id="namespace-selector" disabled><option value="">Select an identity first</option></select><div id="explicit-namespace-help" class="field-help" hidden>Namespace discovery is not permitted for this identity. Enter a namespace you are authorized to access.</div><div class="explicit-namespace-controls" hidden><input id="explicit-namespace" type="text" placeholder="Enter namespace name" aria-label="Known namespace"><button id="open-namespace" type="button">Open</button></div></div></div><div id="operations-context-values" class="context-values"><span class="context-chip"><strong>Context</strong> Loading authoritative state…</span></div></section><p id="workbench-message" class="notice" role="status" aria-live="polite" hidden></p>
<section id="overview-view" class="view" data-page="overview"><div class="view-header"><div><h2>Overview</h2><p>Evidence, authority, and governance state for the active environment.</p></div></div><div id="overview-summary" class="summary-grid"></div><div id="overview-recent" class="panel"></div></section>
<section id="workloads-view" class="view" data-page="workloads" hidden><div class="view-header"><div><h2>Workloads</h2><p>Canonical workload and container identities with current operational state.</p></div></div><div id="workload-list"></div></section>
<section id="environment-view" class="view" data-page="environment" hidden><div class="view-header"><div><h2>Environment</h2><p>Projection subjects and certified state dimensions.</p></div></div><p id="environment-boundary" class="boundary">BEST_EFFORT_MULTI_OBJECT_READ</p><div id="environment-list"></div></section>
<section id="attention-view" class="view" data-page="attention" hidden><div class="view-header"><div><h2>Attention</h2><p>Investigation queue for bounded conditions requiring operator review.</p></div></div><ul id="attention-list" class="attention-list"></ul></section>
<section id="history-view" class="view" data-page="history" hidden><div id="history-detail"></div></section>
<section id="observations-view" class="view" data-page="observations" hidden><div class="view-header"><div><h2>Observations / Evidence</h2><p>Valid evidence remains actionable; malformed evidence remains excluded.</p></div></div><div class="panel"><label for="workload-picker"><strong>Canonical workload / container</strong></label><select id="workload-picker"><option value="">Select a discovered container</option></select><div class="action-group" aria-label="Observation actions"><button id="start-observation" type="button">Start observation</button><button id="stop-observation" type="button">Stop selected</button><button id="generate-proposal" type="button">Generate proposal</button></div></div><div class="section-grid"><section class="panel"><h3>Observation records</h3><ul id="observation-list"></ul></section><section class="panel"><h3>Evidence summary</h3><div id="observation-detail"></div></section></div></section>
<section id="proposals-view" class="view" data-page="proposals" hidden><div class="view-header"><div><h2>Proposals / Governance</h2><p>Named operations require effective capability, semantic eligibility, and the displayed resourceVersion.</p></div></div><div id="proposal-list"></div></section>
<section id="governance-view" class="view" data-page="governance" hidden><div class="view-header"><div><h2>Governance</h2><p>Consequential actions are bound to the displayed proposal, namespace, and resourceVersion.</p></div></div><div class="panel"><h3>Mutation safety</h3><p>Review, approval, application, and rollback remain explicit operations. A stale resourceVersion is rejected and is never replayed automatically.</p><button id="open-proposals" type="button">Open proposals</button></div></section>
</main></div><script src="/workbench.js" defer></script></body></html>`))

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
