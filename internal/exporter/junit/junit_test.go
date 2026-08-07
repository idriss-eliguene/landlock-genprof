// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package junit

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestToXML_EmptyCases_ValidSuite(t *testing.T) {
	data, err := ToXML(Meta{SuiteName: "landlock-genprof diff"}, nil)
	if err != nil {
		t.Fatalf("ToXML() error = %v", err)
	}

	var decoded xmlTestSuites
	if err := xml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v", err)
	}
	if len(decoded.Suites) != 1 {
		t.Fatalf("len(Suites) = %d, want 1", len(decoded.Suites))
	}
	if decoded.Suites[0].Tests != 0 || decoded.Suites[0].Failures != 0 {
		t.Errorf("tests=%d failures=%d, want 0/0", decoded.Suites[0].Tests, decoded.Suites[0].Failures)
	}
	if !strings.HasPrefix(string(data), xml.Header) {
		t.Errorf("output should start with the XML header:\n%s", data)
	}
}

func TestToXML_MixedPassFail(t *testing.T) {
	data, err := ToXML(Meta{SuiteName: "landlock-genprof diff"}, []TestCase{
		{ClassName: "diff", Name: "/etc/nginx"},
		{ClassName: "diff", Name: "/var/lib/app/state.db", Failure: "+[truncate]"},
	})
	if err != nil {
		t.Fatalf("ToXML() error = %v", err)
	}

	var decoded xmlTestSuites
	if err := xml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v", err)
	}
	suite := decoded.Suites[0]
	if suite.Tests != 2 {
		t.Errorf("tests = %d, want 2", suite.Tests)
	}
	if suite.Failures != 1 {
		t.Errorf("failures = %d, want 1", suite.Failures)
	}
	if suite.Cases[0].Failure != nil {
		t.Errorf("first case should have no failure, got %+v", suite.Cases[0].Failure)
	}
	if suite.Cases[1].Failure == nil || suite.Cases[1].Failure.Message != "+[truncate]" {
		t.Errorf("second case failure = %+v, want message \"+[truncate]\"", suite.Cases[1].Failure)
	}

	out := string(data)
	for _, want := range []string{
		`name="landlock-genprof diff"`,
		`classname="diff"`,
		`name="/var/lib/app/state.db"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
