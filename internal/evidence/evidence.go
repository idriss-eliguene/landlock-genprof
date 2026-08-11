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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// Supported schema versions
const (
	schemaVersionV1 = "v1"
	schemaVersionV2 = "v2"
)

var (
	ErrUnsupportedEvidenceVersion = errors.New("unsupported evidence schema version")
	ErrInvalidRunMetadata         = errors.New("invalid run metadata")
	ErrInvalidProvenance          = errors.New("invalid provenance metadata")
)

// Event is the JSON-serializable representation of a single captured event.
// It mirrors tracer.Event but carries omitempty JSON tags for compactness.
type OriginType string

const (
	OriginDirect   OriginType = "direct"
	OriginDerived  OriginType = "derived"
	OriginAdvisory OriginType = "advisory"
	OriginImported OriginType = "imported"
	OriginUnknown  OriginType = "unknown"
)

// ProvenanceSource is a document-local description of an evidence source.
// It is deliberately minimal and opaque; presence of BackendAgentID does
// not confer SubjectIdentity or authentication by itself.
type ProvenanceSource struct {
	BackendKind      string     `json:"backendKind"`
	OriginType       OriginType `json:"originType"`
	BackendAgentID   string     `json:"backendAgentId,omitempty"`
	CollectorVersion string     `json:"collectorVersion,omitempty"`
}

