// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package spoimport

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCoverageStates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		present bool
		state   CoverageState
		version string
	}{
		{"missing", "", false, CoverageAbsent, ""},
		{"semantically absent", "  ", true, CoverageAbsent, ""},
		{"known v1", `{"version":"v1","total":3,"syscalls":{"read":3,"write":1}}`, true, CoverageKnown, CoverageSchemaV1},
		{"invalid JSON", `{`, true, CoverageMalformed, ""},
		{"missing total", `{"version":"v1","syscalls":{}}`, true, CoverageMalformed, ""},
		{"missing syscalls", `{"version":"v1","total":3}`, true, CoverageMalformed, ""},
		{"unsupported", `{"version":"v2","total":3,"syscalls":{}}`, true, CoverageUnsupported, "v2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCoverage(tc.raw, tc.present)
			if got.State != tc.state || got.Version != tc.version {
				t.Fatalf("ParseCoverage() = %#v, want state=%s version=%q", got, tc.state, tc.version)
			}
		})
	}
}

func TestParseCoverageV1SemanticValidation(t *testing.T) {
	for _, raw := range []string{
		`{"version":"v1","total":0,"syscalls":{}}`,
		`{"version":"v1","total":-1,"syscalls":{}}`,
		`{"version":"v1","total":2,"syscalls":{"read":3}}`,
		`{"version":"v1","total":2,"syscalls":{"read":0}}`,
		`{"version":"v1","total":2,"syscalls":{"bad name":1}}`,
		`{"version":"v1","total":2,"syscalls":[]}`,
	} {
		if got := ParseCoverage(raw, true); got.State != CoverageMalformed {
			t.Errorf("ParseCoverage(%s).State = %s, want malformed", raw, got.State)
		}
	}
}

func TestCoverageCanonicalEquivalence(t *testing.T) {
	a := ParseCoverage(`{
  "version": "v1",
  "total": 3,
  "syscalls": {"write": 1, "read": 3}
}`, true)
	b := ParseCoverage(`{"syscalls":{"read":3,"write":1},"total":3,"version":"v1"}`, true)
	if !reflect.DeepEqual(a, b) || a.Canonical() != b.Canonical() {
		t.Fatalf("equivalent inputs did not normalize equally:\n%#v %s\n%#v %s", a, a.Canonical(), b, b.Canonical())
	}
	if strings.Contains(a.Canonical(), "\n") || strings.Index(a.Canonical(), "read") > strings.Index(a.Canonical(), "write") {
		t.Fatalf("canonical coverage is not compact and key-sorted: %s", a.Canonical())
	}
}

func TestParseCanonicalCoverageRoundTrip(t *testing.T) {
	for _, want := range []Coverage{
		{State: CoverageAbsent},
		{State: CoverageMalformed},
		{State: CoverageUnsupported, Version: "v2"},
		{State: CoverageKnown, Version: CoverageSchemaV1, Total: 2, Syscalls: map[string]int{"read": 2}},
	} {
		if got := ParseCanonicalCoverage(want.Canonical()); !reflect.DeepEqual(got, want) {
			t.Errorf("round trip = %#v, want %#v", got, want)
		}
	}
}
