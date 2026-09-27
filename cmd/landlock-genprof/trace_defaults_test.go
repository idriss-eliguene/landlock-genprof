package main

import (
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

// TestDefaultOutFiles pins the filenames trace writes when an output flag
// is left at its auto value: a typo in one of these Sprintf formats would
// otherwise change what lands on disk without any test noticing.
func TestDefaultOutFiles(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{name: "profile", fn: defaultOutFile, want: "nginx-demo-profile.yaml"},
		{name: "networkpolicy", fn: defaultNetworkOutFile, want: "nginx-demo-networkpolicy.yaml"},
		{name: "seccomp", fn: defaultSeccompOutFile, want: "nginx-demo-seccomp.json"},
		{name: "seccompprofile", fn: defaultSeccompProfileOutFile, want: "nginx-demo-seccompprofile.yaml"},
		{name: "capabilities", fn: defaultCapabilitiesOutFile, want: "nginx-demo-capabilities.yaml"},
		{name: "securitycontext", fn: defaultSecurityContextOutFile, want: "nginx-demo-securitycontext.yaml"},
		{name: "report", fn: defaultReportOutFile, want: "nginx-demo-report.md"},
		{name: "patched manifest", fn: defaultPatchedManifestOutFile, want: "nginx-demo-patched.yaml"},
		{name: "candidate", fn: defaultCandidateOutFile, want: "nginx-demo-candidate.json"},
		{name: "events", fn: defaultEventsOutFile, want: "nginx-demo-events.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn("nginx-demo"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPodLockLabelHint covers each owner-kind branch: workloads must be
// labeled on their pod template (a label on the pod itself would be lost
// on the next rollout), while a bare pod is labeled directly.
func TestPodLockLabelHint(t *testing.T) {
	templatePatch := `-p '{"spec":{"template":{"metadata":{"labels":{"podlock.kubewarden.io/profile":"nginx-demo"}}}}}'`

	tests := []struct {
		name        string
		owner       k8s.OwnerKind
		wantContain []string
		wantAbsent  string
	}{
		{
			name:        "deployment",
			owner:       k8s.OwnerDeployment,
			wantContain: []string{"label the pod template", "kubectl patch deployment nginx-demo", templatePatch},
			wantAbsent:  "kubectl label pod",
		},
		{
			name:        "daemonset",
			owner:       k8s.OwnerDaemonSet,
			wantContain: []string{"label the pod template", "kubectl patch daemonset nginx-demo", templatePatch},
			wantAbsent:  "kubectl label pod",
		},
		{
			name:        "bare pod",
			owner:       k8s.OwnerNone,
			wantContain: []string{"label the target pod", "kubectl label pod nginx-demo podlock.kubewarden.io/profile=nginx-demo"},
			wantAbsent:  "kubectl patch",
		},
		{
			name:        "statefulset falls back to pod label",
			owner:       k8s.OwnerStatefulSet,
			wantContain: []string{"label the target pod", "kubectl label pod nginx-demo podlock.kubewarden.io/profile=nginx-demo"},
			wantAbsent:  "kubectl patch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podLockLabelHint(tt.owner, "nginx-demo")
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("hint %q does not contain %q", got, want)
				}
			}
			if strings.Contains(got, tt.wantAbsent) {
				t.Errorf("hint %q unexpectedly contains %q", got, tt.wantAbsent)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("hint %q should end with a newline", got)
			}
		})
	}
}