// Event is the JSON-serializable representation of a single captured event.
// It mirrors tracer.Event but carries omitempty JSON tags for compactness.
// It is extended with an optional provenance reference that is document-
// local and must refer to a key in Document.ProvenanceSources when present.
type Event struct {
	Timestamp    time.Time `json:"timestamp,omitempty"`
	Syscall      string    `json:"syscall,omitempty"`
	Path         string    `json:"path,omitempty"`
	Port         int       `json:"port,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	IsDir        bool      `json:"isDir,omitempty"`
	Truncate     bool      `json:"truncate,omitempty"`
	ProvenanceID string    `json:"provenanceId,omitempty"`
}

// RunMetadata carries the minimal run-level metadata required for exact
// semantic reconstruction. It intentionally uses primitive types so the
// evidence package doesn't import semantic types.
type RunMetadata struct {
	Source     string     `json:"source"`
	RecordTime time.Time  `json:"recordTime"`
	Start      *time.Time `json:"start,omitempty"`
	End        *time.Time `json:"end,omitempty"`
	// Optional passive provenance/audit metadata
	RunID                     string `json:"runId,omitempty"`
	SemanticProjectionVersion string `json:"semanticProjectionVersion,omitempty"`
}

// Document is the versioned evidence envelope. Version must be either
// "v1" or "v2". For v1 the Run field is nil.
type Document struct {
	Version           string                      `json:"version"`
	Run               *RunMetadata                `json:"run,omitempty"`
	Architectures     []string                    `json:"architectures,omitempty"`
	ProvenanceSources map[string]ProvenanceSource `json:"provenanceSources,omitempty"`
	Events            []Event                     `json:"events"`
}

// Decode parses either a v1 or v2 evidence document and returns a
// Document. It validates the version and performs basic structural
// checks for v2 run metadata. It also performs focused provenance
// validation: duplicate JSON object keys in provenance structures are
// rejected and explicit empty provenanceId in events is rejected.
func Decode(data []byte) (Document, error) {
	// First pass: focused stream-scan to detect duplicate provenance keys
	// and explicit-empty provenanceId occurrences which encoding/json
	// would otherwise normalize away.
	dec := json.NewDecoder(bytes.NewReader(data))
	// top-level must be an object
	tok, err := dec.Token()
	if err != nil {
		return Document{}, fmt.Errorf("unmarshaling evidence: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return Document{}, fmt.Errorf("unmarshaling evidence: top-level JSON object required")
	}

	// helper: check for duplicate keys in a raw JSON object represented by raw bytes
	checkObjectDuplicates := func(raw json.RawMessage) error {
		ndec := json.NewDecoder(bytes.NewReader(raw))
		tok, err := ndec.Token()
		if err != nil {
			return err
		}
		d, ok := tok.(json.Delim)
		if !ok || d != '{' {
			return fmt.Errorf("expected object")
		}
		seen := map[string]struct{}{}
		for ndec.More() {
			kTok, err := ndec.Token()
			if err != nil {
				return err
			}
			k := kTok.(string)
			if _, exists := seen[k]; exists {
				return ErrInvalidProvenance
			}
			seen[k] = struct{}{}
			// skip value
			var skip json.RawMessage
			if err := ndec.Decode(&skip); err != nil {
				return err
			}
		}
		return nil
	}

	// scan top-level keys
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return Document{}, fmt.Errorf("unmarshaling evidence: %w", err)
		}
		key := keyTok.(string)
		switch key {
		case "provenanceSources":
			// read raw value
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return Document{}, fmt.Errorf("unmarshaling provenanceSources: %w", err)
			}
			// provenanceSources must be an object; iterate entries and detect duplicate keys
			// at the provenanceSources level: two identical pIDs must not appear twice.
			// Use decoder over raw to visit entries and ensure no duplicate member names.
			pdec := json.NewDecoder(bytes.NewReader(raw))
			tok, err := pdec.Token()
			if err != nil {
				return Document{}, fmt.Errorf("unmarshaling provenanceSources: %w", err)
			}
			delim, ok := tok.(json.Delim)
			if !ok || delim != '{' {
				return Document{}, fmt.Errorf("malformed provenanceSources: object required")
			}
			seenKeys := map[string]struct{}{}
			for pdec.More() {
				kTok, err := pdec.Token()
				if err != nil {
					return Document{}, fmt.Errorf("unmarshaling provenanceSources: %w", err)
				}
				pkey := kTok.(string)
				if _, exists := seenKeys[pkey]; exists {
					return Document{}, fmt.Errorf("%w: duplicate provenanceSources key %q", ErrInvalidProvenance, pkey)
				}
				seenKeys[pkey] = struct{}{}
				// read the raw value for this provenance source
				var srcRaw json.RawMessage
				if err := pdec.Decode(&srcRaw); err != nil {
					return Document{}, fmt.Errorf("unmarshaling provenance source %q: %w", pkey, err)
				}
				// check inner-object duplicate keys (e.g., backendKind repeated)
				if err := checkObjectDuplicates(srcRaw); err != nil {
					return Document{}, fmt.Errorf("%w: provenance source %q has duplicate fields", ErrInvalidProvenance, pkey)
				}
			}
			// consume trailing '}'
			if _, err := pdec.Token(); err != nil && err != io.EOF {
				// ignore EOF if present
				return Document{}, fmt.Errorf("unmarshaling provenanceSources: %w", err)
			}
		case "events":
			// scan events array to detect explicit-empty provenanceId
			var rawEvents json.RawMessage
			if err := dec.Decode(&rawEvents); err != nil {
				return Document{}, fmt.Errorf("unmarshaling events: %w", err)
			}
			edec := json.NewDecoder(bytes.NewReader(rawEvents))
			tok, err := edec.Token()
			if err != nil {
				return Document{}, fmt.Errorf("unmarshaling events: %w", err)
			}
			d, ok := tok.(json.Delim)
			if !ok || d != '[' {
				return Document{}, fmt.Errorf("malformed events: array required")
			}
			// for each event, inspect keys
			for edec.More() {
				var evRaw json.RawMessage
				if err := edec.Decode(&evRaw); err != nil {
					return Document{}, fmt.Errorf("unmarshaling event: %w", err)
				}
				// scan object properties
				ndec := json.NewDecoder(bytes.NewReader(evRaw))
				tok, err := ndec.Token()
				if err != nil {
					return Document{}, fmt.Errorf("unmarshaling event object: %w", err)
				}
				if dm, ok := tok.(json.Delim); !ok || dm != '{' {
					return Document{}, fmt.Errorf("malformed event: object required")
				}
				for ndec.More() {
					kTok, err := ndec.Token()
					if err != nil {
						return Document{}, fmt.Errorf("unmarshaling event property: %w", err)
					}
					k := kTok.(string)
					// read value token
					vTok, err := ndec.Token()
					if err != nil {
						return Document{}, fmt.Errorf("unmarshaling event property value: %w", err)
					}
					if k == "provenanceId" {
						// explicit empty string is malformed per ADR-0005
						if s, ok := vTok.(string); ok && s == "" {
							return Document{}, fmt.Errorf("%w: explicit empty provenanceId", ErrInvalidProvenance)
						}
					}
				}
			}
			// consume trailing ']'
			if _, err := edec.Token(); err != nil && err != io.EOF {
				return Document{}, fmt.Errorf("unmarshaling events: %w", err)
			}
		default:
			// skip other values
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return Document{}, fmt.Errorf("unmarshaling: %w", err)
			}
		}
	}

	// Second pass: normal unmarshal into Document for existing validation
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("unmarshaling evidence: %w", err)
	}
	if doc.Version == "" {
		return Document{}, fmt.Errorf("%w: empty version", ErrUnsupportedEvidenceVersion)
	}
	switch doc.Version {
	case schemaVersionV1:
		// v1: nothing more to validate
		return doc, nil
	case schemaVersionV2:
		// v2 requires run metadata for exact replay
		if doc.Run == nil {
			return Document{}, fmt.Errorf("%w: missing run metadata for v2", ErrInvalidRunMetadata)
		}
		if doc.Run.Source == "" {
			return Document{}, fmt.Errorf("%w: missing run.source", ErrInvalidRunMetadata)
		}
		if doc.Run.RecordTime.IsZero() {
			return Document{}, fmt.Errorf("%w: missing run.recordTime", ErrInvalidRunMetadata)
		}
		if doc.Run.Start != nil && doc.Run.End != nil && doc.Run.Start.After(*doc.Run.End) {
			return Document{}, fmt.Errorf("%w: run.start > run.end", ErrInvalidRunMetadata)
		}
		// Validate provenance sources if present
		if len(doc.ProvenanceSources) > 0 {
			for k, ps := range doc.ProvenanceSources {
				if k == "" {
					return Document{}, fmt.Errorf("malformed provenance: empty provenance key")
				}
				if ps.BackendKind == "" {
					return Document{}, fmt.Errorf("malformed provenance: backendKind required for %q", k)
				}
				switch ps.OriginType {
				case OriginDirect, OriginDerived, OriginAdvisory, OriginImported, OriginUnknown:
					// ok
				default:
					return Document{}, fmt.Errorf("malformed provenance: invalid originType for %q", k)
				}
			}
		}
		// Validate event provenance references: if an Event.ProvenanceID is non-empty,
		// it MUST reference a key in ProvenanceSources. Note: encoding/json cannot
		// distinguish an explicitly empty string from an absent field. We treat an
		// empty string as absence here (legacy) because of unmarshal limitations.
		for i, ev := range doc.Events {
			if ev.ProvenanceID != "" {
				if doc.ProvenanceSources == nil {
					return Document{}, fmt.Errorf("malformed provenance: event index %d references unknown provenance %q", i, ev.ProvenanceID)
				}
				if _, ok := doc.ProvenanceSources[ev.ProvenanceID]; !ok {
					return Document{}, fmt.Errorf("malformed provenance: event index %d references unknown provenance %q", i, ev.ProvenanceID)
				}
			}
		}
		return doc, nil
	default:
		return Document{}, fmt.Errorf("%w: %q", ErrUnsupportedEvidenceVersion, doc.Version)
	}
}

// ToJSON serializes events/architectures using the legacy v1 envelope to
// preserve backward compatibility. Behavior unchanged from previous API.
func ToJSON(events []tracer.Event, architectures []string) ([]byte, error) {
	doc := Document{
		Version:       schemaVersionV1,
		Architectures: architectures,
		Events:        make([]Event, len(events)),
	}
	for i, ev := range events {
		doc.Events[i] = Event{
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

// Encode serializes a Document (v1 or v2) to JSON. Use ToJSON for v1
// compatibility; Encode is the general writer for v2 when callers need
// to include run metadata.
func Encode(doc Document) ([]byte, error) {
	if doc.Version == "" {
		return nil, fmt.Errorf("missing document version")
	}
	if doc.Version != schemaVersionV1 && doc.Version != schemaVersionV2 {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedEvidenceVersion, doc.Version)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling evidence: %w", err)
	}
	return data, nil
}

// ToJSONWithRun is a convenience writer producing a v2 envelope including
// the provided RunMetadata.
func ToJSONWithRun(events []tracer.Event, architectures []string, run RunMetadata) ([]byte, error) {
	doc := Document{
		Version:       schemaVersionV2,
		Run:           &run,
		Architectures: architectures,
		Events:        make([]Event, len(events)),
	}
	for i, ev := range events {
		doc.Events[i] = Event{
			Timestamp: ev.Timestamp,
			Syscall:   ev.Syscall,
			Path:      ev.Path,
			Port:      ev.Port,
			Mode:      ev.Mode,
			IsDir:     ev.IsDir,
			Truncate:  ev.Truncate,
		}
	}
	return Encode(doc)
}

// FromJSON preserves the original API: it delegates to Decode and
// returns only events and architectures for backward compatibility.
func FromJSON(data []byte) (events []tracer.Event, architectures []string, err error) {
	doc, err := Decode(data)
	if err != nil {
		return nil, nil, err
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
