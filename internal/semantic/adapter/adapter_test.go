package adapter

import (
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func mustTime(t *testing.T, y int, m time.Month, d, hh, mm, ss int) time.Time {
	t.Helper()
	return time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
}

func Test_DedupDifferentTimestamps(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 0, 10)}
	e1 := tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "read", Path: "/x", Mode: "read"}
	e2 := tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "read", Path: "/x", Mode: "read"}
	e3 := tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "read", Path: "/x", Mode: "read"}
	res, err := BuildGraphFromEvents(meta, []tracer.Event{e1, e2, e3})
	if err != nil {
		t.Fatalf("BuildGraphFromEvents error: %v", err)
	}
	if len(res.AssertionIDs) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(res.AssertionIDs))
	}
	id := res.AssertionIDs[0]
	if gs, ok := res.EvidenceGroups[id]; !ok || len(gs) != 3 {
		t.Fatalf("expected evidence group of length 3, got %v (ok=%v)", gs, ok)
	}
	// check Graph BeliefState
	L, err := res.Graph.BeliefState()
	if err != nil {
		t.Fatalf("BeliefState error: %v", err)
	}
	if L.Lookup(id) != 1 { // internal _beliefIn == 1
		t.Fatalf("expected assertion in, got status %v", L.Lookup(id))
	}
}

func Test_SameTimestampDedup(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 0, 10)}
	ts := mustTime(t, 2026, time.January, 1, 0, 0, 5)
	e1 := tracer.Event{Timestamp: ts, Syscall: "read", Path: "/x", Mode: "read"}
	e2 := tracer.Event{Timestamp: ts, Syscall: "read", Path: "/x", Mode: "read"}
	res, err := BuildGraphFromEvents(meta, []tracer.Event{e1, e2})
	if err != nil {
		t.Fatalf("BuildGraphFromEvents error: %v", err)
	}
	if len(res.AssertionIDs) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(res.AssertionIDs))
	}
	id := res.AssertionIDs[0]
	if gs, ok := res.EvidenceGroups[id]; !ok || len(gs) != 2 {
		t.Fatalf("expected evidence group of length 2, got %v (ok=%v)", gs, ok)
	}
}

func Test_100Occurrences(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 1, 0)}
	events := make([]tracer.Event, 100)
	for i := 0; i < 100; i++ {
		events[i] = tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, i%10), Syscall: "read", Path: "/x", Mode: "read"}
	}
	res, err := BuildGraphFromEvents(meta, events)
	if err != nil {
		t.Fatalf("BuildGraphFromEvents error: %v", err)
	}
	if len(res.AssertionIDs) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(res.AssertionIDs))
	}
	id := res.AssertionIDs[0]
	if gs := res.EvidenceGroups[id]; len(gs) != 100 {
		t.Fatalf("expected evidence group length 100, got %d", len(gs))
	}
}

func Test_TwoDifferentObservations(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 2, 0)}
	e1 := tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "read", Path: "/x", Mode: "read"}
	e2 := tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "write", Path: "/y", Mode: "write"}
	res, err := BuildGraphFromEvents(meta, []tracer.Event{e1, e2})
	if err != nil {
		t.Fatalf("BuildGraphFromEvents error: %v", err)
	}
	if len(res.AssertionIDs) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(res.AssertionIDs))
	}
}

func Test_ZeroEvents(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 3, 0)}
	res, err := BuildGraphFromEvents(meta, []tracer.Event{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Graph == nil {
		t.Fatalf("expected non-nil Graph even for empty result")
	}
	if len(res.AssertionIDs) != 0 {
		t.Fatalf("expected 0 assertions, got %d", len(res.AssertionIDs))
	}
}

func Test_InvalidEventTimestamp(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 4, 0)}
	e := tracer.Event{Timestamp: time.Time{}, Syscall: "read", Path: "/x", Mode: "read"}
	if _, err := BuildGraphFromEvents(meta, []tracer.Event{e}); err == nil {
		t.Fatalf("expected error for zero event timestamp")
	}
}

func Test_InvalidRecordTime(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: time.Time{}}
	e := tracer.Event{Timestamp: mustTime(t, 2026, time.January, 1, 0, 5, 0), Syscall: "read", Path: "/x", Mode: "read"}
	if _, err := BuildGraphFromEvents(meta, []tracer.Event{e}); err == nil {
		t.Fatalf("expected error for zero RecordTime")
	}
}
