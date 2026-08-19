// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package landlockjson converts a landlock.Candidate to and from a
// canonical JSON document — the second consumer of internal/landlock,
// proving its output is genuinely format-independent rather than
// implicitly PodLock-shaped (see docs/landlock-kernel-extraction.md's
// Phase 3).
//
// Unlike every other exporter in this codebase (podlock, networkpolicy,
// seccomp, capabilities, securitycontext — all one-way, IR to output
// format), this one is round-trip: FromJSON exists because `verify`
// needs to read a Candidate back from a file, not just produce one.
// This is deliberate and the reason this format exists at all — a
// PodLock LandlockProfile can't serve the same purpose, since its
// schema has no field for LandlockRightTruncate at all (categories are
// readOnly/readWrite/readExec/readWriteExec only): a candidate exported
// to PodLock and re-imported would have already and irreversibly lost
// the one right that currently makes `verify` non-trivial (see
// docs/landlock-kernel-extraction.md's "known gap" section).
//
// Own dedicated types (document/rule/evidence below), not json tags on
// landlock.Candidate/Rule/EvidenceRef directly — same reasoning
// pkg/podlock/pkg/seccomp/pkg/spo already give for keeping serialization
// concerns out of the types they serialize.
package landlockjson

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

// schemaVersion is this document format's own version — checked by
// FromJSON so a future incompatible schema change fails loudly instead
// of silently misreading old data.
const schemaVersion = "v1"

type document struct {
	Version string `json:"version"`
	Rules   []rule `json:"rules"`
}

type rule struct {
	Path       string     `json:"path"`
	Rights     []string   `json:"rights"`
	Confidence string     `json:"confidence,omitempty"`
	SeenCount  int        `json:"seenCount"`
	Evidence   []evidence `json:"evidence,omitempty"`
}

type evidence struct {
	Source    string    `json:"source,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ToJSON serializes a Candidate to the canonical JSON document format,
// indented for human readability (this is meant to be inspected and
// diffed, not just machine-consumed — matches every other exporter's
// own reviewability goal, see docs/threat-model.md).
func ToJSON(c landlock.Candidate) ([]byte, error) {
	doc := document{Version: schemaVersion, Rules: make([]rule, len(c.Rules))}
	for i, r := range c.Rules {
		rights := make([]string, len(r.Rights))
		for j, right := range r.Rights {
			rights[j] = string(right)
		}
		ev := make([]evidence, len(r.Evidence))
		for j, e := range r.Evidence {
			ev[j] = evidence{Source: e.Source, Timestamp: e.Timestamp}
		}
		doc.Rules[i] = rule{
			Path:       r.Path,
			Rights:     rights,
			Confidence: string(r.Confidence),
			SeenCount:  r.SeenCount,
			Evidence:   ev,
		}
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling candidate: %w", err)
	}
	return data, nil
}

// FromJSON parses a canonical JSON document (see ToJSON) back into a
// Candidate — the exact inverse, field for field, checked by
// TestToJSON_FromJSON_RoundTrips.
func FromJSON(data []byte) (landlock.Candidate, error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return landlock.Candidate{}, fmt.Errorf("unmarshaling candidate: %w", err)
	}
	if doc.Version != schemaVersion {
		return landlock.Candidate{}, fmt.Errorf(
			"unsupported candidate schema version %q (this build understands %q)", doc.Version, schemaVersion)
	}

	rules := make([]landlock.Rule, len(doc.Rules))
	for i, r := range doc.Rules {
		rights := make([]landlock.LandlockRight, len(r.Rights))
		for j, right := range r.Rights {
			rights[j] = landlock.LandlockRight(right)
		}
		ev := make([]landlock.EvidenceRef, len(r.Evidence))
		for j, e := range r.Evidence {
			ev[j] = landlock.EvidenceRef{Source: e.Source, Timestamp: e.Timestamp}
		}
		rules[i] = landlock.Rule{
			Path:       r.Path,
			Rights:     rights,
			Confidence: landlock.Confidence(r.Confidence),
			SeenCount:  r.SeenCount,
			Evidence:   ev,
		}
	}

	return landlock.Candidate{Rules: rules}, nil
}
