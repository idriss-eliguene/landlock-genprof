// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package junit renders a set of pass/fail checks as JUnit XML — the
// format nearly every CI dashboard (Jenkins, GitLab, GitHub Actions test
// reporters) already knows how to render as a per-check pass/fail table,
// so `diff --output junit` plugs into existing pipelines instead of
// asking every consumer to parse this project's own text output
// (docs/cli-design.md, Phase 3 — CI/CD integration).
//
// Deliberately generic (TestCase, not anything diff-specific): sibling of
// internal/exporter/sarif for the same reason — a future command with
// its own pass/fail checks (a `policy` gate, say) renders through this
// same package rather than inventing its own JUnit writer.
package junit

import (
	"encoding/xml"
	"fmt"
)

// Meta names the suite — the one grouping label most JUnit viewers show
// above the list of cases.
type Meta struct {
	SuiteName string
}

// TestCase is one pass/fail check. Failure empty means it passed; a
// non-empty Failure is both the short reason (shown as the failure's
// summary in most viewers) and the case's full text.
type TestCase struct {
	ClassName string
	Name      string
	Failure   string
}

// ToXML renders cases as a single-suite JUnit XML document, indented for
// human readability like every other artifact this project produces
// (docs/threat-model.md). An empty cases slice still produces a valid
// document (an empty <testsuite>), the expected shape for "nothing to
// check," not an error.
func ToXML(meta Meta, cases []TestCase) ([]byte, error) {
	failures := 0
	xmlCases := make([]xmlTestCase, len(cases))
	for i, c := range cases {
		xmlCases[i] = xmlTestCase{ClassName: c.ClassName, Name: c.Name}
		if c.Failure != "" {
			failures++
			xmlCases[i].Failure = &xmlFailure{Message: c.Failure, Text: c.Failure}
		}
	}

	suites := xmlTestSuites{
		Suites: []xmlSuite{{
			Name:     meta.SuiteName,
			Tests:    len(cases),
			Failures: failures,
			Cases:    xmlCases,
		}},
	}

	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling JUnit XML: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

type xmlTestSuites struct {
	XMLName xml.Name   `xml:"testsuites"`
	Suites  []xmlSuite `xml:"testsuite"`
}

type xmlSuite struct {
	Name     string        `xml:"name,attr"`
	Tests    int           `xml:"tests,attr"`
	Failures int           `xml:"failures,attr"`
	Cases    []xmlTestCase `xml:"testcase"`
}

type xmlTestCase struct {
	ClassName string      `xml:"classname,attr"`
	Name      string      `xml:"name,attr"`
	Failure   *xmlFailure `xml:"failure,omitempty"`
}

type xmlFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}
