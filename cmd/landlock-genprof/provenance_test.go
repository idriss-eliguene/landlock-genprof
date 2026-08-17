package main

import (
	"os"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/evidence"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
	adpt "github.com/idriss-eliguene/landlock-genprof/internal/semantic/adapter"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func makeRunMeta() *adpt.RunMeta {
	r := adpt.RunMeta{Source: semantic.NewSubjectIdentity("landlock-genprof-test"), RecordTime: time.Now().UTC()}
	return &r
}

func TestWriteEventsJSON_ProvenanceDistinctAndDedup(t *testing.T) {
	f, err := os.CreateTemp("", "events-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// two events with identical payload but different provenance
	ev1 := tracer.Event{Syscall: "openat", Mode: "syscall", Path: "/a"}
	ev1.Provenance = &tracer.ProvenanceDescriptor{BackendKind: "trace_open", OriginType: "direct"}
	ev2 := tracer.Event{Syscall: "openat", Mode: "syscall", Path: "/a"}
	ev2.Provenance = &tracer.ProvenanceDescriptor{BackendKind: "advise_seccomp", OriginType: "advisory"}

	events := []tracer.Event{ev1, ev2}

	if err := writeEventsJSON(os.Stdout, f.Name(), events, nil, makeRunMeta()); err != nil {
		t.Fatalf("writeEventsJSON: %v", err)
	}

	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	doc, err := evidence.Decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Version != "v2" {
		t.Fatalf("expected v2 got %q", doc.Version)
	}
	if len(doc.ProvenanceSources) != 2 {
		t.Fatalf("expected 2 provenance sources, got %d", len(doc.ProvenanceSources))
	}
	if doc.Events[0].ProvenanceID == doc.Events[1].ProvenanceID {
		t.Fatalf("expected distinct provenance IDs for distinct producers")
	}
}

func TestWriteEventsJSON_DedupMany(t *testing.T) {
	f, err := os.CreateTemp("", "events-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	pdesc := &tracer.ProvenanceDescriptor{BackendKind: "trace_open", OriginType: "direct"}
	events := make([]tracer.Event, 100)
	for i := range events {
		events[i] = tracer.Event{Syscall: "openat", Mode: "syscall", Path: "/a"}
		events[i].Provenance = pdesc
	}

	if err := writeEventsJSON(os.Stdout, f.Name(), events, nil, makeRunMeta()); err != nil {
		t.Fatalf("writeEventsJSON: %v", err)
	}

	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	doc, err := evidence.Decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.ProvenanceSources) != 1 {
		t.Fatalf("expected 1 provenance source, got %d", len(doc.ProvenanceSources))
	}
	pid := doc.Events[0].ProvenanceID
	for i := range doc.Events {
		if doc.Events[i].ProvenanceID != pid {
			t.Fatalf("event %d has different provenance id", i)
		}
	}
}

func TestWriteEventsJSON_MixedBackendsOrder(t *testing.T) {
	f, err := os.CreateTemp("", "events-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// build mixed-order events
	events := []tracer.Event{
		{Syscall: "openat", Path: "/a", Provenance: &tracer.ProvenanceDescriptor{BackendKind: "trace_open", OriginType: "direct"}},
		{Syscall: "connect", Mode: "egress", Provenance: &tracer.ProvenanceDescriptor{BackendKind: "trace_tcp", OriginType: "direct"}},
		{Syscall: "capability", Mode: "capability", Provenance: &tracer.ProvenanceDescriptor{BackendKind: "trace_capabilities", OriginType: "direct"}},
		{Syscall: "openat", Mode: "syscall", Provenance: &tracer.ProvenanceDescriptor{BackendKind: "advise_seccomp", OriginType: "advisory"}},
	}

	if err := writeEventsJSON(os.Stdout, f.Name(), events, nil, makeRunMeta()); err != nil {
		t.Fatalf("writeEventsJSON: %v", err)
	}

	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	doc, err := evidence.Decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// pIDs assigned in first-seen order
	// collect pIDs sequence
	pseq := []string{doc.Events[0].ProvenanceID, doc.Events[1].ProvenanceID, doc.Events[2].ProvenanceID, doc.Events[3].ProvenanceID}
	// ensure non-empty where provenance provided
	for i, ev := range events {
		if ev.Provenance != nil && pseq[i] == "" {
			t.Fatalf("expected provenance id for event %d", i)
		}
	}
	if len(doc.ProvenanceSources) != 4 {
		t.Fatalf("expected 4 provenance sources, got %d", len(doc.ProvenanceSources))
	}
}
