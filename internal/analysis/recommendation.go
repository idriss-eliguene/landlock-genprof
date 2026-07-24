// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package analysis turns observed behavior (internal/profile) into
// product-facing, explainable security recommendations.
package analysis

import (
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// WorkloadRef identifies the analyzed workload target.
type WorkloadRef struct {
	Namespace string
	Pod       string
	Container string
	Binary    string
}

// DomainRecommendation summarizes recommendation coverage per domain.
type DomainRecommendation struct {
	Domain          string // filesystem, network, syscalls, hardening
	RequiredCount   int
	ExcludedLowConf int
	Backend         string // podlock, networkpolicy, spo, securitycontext
	Available       bool
}

// Evidence explains why a recommendation exists.
type Evidence struct {
	Reason       string
	ObservedRuns int
	TotalRuns    int
	Confidence   profile.Confidence
}

// RecommendationItem is one explainable recommendation.
type RecommendationItem struct {
	ID          string
	Title       string
	Description string
	Evidence    Evidence
}

// SecurityRecommendation is the product-facing analysis output.
type SecurityRecommendation struct {
	Workload          WorkloadRef
	TrainingRuns      int
	OverallConfidence int // 0-100
	Domains           []DomainRecommendation
	Items             []RecommendationItem
	GeneratedAt       time.Time
}

// BuildSecurityRecommendation creates an explainable recommendation from
// behavior evidence and run history.
func BuildSecurityRecommendation(workload WorkloadRef, behavior profile.BehaviorProfile, runsRecorded int) SecurityRecommendation {
	if runsRecorded < 1 {
		runsRecorded = 1
	}

	fsRequired := len(behavior.Filesystem.Accesses)
	netRequired := len(behavior.Network.Accesses)
	sysRequired := len(behavior.Syscalls.Accesses)

	hardeningRequired := 0
	if len(behavior.Capabilities.Accesses) > 0 {
		hardeningRequired++
	}
	if len(behavior.Syscalls.Accesses) > 0 {
		hardeningRequired++
	}

	netLow := 0
	for _, access := range behavior.Network.Accesses {
		if access.Confidence == profile.ConfidenceLow {
			netLow++
		}
	}

	overall := confidenceScore(behavior)

	domains := []DomainRecommendation{
		{Domain: "filesystem", RequiredCount: fsRequired, Backend: "podlock", Available: fsRequired > 0},
		{Domain: "network", RequiredCount: netRequired, ExcludedLowConf: netLow, Backend: "networkpolicy", Available: netRequired > 0},
		{Domain: "syscalls", RequiredCount: sysRequired, Backend: "spo", Available: sysRequired > 0},
		{Domain: "hardening", RequiredCount: hardeningRequired, Backend: "securitycontext", Available: hardeningRequired > 0},
	}

	items := make([]RecommendationItem, 0, len(behavior.Network.Accesses)+len(behavior.Capabilities.Accesses))
	for _, access := range behavior.Network.Accesses {
		items = append(items, RecommendationItem{
			ID:          fmt.Sprintf("network-egress-%d", access.Port),
			Title:       fmt.Sprintf("Allow TCP port %d", access.Port),
			Description: "Observed runtime connectivity requires this destination port.",
			Evidence: Evidence{
				Reason:       fmt.Sprintf("Container contacted TCP port %d", access.Port),
				ObservedRuns: min(access.SeenCount, runsRecorded),
				TotalRuns:    runsRecorded,
				Confidence:   access.Confidence,
			},
		})
	}
	for _, access := range behavior.Capabilities.Accesses {
		capName := access.Name
		if len(capName) > 4 && capName[:4] == "CAP_" {
			capName = capName[4:]
		}
		items = append(items, RecommendationItem{
			ID:          "cap-" + capName,
			Title:       "Add capability " + capName,
			Description: "Observed capability checks indicate this permission is required.",
			Evidence: Evidence{
				Reason:       fmt.Sprintf("Kernel capability check observed: %s", access.Name),
				ObservedRuns: min(access.SeenCount, runsRecorded),
				TotalRuns:    runsRecorded,
				Confidence:   access.Confidence,
			},
		})
	}

	return SecurityRecommendation{
		Workload:          workload,
		TrainingRuns:      runsRecorded,
		OverallConfidence: overall,
		Domains:           domains,
		Items:             items,
		GeneratedAt:       time.Now().UTC(),
	}
}

func confidenceScore(behavior profile.BehaviorProfile) int {
	total := 0
	weight := 0

	add := func(c profile.Confidence) {
		w := 40
		switch c {
		case profile.ConfidenceHigh:
			w = 100
		case profile.ConfidenceMedium:
			w = 70
		case profile.ConfidenceLow:
			w = 40
		}
		total += w
		weight++
	}

	for _, a := range behavior.Filesystem.Accesses {
		add(a.Confidence)
	}
	for _, a := range behavior.Network.Accesses {
		add(a.Confidence)
	}
	for _, a := range behavior.Syscalls.Accesses {
		add(a.Confidence)
	}
	for _, a := range behavior.Capabilities.Accesses {
		add(a.Confidence)
	}

	if weight == 0 {
		return 0
	}
	return total / weight
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
