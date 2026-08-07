// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package sarif

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToJSON_EmptyResults_ProducesEmptyArrayNotNull(t *testing.T) {
	data, err := ToJSON(Meta{ToolName: "landlock-genprof"}, nil, nil)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var decoded struct {
		Runs []struct {
			Results []sarifResult `json:"results"`
			Tool    struct {
				Driver struct {
					Rules []sarifRule `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.Runs[0].Results == nil {
		t.Error("results should be an empty array, not null, for a clean run")
	}
	if decoded.Runs[0].Tool.Driver.Rules == nil {
		t.Error("driver.rules should be an empty array, not null")
	}

	if !strings.Contains(string(data), `"results": []`) {
		t.Errorf("expected an explicit empty results array in the raw JSON:\n%s", data)
	}
}

func TestToJSON_ValidSARIFShape(t *testing.T) {
	data, err := ToJSON(
		Meta{ToolName: "landlock-genprof", ToolVersion: "v0.1.3", InformationURI: "https://example.invalid"},
		[]Rule{{ID: "landlock-abi-incompatible", ShortDescription: "needs a right unavailable at the target ABI"}},
		[]Result{{RuleID: "landlock-abi-incompatible", Path: "/var/lib/app/state.db", Message: "needs TRUNCATE (ABI 3)"}},
	)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded["version"] != "2.1.0" {
		t.Errorf("version = %v, want \"2.1.0\"", decoded["version"])
	}
	if decoded["$schema"] == nil {
		t.Error("missing $schema")
	}

	out := string(data)
	for _, want := range []string{
		`"name": "landlock-genprof"`,
		`"version": "v0.1.3"`,
		`"ruleId": "landlock-abi-incompatible"`,
		`"level": "error"`,
		`/var/lib/app/state.db`,
		`needs TRUNCATE (ABI 3)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
