// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package evidence persists a training run's raw tracer.Event stream to
// and from a canonical JSON document — the artifact `trace --events-out`
// writes and `synthesize --events-file` reads, sitting one stage earlier
// than internal/exporter/landlockjson's Candidate documents (see
// docs/cli-design.md: evidence -> synthesis -> verification -> ...).
//
// Round-trip (ToJSON/FromJSON), like internal/exporter/landlockjson and
// for the same reason: this format exists specifically to be read back,
// not just produced. Deliberately not "internal/exporter/eventsjson" —
// this isn't a Behavior IR or Candidate export, it's the raw capture
// stage the CLI design's own "evidence" noun group is named after; a
// tracer.Event is the input to synthesis, never its output.
//
// Own dedicated types (document/event below), not json tags on
// tracer.Event directly — same reasoning pkg/podlock/pkg/seccomp/
// internal/exporter/landlockjson already give for keeping serialization
// concerns out of the types they serialize. internal/tracer itself stays
// free of any encoding/json dependency.
package evidence

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// schemaVersion is this document format's own version — checked by
// FromJSON so a future incompatible schema change fails loudly instead
// of silently misreading old data, same discipline as landlockjson's own
// schemaVersion.
const schemaVersion = "v1"

type document struct {
	Version       string   `json:"version"`
	Architectures []string `json:"architectures,omitempty"`
	Events        []event  `json:"events"`
}

type event struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Syscall   string    `json:"syscall,omitempty"`
	Path      string    `json:"path,omitempty"`
	Port      int       `json:"port,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	IsDir     bool      `json:"isDir,omitempty"`
	Truncate  bool      `json:"truncate,omitempty"`
}

// ToJSON serializes a training run's captured events and architectures to
// the canonical evidence document format, indented for human
// readability — meant to be inspected and diffed, matching every other
// artifact this project produces (docs/threat-model.md).
func ToJSON(events []tracer.Event, architectures []string) ([]byte, error) {
	doc := document{
		Version:       schemaVersion,
		Architectures: architectures,
		Events:        make([]event, len(events)),
	}
	for i, ev := range events {
		doc.Events[i] = event{
			Timestamp: ev.Timestamp,
			Syscall:   ev.Syscall,
			Path:      ev.Path,
			Port:      ev.Port,
			Mode:      ev.Mode,
			IsDir:     ev.IsDir,
			Truncate:  ev.Truncate,
		}
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling evidence: %w", err)
	}
	return data, nil
}

// FromJSON parses a canonical evidence document (see ToJSON) back into
// events and architectures — the exact inverse, field for field, checked
// by TestFromJSON_ToJSON_RoundTrips.
func FromJSON(data []byte) (events []tracer.Event, architectures []string, err error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("unmarshaling evidence: %w", err)
	}
	if doc.Version != schemaVersion {
		return nil, nil, fmt.Errorf(
			"unsupported evidence schema version %q (this build understands %q)", doc.Version, schemaVersion)
	}

	events = make([]tracer.Event, len(doc.Events))
	for i, ev := range doc.Events {
		events[i] = tracer.Event{
			Timestamp: ev.Timestamp,
			Syscall:   ev.Syscall,
			Path:      ev.Path,
			Port:      ev.Port,
			Mode:      ev.Mode,
			IsDir:     ev.IsDir,
			Truncate:  ev.Truncate,
		}
	}
	return events, doc.Architectures, nil
}
