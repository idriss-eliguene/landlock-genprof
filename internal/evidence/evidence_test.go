// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package evidence

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func mockEvents() ([]tracer.Event, []string) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/etc/nginx/nginx.conf", Mode: "read", Timestamp: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
		{Syscall: "openat", Path: "/usr/sbin/nginx", Mode: "exec", Timestamp: time.Date(2026, 8, 6, 12, 0, 1, 0, time.UTC)},
		{
			Syscall: "openat", Path: "/var/cache/nginx", Mode: "write", IsDir: true, Truncate: true,
			Timestamp: time.Date(2026, 8, 6, 12, 0, 2, 0, time.UTC),
		},
		{Syscall: "connect", Port: 443, Mode: "egress", Timestamp: time.Date(2026, 8, 6, 12, 0, 3, 0, time.UTC)},
	}
	return events, []string{"SCMP_ARCH_X86_64"}
}

func TestToJSON_ContainsExpectedFields(t *testing.T) {
	events, architectures := mockEvents()
	out, err := ToJSON(events, architectures)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	text := string(out)
	for _, want := range []string{
		`"version": "v1"`,
		`"SCMP_ARCH_X86_64"`,
		`"path": "/etc/nginx/nginx.conf"`,
		`"mode": "exec"`,
		`"isDir": true`,
		`"truncate": true`,
		`"port": 443`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ToJSON() output missing %q:\n%s", want, text)
		}
	}
}

func TestFromJSON_ToJSON_RoundTrips(t *testing.T) {
	events, architectures := mockEvents()

	data, err := ToJSON(events, architectures)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	roundTrippedEvents, roundTrippedArchitectures, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}

	if !reflect.DeepEqual(events, roundTrippedEvents) {
		t.Errorf("round-tripped events mismatch:\noriginal     = %+v\nroundTripped = %+v", events, roundTrippedEvents)
	}
	if !reflect.DeepEqual(architectures, roundTrippedArchitectures) {
		t.Errorf("round-tripped architectures mismatch:\noriginal     = %v\nroundTripped = %v", architectures, roundTrippedArchitectures)
	}
}

func TestFromJSON_RejectsWrongSchemaVersion(t *testing.T) {
	_, _, err := FromJSON([]byte(`{"version": "v99", "events": []}`))
	if err == nil {
		t.Fatal("FromJSON() error = nil, want an error for an unsupported schema version")
	}
	if !strings.Contains(err.Error(), "v99") {
		t.Errorf("error = %v, want it to mention the unsupported version", err)
	}
}

func TestFromJSON_RejectsMalformedJSON(t *testing.T) {
	_, _, err := FromJSON([]byte(`not json`))
	if err == nil {
		t.Fatal("FromJSON() error = nil, want an error for malformed input")
	}
}

func TestToJSON_EmptyEvents(t *testing.T) {
	out, err := ToJSON(nil, nil)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	events, architectures, err := FromJSON(out)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
	if len(architectures) != 0 {
		t.Errorf("len(architectures) = %d, want 0", len(architectures))
	}
}
