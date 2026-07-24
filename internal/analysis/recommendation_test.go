// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package analysis

import (
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func TestBuildSecurityRecommendation_BasicShape(t *testing.T) {
	behavior := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{{
			Path:       "/etc/nginx",
			Confidence: profile.ConfidenceHigh,
			SeenCount:  3,
		}}},
		Network: profile.NetworkProfile{Accesses: []profile.NetworkAccess{{
			Port:       443,
			Direction:  profile.DirectionEgress,
			Confidence: profile.ConfidenceMedium,
			SeenCount:  2,
		}}},
		Syscalls: profile.SyscallProfile{Accesses: []profile.SyscallAccess{{
			Name:       "openat",
			Confidence: profile.ConfidenceLow,
			SeenCount:  1,
		}}},
		Capabilities: profile.CapabilityProfile{Accesses: []profile.CapabilityAccess{{
			Name:       "CAP_NET_BIND_SERVICE",
			Confidence: profile.ConfidenceHigh,
			SeenCount:  2,
		}}},
	}

	rec := BuildSecurityRecommendation(WorkloadRef{
		Namespace: "default",
		Pod:       "payment-api",
		Container: "app",
		Binary:    "/app/server",
	}, behavior, 5)

	if rec.Workload.Pod != "payment-api" {
		t.Fatalf("Workload.Pod = %q, want payment-api", rec.Workload.Pod)
	}
	if rec.TrainingRuns != 5 {
		t.Fatalf("TrainingRuns = %d, want 5", rec.TrainingRuns)
	}
	if len(rec.Domains) != 4 {
		t.Fatalf("len(Domains) = %d, want 4", len(rec.Domains))
	}
	if rec.OverallConfidence <= 0 || rec.OverallConfidence > 100 {
		t.Fatalf("OverallConfidence = %d, want 1..100", rec.OverallConfidence)
	}
	if len(rec.Items) == 0 {
		t.Fatal("Items is empty, want explainable recommendations")
	}
}
