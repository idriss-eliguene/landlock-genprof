// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package landlockjson

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

func mockCandidate() landlock.Candidate {
	return landlock.Candidate{
		Rules: []landlock.Rule{
			{
				Path:       "/etc/nginx",
				Rights:     []landlock.LandlockRight{landlock.LandlockRightReadFile, landlock.LandlockRightReadDir},
				Confidence: landlock.ConfidenceHigh,
				SeenCount:  3,
				Evidence: []landlock.EvidenceRef{
					{Source: "run-1", Timestamp: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
					{Source: "run-1", Timestamp: time.Date(2026, 8, 6, 12, 0, 1, 0, time.UTC)},
					{Source: "run-2", Timestamp: time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC)},
				},
			},
			{
				Path:       "/var/lib/app/state.db",
				Rights:     []landlock.LandlockRight{landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate},
				Confidence: landlock.ConfidenceLow,
				SeenCount:  1,
				Evidence:   []landlock.EvidenceRef{{Source: "run-1", Timestamp: time.Date(2026, 8, 6, 12, 0, 2, 0, time.UTC)}},
			},
		},
	}
}

func TestToJSON_ContainsExpectedFields(t *testing.T) {
	out, err := ToJSON(mockCandidate())
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	text := string(out)
	for _, want := range []string{
		`"version": "v1"`,
		`"path": "/etc/nginx"`,
		`"read_file"`,
		`"read_dir"`,
		`"truncate"`,
		`"confidence": "high"`,
		`"seenCount": 3`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ToJSON() output missing %q:\n%s", want, text)
		}
	}
}

func TestFromJSON_ToJSON_RoundTrips(t *testing.T) {
	original := mockCandidate()

	data, err := ToJSON(original)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	roundTripped, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}

	if !reflect.DeepEqual(original, roundTripped) {
		t.Errorf("round-trip mismatch:\noriginal     = %+v\nroundTripped = %+v", original, roundTripped)
	}
}

func TestFromJSON_RejectsWrongSchemaVersion(t *testing.T) {
	_, err := FromJSON([]byte(`{"version": "v99", "rules": []}`))
	if err == nil {
		t.Fatal("FromJSON() error = nil, want an error for an unsupported schema version")
	}
	if !strings.Contains(err.Error(), "v99") {
		t.Errorf("error = %v, want it to mention the unsupported version", err)
	}
}

func TestFromJSON_RejectsMalformedJSON(t *testing.T) {
	_, err := FromJSON([]byte(`not json`))
	if err == nil {
		t.Fatal("FromJSON() error = nil, want an error for malformed input")
	}
}

func TestToJSON_EmptyCandidate(t *testing.T) {
	out, err := ToJSON(landlock.Candidate{})
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	roundTripped, err := FromJSON(out)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}
	if len(roundTripped.Rules) != 0 {
		t.Errorf("len(Rules) = %d, want 0", len(roundTripped.Rules))
	}
}
