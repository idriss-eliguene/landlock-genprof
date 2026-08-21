// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package spoimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

// CoverageState is landlock-genprof's interpretation state for SPO's optional,
// non-stable syscall-coverage annotation. It is provenance, never confidence.
type CoverageState string

const (
	CoverageAbsent      CoverageState = "absent"
	CoverageKnown       CoverageState = "known"
	CoverageMalformed   CoverageState = "malformed"
	CoverageUnsupported CoverageState = "unsupported"
	CoverageSchemaV1                  = "v1"
)

// Coverage is the normalized semantic value stored in governed provenance.
// Raw upstream bytes are deliberately absent.
type Coverage struct {
	State    CoverageState  `json:"state"`
	Version  string         `json:"version,omitempty"`
	Total    int            `json:"total,omitempty"`
	Syscalls map[string]int `json:"syscalls,omitempty"`
}

type coverageEnvelope struct {
	Version json.RawMessage `json:"version"`
}

type coverageV1 struct {
	Version  string         `json:"version"`
	Total    *int           `json:"total"`
	Syscalls map[string]int `json:"syscalls"`
}

var syscallName = regexp.MustCompile(`^[a-z0-9_]+$`)

// ParseCoverage classifies and normalizes SPO's optional annotation. Invalid or
// newer metadata never blocks policy import; callers bind the resulting state.
func ParseCoverage(raw string, present bool) Coverage {
	if !present || strings.TrimSpace(raw) == "" {
		return Coverage{State: CoverageAbsent}
	}

	var envelope coverageEnvelope
	if err := decodeJSON(raw, &envelope); err != nil || len(envelope.Version) == 0 {
		return Coverage{State: CoverageMalformed}
	}
	var version string
	if err := json.Unmarshal(envelope.Version, &version); err != nil || version == "" {
		return Coverage{State: CoverageMalformed}
	}
	if version != CoverageSchemaV1 {
		return Coverage{State: CoverageUnsupported, Version: version}
	}

	var value coverageV1
	if err := decodeJSON(raw, &value); err != nil || value.Version != CoverageSchemaV1 || value.Total == nil || value.Syscalls == nil {
		return Coverage{State: CoverageMalformed}
	}
	if *value.Total <= 0 {
		return Coverage{State: CoverageMalformed}
	}
	for name, count := range value.Syscalls {
		if !syscallName.MatchString(name) || count <= 0 || count > *value.Total {
			return Coverage{State: CoverageMalformed}
		}
	}
	return Coverage{State: CoverageKnown, Version: CoverageSchemaV1, Total: *value.Total, Syscalls: value.Syscalls}
}

func decodeJSON(raw string, into interface{}) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

// Canonical returns the only coverage representation allowed into governed
// provenance. encoding/json sorts map keys, so whitespace and input ordering
// cannot affect CandidateDigest.
func (c Coverage) Canonical() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err) // Coverage contains only JSON-native values.
	}
	return string(b)
}

// ParseCanonicalCoverage reads landlock-genprof-owned normalized provenance
// for review. Invalid stored data is reported as malformed, never inferred.
func ParseCanonicalCoverage(value string) Coverage {
	var coverage Coverage
	if value == "" || decodeJSON(value, &coverage) != nil {
		return Coverage{State: CoverageMalformed}
	}
	switch coverage.State {
	case CoverageAbsent, CoverageMalformed:
		return Coverage{State: coverage.State}
	case CoverageUnsupported:
		if coverage.Version == "" {
			return Coverage{State: CoverageMalformed}
		}
		return Coverage{State: CoverageUnsupported, Version: coverage.Version}
	case CoverageKnown:
		// Reuse the v1 semantic validator via its wire-equivalent fields.
		return ParseCoverage(coverage.CanonicalKnownInput(), true)
	default:
		return Coverage{State: CoverageMalformed}
	}
}

func (c Coverage) CanonicalKnownInput() string {
	b, _ := json.Marshal(coverageV1{Version: c.Version, Total: &c.Total, Syscalls: c.Syscalls})
	return string(b)
}
