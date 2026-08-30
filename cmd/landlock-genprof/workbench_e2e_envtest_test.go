//go:build envtest

// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Workbench end-to-end certification.
//
// workbench_test.go already covers the projection and the handler through
// httptest: an in-process ServeHTTP call against a view the test built
// itself. That proves the projection and the read-only handler contract,
// but it never runs the shipped `ui` command, never opens a socket, and
// never reads a real SecurityProfileProposal from a real API server.
//
// This file closes exactly that gap and nothing else:
//
//	real kube-apiserver + etcd (envtest, real CRD)
//	  -> real SecurityProfileProposal written by production proposal.Save
//	    -> production landlock-genprof binary
//	      -> production `ui` command
//	        -> 127.0.0.1:<port>, real TCP
//	          -> real HTTP
//	            -> rendered HTML
//
// Every expected value comes from canonical state or from production CLI
// output. Nothing here recomputes a candidate digest, an approval verdict,
// or a provenance classification: a second implementation of those is
// precisely what the Workbench must not become, and a test that reimplements
// them would certify its own arithmetic rather than the product's.
//
// Build-tagged `envtest` and run by `make envtest` alongside the existing CRD
// semantics suites, so this needs no cluster, no operator, and no new
// workflow. envtest is a real API server, not a node-bearing cluster; the
// Workbench only ever issues two reads, so nothing it does depends on a
// kubelet, a CNI, or SPO convergence.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

var (
	e2eEnv        *envtest.Environment
	e2eConfig     *rest.Config
	e2eKubeconfig string
	e2eBinary     string
)

// workbenchStartTimeout bounds readiness polling. The listener is opened by
// the production command immediately after two API reads, so this is
// generous rather than tuned; a hang fails the test rather than the suite.
const workbenchStartTimeout = 30 * time.Second

// maxResponseBytes bounds what a test reads from the listener. The page is a
// few kilobytes; this only stops a pathological response from filling memory.
const maxResponseBytes = 1 << 20

