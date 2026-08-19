// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package sarif renders a verification pass's findings as a SARIF
// 2.1.0 log (https://docs.oasis-open.org/sarif/sarif/v2.1.0/) — the
// format GitHub Code Scanning and most CI dashboards already know how to
// annotate, so `verify --output sarif` plugs into existing pipelines
// instead of asking every consumer to parse this project's own text
// output (docs/cli-design.md, Phase 3 — CI/CD integration).
//
// Deliberately generic (Rule/Result, not anything ABI-specific): `verify`
// is a pipeline of independently testable passes (see that command's own
// doc comment), and the plugin architecture note in docs/cli-design.md
// says future passes register themselves rather than each inventing
// their own output format — this package is what every current and
// future pass's findings render through, not something owned by the
// ABI-compatibility pass alone.
//
// Findings use logicalLocations, never physicalLocation/artifactLocation:
// a Result's Path is a runtime filesystem path inside the traced
// container (e.g. /etc/nginx/nginx.conf), not a file in the repository
// or PR being scanned. Claiming it as a physical artifact location would
// imply GitHub Code Scanning can annotate a source line for it, which it
// can't — logicalLocations says "this finding is about this path"
// without that false promise.
package sarif

import (
	"encoding/json"
	"fmt"
)

const schemaURI = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"

// Meta identifies the tool producing the log — every SARIF consumer's UI
// (GitHub included) shows this alongside each finding.
type Meta struct {
	ToolName       string
	ToolVersion    string
	InformationURI string
}

// Rule describes one finding category a pass can report — SARIF's own
// term, unrelated to a Landlock rule. Referenced by Result.RuleID.
type Rule struct {
	ID               string
	ShortDescription string
}

// Result is one finding: Path is the runtime location it's about (see
// package doc), not a repository file.
type Result struct {
	RuleID  string
	Path    string
	Message string
}

// ToJSON renders a verification pass's findings as a SARIF 2.1.0 log,
// indented for human readability when inspected directly — matching
// every other artifact this project produces (docs/threat-model.md).
// An empty results slice still produces a valid, well-formed log (an
// empty "results" array) — the expected shape for a clean run, not an
// error.
func ToJSON(meta Meta, rules []Rule, results []Result) ([]byte, error) {
	sarifRules := make([]sarifRule, len(rules))
	for i, r := range rules {
		sarifRules[i] = sarifRule{
			ID:               r.ID,
			ShortDescription: sarifText{Text: r.ShortDescription},
		}
	}

	sarifResults := make([]sarifResult, len(results))
	for i, r := range results {
		sarifResults[i] = sarifResult{
			RuleID:  r.RuleID,
			Level:   "error",
			Message: sarifText{Text: r.Message},
			Locations: []sarifLocation{{
				LogicalLocations: []sarifLogicalLocation{{Name: r.Path}},
			}},
		}
	}

	log := sarifLog{
		Schema:  schemaURI,
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           meta.ToolName,
				InformationURI: meta.InformationURI,
				Version:        meta.ToolVersion,
				Rules:          sarifRules,
			}},
			Results: sarifResults,
		}},
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling SARIF log: %w", err)
	}
	return data, nil
}

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri,omitempty"`
	Version        string      `json:"version,omitempty"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	ShortDescription sarifText `json:"shortDescription"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifLocation struct {
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations"`
}

type sarifLogicalLocation struct {
	Name string `json:"name"`
}
