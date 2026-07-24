// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package report

import (
	"strings"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// mockNginxBehavior mirrors the shape internal/policy.Synthesize would
// produce for a typical nginx --restart training run — all four domains
// populated, matching the live-confirmed result from docs/e2e-demo.md
// Finding 5.
func mockNginxBehavior() profile.BehaviorProfile {
	return profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, Confidence: profile.ConfidenceHigh, SeenCount: 5},
		}},
		Network: profile.NetworkProfile{Accesses: []profile.NetworkAccess{
			{Port: 80, Direction: profile.DirectionIngress, Confidence: profile.ConfidenceHigh, SeenCount: 3},
		}},
		Syscalls: profile.SyscallProfile{
			Accesses:      []profile.SyscallAccess{{Name: "openat", Confidence: profile.ConfidenceLow, SeenCount: 1}},
			Architectures: []string{"SCMP_ARCH_X86_64"},
		},
		Capabilities: profile.CapabilityProfile{Accesses: []profile.CapabilityAccess{
			{Name: "CAP_SETUID", Confidence: profile.ConfidenceHigh, SeenCount: 1},
		}},
	}
}

func mockMeta() Meta {
	return Meta{
		Name:      "nginx-demo",
		Namespace: "default",
		Container: "nginx",
		Binary:    "/usr/sbin/nginx",
		Duration:  60 * time.Second,
	}
}

func TestToMarkdown_AllDomainsPopulated(t *testing.T) {
	out := string(ToMarkdown(mockMeta(), mockNginxBehavior(), GeneratedFiles{
		Profile:         "nginx-demo-profile.yaml",
		NetworkPolicy:   "nginx-demo-networkpolicy.yaml",
		Seccomp:         "nginx-demo-seccomp.json",
		Capabilities:    "nginx-demo-capabilities.yaml",
		SecurityContext: "nginx-demo-securitycontext.yaml",
	}))

	for _, want := range []string{
		"# Security Profile Review — nginx-demo",
		"/etc/nginx", "high",
		"80", "ingress",
		"openat", "low",
		"CAP_SETUID",
		"nginx-demo-profile.yaml",
		"nginx-demo-networkpolicy.yaml",
		"nginx-demo-seccomp.json",
		"nginx-demo-capabilities.yaml",
		"nginx-demo-securitycontext.yaml",
		"## Review checklist",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing expected content %q:\n%s", want, out)
		}
	}
}

// TestToMarkdown_EmptyCapabilities checks that an empty Capabilities
// domain produces the Finding 5 startup-blind-spot note, not a bare
// empty table.
func TestToMarkdown_EmptyCapabilities(t *testing.T) {
	behavior := mockNginxBehavior()
	behavior.Capabilities = profile.CapabilityProfile{}

	out := string(ToMarkdown(mockMeta(), behavior, GeneratedFiles{}))

	if !strings.Contains(out, "No capability checks observed") {
		t.Errorf("report missing the empty-capabilities note:\n%s", out)
	}
	if !strings.Contains(out, "Finding 5") {
		t.Errorf("report missing the Finding 5 reference:\n%s", out)
	}
	if !strings.Contains(out, "--restart") {
		t.Errorf("report missing the --restart suggestion:\n%s", out)
	}
}

// TestToMarkdown_EmptyNetworkAndSyscalls checks the plainer "not
// observed" notes for domains that don't have a dedicated Finding to
// reference (Network) alongside Syscalls' own startup-blind-spot note
// (Finding 2).
func TestToMarkdown_EmptyNetworkAndSyscalls(t *testing.T) {
	behavior := mockNginxBehavior()
	behavior.Network = profile.NetworkProfile{}
	behavior.Syscalls = profile.SyscallProfile{}

	out := string(ToMarkdown(mockMeta(), behavior, GeneratedFiles{}))

	if !strings.Contains(out, "No network activity observed") {
		t.Errorf("report missing the empty-network note:\n%s", out)
	}
	if !strings.Contains(out, "No syscalls observed") || !strings.Contains(out, "Finding 2") {
		t.Errorf("report missing the empty-syscalls / Finding 2 note:\n%s", out)
	}
}

// TestToMarkdown_HistoryUsedTogglesChecklist checks that the
// "--history"-recommending checklist line only appears when it wasn't
// already used this run.
func TestToMarkdown_HistoryUsedTogglesChecklist(t *testing.T) {
	behavior := mockNginxBehavior()

	withoutHistory := string(ToMarkdown(mockMeta(), behavior, GeneratedFiles{}))
	if !strings.Contains(withoutHistory, "Re-run with `--history`") {
		t.Errorf("expected the --history checklist line without HistoryUsed:\n%s", withoutHistory)
	}

	meta := mockMeta()
	meta.HistoryUsed = true
	withHistory := string(ToMarkdown(meta, behavior, GeneratedFiles{}))
	if strings.Contains(withHistory, "Re-run with `--history`") {
		t.Errorf("expected no --history checklist line with HistoryUsed:\n%s", withHistory)
	}
	if !strings.Contains(withHistory, "yes — Confidence below reflects the real cross-run ratio") {
		t.Errorf("expected the header to reflect HistoryUsed:\n%s", withHistory)
	}
}

// TestToMarkdown_SyscallLowConfidenceWarning checks the seccomp-specific
// "always Low without --history" note appears without --history and not
// with it.
func TestToMarkdown_SyscallLowConfidenceWarning(t *testing.T) {
	behavior := mockNginxBehavior()

	withoutHistory := string(ToMarkdown(mockMeta(), behavior, GeneratedFiles{}))
	if !strings.Contains(withoutHistory, "Confidence reflects only this run without `--history`") {
		t.Errorf("expected the syscall confidence warning without HistoryUsed:\n%s", withoutHistory)
	}

	meta := mockMeta()
	meta.HistoryUsed = true
	withHistory := string(ToMarkdown(meta, behavior, GeneratedFiles{}))
	if strings.Contains(withHistory, "Confidence reflects only this run without `--history`") {
		t.Errorf("expected no syscall confidence warning with HistoryUsed:\n%s", withHistory)
	}
}