func TestMain(m *testing.M) {
	code, err := runE2EMain(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "workbench e2e setup: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runE2EMain(m *testing.M) (int, error) {
	crdPath := filepath.Join("..", "..", "deploy", "crd-securityprofileproposal.yaml")
	if _, err := os.Stat(crdPath); err != nil {
		return 0, fmt.Errorf("locating CRD %s: %w", crdPath, err)
	}

	e2eEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths:              []string{crdPath},
			ErrorIfPathMissing: true,
		},
	}

	var err error
	e2eConfig, err = e2eEnv.Start()
	if err != nil {
		return 0, fmt.Errorf("starting envtest control plane: %w", err)
	}
	defer func() {
		if stopErr := e2eEnv.Stop(); stopErr != nil {
			fmt.Fprintf(os.Stderr, "stopping envtest: %v\n", stopErr)
		}
	}()

	workDir, err := os.MkdirTemp("", "workbench-e2e")
	if err != nil {
		return 0, fmt.Errorf("creating work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	// The production binary resolves credentials through k8s.RestConfig,
	// which honours KUBECONFIG. Handing it a real kubeconfig for the
	// envtest API server is what keeps the credential path production
	// rather than a test seam.
	user, err := e2eEnv.AddUser(envtest.User{Name: "workbench-e2e", Groups: []string{"system:masters"}}, nil)
	if err != nil {
		return 0, fmt.Errorf("provisioning envtest user: %w", err)
	}
	kubeconfig, err := user.KubeConfig()
	if err != nil {
		return 0, fmt.Errorf("rendering envtest kubeconfig: %w", err)
	}
	e2eKubeconfig = filepath.Join(workDir, "kubeconfig")
	if err := os.WriteFile(e2eKubeconfig, kubeconfig, 0o600); err != nil {
		return 0, fmt.Errorf("writing kubeconfig: %w", err)
	}

	// Build the real command, untagged, exactly as a release build would.
	e2eBinary = filepath.Join(workDir, "landlock-genprof")
	build := exec.Command("go", "build", "-o", e2eBinary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("building landlock-genprof: %w\n%s", err, out)
	}

	return m.Run(), nil
}

func e2eDynamicClient(t *testing.T) dynamic.Interface {
	t.Helper()
	client, err := dynamic.NewForConfig(e2eConfig)
	if err != nil {
		t.Fatalf("dynamic.NewForConfig() error = %v", err)
	}
	return client
}

// spoSeccompArtifact is a governed SPO-sourced SeccompProfile carrying the
// provenance annotations internal/spobackend defines. It is stored as the
// rendered artifact text the production exporter produces, which is what
// proposal.Spec holds and what CandidateDigest covers — not a parallel
// proposal model invented for this test.
const spoSeccompArtifact = `apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  name: lg-v1-wb-cert
  annotations:
    landlockgenprof.io/seccomp-source: spo
    landlockgenprof.io/seccomp-origin: derived
    landlockgenprof.io/spo-source-profile: wb-cert-source
    landlockgenprof.io/spo-recording-namespace: default
    landlockgenprof.io/spo-recording-name: wb-cert-recording
    landlockgenprof.io/spo-container-id: app
    landlockgenprof.io/spo-syscall-coverage: unknown
spec:
  defaultAction: SCMP_ACT_ERRNO
  syscalls:
    - action: SCMP_ACT_ALLOW
      names: [read, write]
`

const podLockArtifact = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: wb-cert
spec:
  profilesByContainer:
    app:
      /usr/bin/app:
        readOnly: [/etc/app]
`

// seedProposal publishes one SecurityProfileProposal through the production
// write path (the same proposal.Save that `trace` uses), so the object under
// review is a real CR validated by the real CRD schema.
func seedProposal(t *testing.T, name string) proposal.Spec {
	t.Helper()
	spec := proposal.Spec{
		Container:         "app",
		Binary:            "/usr/bin/app",
		GeneratedAt:       "2026-08-30T10:00:00Z",
		PodLock:           podLockArtifact,
		SPOSeccompProfile: spoSeccompArtifact,
	}
	if err := proposal.Save(context.Background(), e2eDynamicClient(t), "default", name, spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	return spec
}

// runCLI executes the production binary against the envtest API server and
// returns its combined output.
func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(e2eBinary, args...)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+e2eKubeconfig)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock-genprof %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// canonicalDigestFor reads the candidate digest out of production `review`
// output. The digest is never recomputed here: `review` is the command a
// reviewer actually runs, and its printed value is the authority this test
// compares the rendered page against.
func canonicalDigestFor(t *testing.T, name string) string {
	t.Helper()
	out := runCLI(t, "review", name, "-n", "default")
	for _, line := range strings.Split(out, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "Candidate digest: "); found {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("review printed no candidate digest:\n%s", out)
	return ""
}

// freeLoopbackPort reserves and releases a loopback port. The Workbench binds
// 127.0.0.1 only, and nothing here changes that: the port is discovered on
// loopback and handed to the documented --port flag.
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("releasing reserved port: %v", err)
	}
	return port
}

// e2eHTTPClient never uses a proxy: a CI runner exporting HTTP_PROXY must not
// be able to turn a loopback assertion into a request that left the host.
var e2eHTTPClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: &http.Transport{Proxy: nil},
}

type workbenchProcess struct {
	baseURL string
	output  *syncBuffer
	stop    func()
}

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startWorkbench runs the production `ui` command against the envtest API
// server and waits for its real listener. It fails rather than sleeps: the
// process exiting early is a hard failure, and readiness is a bounded poll
// against the actual socket.
func startWorkbench(t *testing.T, proposalName string) *workbenchProcess {
	t.Helper()

	port := freeLoopbackPort(t)
	output := &syncBuffer{}
	cmd := exec.Command(e2eBinary, "ui", proposalName, "-n", "default", "--port", fmt.Sprint(port))
	cmd.Env = append(os.Environ(), "KUBECONFIG="+e2eKubeconfig)
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the ui command: %v", err)
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			_ = cmd.Process.Kill()
			<-exited
		})
	}
	t.Cleanup(stop)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(workbenchStartTimeout)
	for {
		select {
		case err := <-exited:
			exited <- err
			t.Fatalf("ui exited before serving (%v)\nproposal: default/%s\nport: %d\noutput:\n%s",
				err, proposalName, port, output.String())
		default:
		}

		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ui listener never came up on %s within %s\nproposal: default/%s\noutput:\n%s",
				baseURL, workbenchStartTimeout, proposalName, output.String())
		}
		time.Sleep(100 * time.Millisecond)
	}

	return &workbenchProcess{baseURL: baseURL, output: output, stop: stop}
}

// get performs a real HTTP request over the real listener.
func (w *workbenchProcess) get(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := e2eHTTPClient.Get(w.baseURL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v\nworkbench output:\n%s", path, err, w.output.String())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		t.Fatalf("reading GET %s body: %v", path, err)
	}
	return resp.StatusCode, string(body)
}

func (w *workbenchProcess) post(t *testing.T, path string) int {
	t.Helper()
	resp, err := e2eHTTPClient.Post(w.baseURL+path, "text/plain", strings.NewReader("mutate"))
	if err != nil {
		t.Fatalf("POST %s failed: %v\nworkbench output:\n%s", path, err, w.output.String())
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestWorkbenchE2E_ProductionUIServesCanonicalProjectionOverRealHTTP is the
// certification this file exists for: the shipped command, a real proposal,
// a real socket, and a real HTTP response whose content is checked against
// canonical values rather than against a copy of the projection logic.
func TestWorkbenchE2E_ProductionUIServesCanonicalProjectionOverRealHTTP(t *testing.T) {
	const name = "wb-cert-projection"
	seedProposal(t, name)
	digest := canonicalDigestFor(t, name)

	workbench := startWorkbench(t, name)

	status, body := workbench.get(t, "/")
	if status != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d\nbody:\n%s", status, http.StatusOK, truncate(body))
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "<!doctype html>") {
		t.Fatalf("GET / did not return rendered HTML:\n%s", truncate(body))
	}

	// Identity: the page names the proposal the command was started against.
	if !strings.Contains(body, "default/"+name) {
		t.Errorf("rendered page does not identify the selected proposal %q:\n%s", "default/"+name, truncate(body))
	}

	// Candidate identity: exactly the digest production `review` printed.
	if !strings.Contains(body, digest) {
		t.Errorf("rendered page does not carry the canonical candidate digest %q:\n%s", digest, truncate(body))
	}

	// Provenance: the canonical source classification, not a page-local one.
	if !strings.Contains(body, "DERIVED POLICY / SECURITY-PROFILES-OPERATOR") {
		t.Errorf("rendered page lost the canonical SPO source classification:\n%s", truncate(body))
	}
	if !strings.Contains(body, "Origin: derived policy") {
		t.Errorf("rendered page lost the canonical derived-policy origin line:\n%s", truncate(body))
	}
	if !strings.Contains(body, "Confidence: not applicable") {
		t.Errorf("rendered page lost the canonical no-confidence statement for derived policy:\n%s", truncate(body))
	}

	// Authorization: a reviewed, unapproved candidate must not read as bound.
	if !strings.Contains(body, "NOT BOUND / RE-APPROVAL REQUIRED") {
		t.Errorf("rendered page did not report an unapproved candidate as unbound:\n%s", truncate(body))
	}

	// Snapshot disclosure reaches the browser, not just the view struct.
	for _, want := range []string{"This page is a snapshot read at ", "Restart the command to refresh cluster state."} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page omitted snapshot disclosure %q:\n%s", want, truncate(body))
		}
	}

	// Unavailable states stay unavailable over the wire.
	for _, want := range []string{
		"NOT_AVAILABLE — application outcome is not persisted",
		"NOT_AVAILABLE — no enforcement evidence is persisted",
		"NOT_AVAILABLE — behavioral verification is not persisted",
		"not a current-to-proposed comparison",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page omitted boundary %q:\n%s", want, truncate(body))
		}
	}

	// No mutation affordance is served to a browser.
	for _, forbidden := range []string{"<form", "<button", "<input", "<script"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("rendered page exposes %q, a browser mutation or scripting affordance:\n%s", forbidden, truncate(body))
		}
	}

	// Method and route contract, over real TCP rather than httptest.
	if got := workbench.post(t, "/"); got != http.StatusMethodNotAllowed {
		t.Errorf("POST / status = %d, want %d", got, http.StatusMethodNotAllowed)
	}
	for _, route := range []string{"/approve", "/apply", "/reject"} {
		if got, _ := workbench.get(t, route); got != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d", route, got, http.StatusNotFound)
		}
	}
}

// TestWorkbenchE2E_SnapshotIsFixedAtStartupAndRefreshedOnRestart certifies
// the publicly documented snapshot claim.
//
// The state transition is a real approval recorded by the production
// `approve` command against the digest production `review` printed. Nothing
// forges a status, mutates the proposal out of band, or writes a digest this
// test computed: the whole point is that the transition is one the product
// itself performs.
func TestWorkbenchE2E_SnapshotIsFixedAtStartupAndRefreshedOnRestart(t *testing.T) {
	const name = "wb-cert-snapshot"
	seedProposal(t, name)
	digest := canonicalDigestFor(t, name)

	before := startWorkbench(t, name)
	_, initial := before.get(t, "/")
	if !strings.Contains(initial, "NOT BOUND / RE-APPROVAL REQUIRED") {
		t.Fatalf("pre-approval snapshot already reports a bound approval:\n%s", truncate(initial))
	}

	// Legitimate transition, performed by the product.
	runCLI(t, "approve", name, "-n", "default", "--expected-digest", digest, "--reason", "workbench e2e certification")

	// Confirm the cluster really moved, using the canonical read path.
	status, err := proposal.GetStatus(context.Background(), e2eDynamicClient(t), "default", name)
	if err != nil {
		t.Fatalf("proposal.GetStatus() error = %v", err)
	}
	if status.ApprovalState != proposal.ApprovalApproved || status.ApprovedCandidateDigest != digest {
		t.Fatalf("approve did not bind the candidate: state=%q approved=%q want state=%q approved=%q",
			status.ApprovalState, status.ApprovedCandidateDigest, proposal.ApprovalApproved, digest)
	}

	// The running Workbench must still serve its startup snapshot.
	_, afterApproval := before.get(t, "/")
	if !strings.Contains(afterApproval, "NOT BOUND / RE-APPROVAL REQUIRED") {
		t.Errorf("running Workbench silently picked up a post-startup cluster change:\n%s", truncate(afterApproval))
	}
	if strings.Contains(afterApproval, "BOUND — approved digest validates") {
		t.Errorf("running Workbench rendered an approval recorded after its snapshot:\n%s", truncate(afterApproval))
	}
	before.stop()

	// A restart obtains the new snapshot.
	after := startWorkbench(t, name)
	_, restarted := after.get(t, "/")
	if !strings.Contains(restarted, "BOUND — approved digest validates against the current candidate") {
		t.Errorf("restarted Workbench did not observe the recorded approval:\n%s", truncate(restarted))
	}
	if !strings.Contains(restarted, string(proposal.ApprovalApproved)) {
		t.Errorf("restarted Workbench did not render the canonical approval state:\n%s", truncate(restarted))
	}
}

// truncate bounds failure output: enough HTML to debug, not a full dump of
// cluster-derived content into CI logs.
func truncate(body string) string {
	const limit = 4000
	if len(body) <= limit {
		return body
	}
	return body[:limit] + "\n... (truncated)"
}
