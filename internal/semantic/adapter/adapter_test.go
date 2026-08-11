package adapter

import (
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
)

func mustTime(t *testing.T, y int, m time.Month, d, hh, mm, ss int) time.Time {
	t.Helper()
	return time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
}

func Test_DedupDifferentTimestamps(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 0, 10)}
	e1 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "read", Path: "/x", Mode: "read"}
	e2 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "read", Path: "/x", Mode: "read"}
	e3 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "read", Path: "/x", Mode: "read"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{e1, e2, e3})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
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
	e1 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: ts, Syscall: "read", Path: "/x", Mode: "read"}
	e2 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: ts, Syscall: "read", Path: "/x", Mode: "read"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{e1, e2})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
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
	events := make([]observation.Observation, 100)
	for i := 0; i < 100; i++ {
		events[i] = observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, i%10), Syscall: "read", Path: "/x", Mode: "read"}
	}
	res, err := BuildGraphFromObservations(meta, events)
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
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
	e1 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "read", Path: "/x", Mode: "read"}
	e2 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "write", Path: "/y", Mode: "write"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{e1, e2})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
	}
	if len(res.AssertionIDs) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(res.AssertionIDs))
	}
}

func Test_ZeroEvents(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 3, 0)}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{})
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
	e := observation.Observation{Kind: observation.KindFilesystem, Timestamp: time.Time{}, Syscall: "read", Path: "/x", Mode: "read"}
	if _, err := BuildGraphFromObservations(meta, []observation.Observation{e}); err == nil {
		t.Fatalf("expected error for zero event timestamp")
	}
}

func Test_InvalidRecordTime(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: time.Time{}}
	e := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 5, 0), Syscall: "read", Path: "/x", Mode: "read"}
	if _, err := BuildGraphFromObservations(meta, []observation.Observation{e}); err == nil {
		t.Fatalf("expected error for zero RecordTime")
	}
}

func Test_UnsupportedNetworkEvents(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 6, 0)}
	// connect/egress
	e1 := observation.Observation{Kind: observation.KindNetwork, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "connect", Port: 80, Mode: "egress"}
	if _, err := BuildGraphFromObservations(meta, []observation.Observation{e1}); err == nil {
		t.Fatalf("expected unsupported event error for connect")
	}
	// bind/ingress
	e2 := observation.Observation{Kind: observation.KindNetwork, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "bind", Port: 8080, Mode: "ingress"}
	if _, err := BuildGraphFromObservations(meta, []observation.Observation{e2}); err == nil {
		t.Fatalf("expected unsupported event error for bind")
	}
	// capability
	e3 := observation.Observation{Kind: observation.KindCapability, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 3), Syscall: "capability", Mode: "capability"}
	if _, err := BuildGraphFromObservations(meta, []observation.Observation{e3}); err == nil {
		t.Fatalf("expected unsupported event error for capability")
	}
}

func Test_MixedInputAtomicity(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 7, 0)}
	good := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "openat", Path: "/x", Mode: "read"}
	bad := observation.Observation{Kind: observation.KindNetwork, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "connect", Port: 80, Mode: "egress"}
	if _, err := BuildGraphFromObservations(meta, []observation.Observation{good, bad}); err == nil {
		t.Fatalf("expected error for mixed input containing unsupported event")
	}
}

func Test_IntervalValidation(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 8, 0)}
	// unsorted timestamps t3,t1,t2 => interval [t1,t3]
	t1 := mustTime(t, 2026, time.January, 1, 0, 0, 1)
	t2 := mustTime(t, 2026, time.January, 1, 0, 0, 2)
	t3 := mustTime(t, 2026, time.January, 1, 0, 0, 3)
	res, err := BuildGraphFromObservations(meta, []observation.Observation{{Kind: observation.KindFilesystem, Timestamp: t3, Syscall: "openat", Path: "/a", Mode: "read"}, {Kind: observation.KindFilesystem, Timestamp: t1, Syscall: "openat", Path: "/b", Mode: "read"}, {Kind: observation.KindFilesystem, Timestamp: t2, Syscall: "openat", Path: "/c", Mode: "read"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	acts := res.Graph.GetActs()
	if len(acts) != 1 {
		t.Fatalf("expected 1 act, got %d", len(acts))
	}
	if !acts[0].Interval().Start.Equal(t1) || !acts[0].Interval().End.Equal(t3) {
		t.Fatalf("unexpected interval: %v", acts[0].Interval())
	}

	// identical timestamps -> [t,t]
	ts := mustTime(t, 2026, time.January, 1, 0, 0, 5)
	res2, err := BuildGraphFromObservations(meta, []observation.Observation{{Kind: observation.KindFilesystem, Timestamp: ts, Syscall: "openat", Path: "/x", Mode: "read"}, {Kind: observation.KindFilesystem, Timestamp: ts, Syscall: "openat", Path: "/x", Mode: "read"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	acts2 := res2.Graph.GetActs()
	if len(acts2) != 1 {
		t.Fatalf("expected 1 act, got %d", len(acts2))
	}
	if !acts2[0].Interval().Start.Equal(ts) || !acts2[0].Interval().End.Equal(ts) {
		t.Fatalf("unexpected identical timestamp interval: %v", acts2[0].Interval())
	}

	// explicit Start > explicit End -> error
	start := mustTime(t, 2026, time.January, 1, 0, 0, 10)
	end := mustTime(t, 2026, time.January, 1, 0, 0, 5)
	meta2 := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), Start: &start, End: &end, RecordTime: mustTime(t, 2026, time.January, 1, 0, 8, 0)}
	if _, err := BuildGraphFromObservations(meta2, []observation.Observation{{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 6), Syscall: "openat", Path: "/x", Mode: "read"}}); err == nil {
		t.Fatalf("expected interval validation error for Start > End")
	}

	// explicit Start only where Start > derived max -> error
	start2 := mustTime(t, 2026, time.January, 1, 0, 0, 20)
	meta3 := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), Start: &start2, RecordTime: mustTime(t, 2026, time.January, 1, 0, 8, 0)}
	if _, err := BuildGraphFromObservations(meta3, []observation.Observation{{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "openat", Path: "/x", Mode: "read"}}); err == nil {
		t.Fatalf("expected interval validation error for explicit Start > derived End")
	}

	// explicit End only where derived min > End -> error
	end2 := mustTime(t, 2026, time.January, 1, 0, 0, 0)
	meta4 := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), End: &end2, RecordTime: mustTime(t, 2026, time.January, 1, 0, 8, 0)}
	if _, err := BuildGraphFromObservations(meta4, []observation.Observation{{Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "openat", Path: "/x", Mode: "read"}}); err == nil {
		t.Fatalf("expected interval validation error for derived Start > explicit End")
	}
}

func Test_1vs100FrequencyBelief(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 9, 0)}
	one := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "openat", Path: "/file", Mode: "read"}
	res1, err := BuildGraphFromObservations(meta, []observation.Observation{one})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	res100, err := BuildGraphFromObservations(meta, func() []observation.Observation {
		evs := make([]observation.Observation, 100)
		for i := 0; i < 100; i++ {
			evs[i] = one
		}
		return evs
	}())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res1.AssertionIDs) != 1 || len(res100.AssertionIDs) != 1 {
		t.Fatalf("expected 1 AE in both cases; got %d and %d", len(res1.AssertionIDs), len(res100.AssertionIDs))
	}
	id1 := res1.AssertionIDs[0]
	id100 := res100.AssertionIDs[0]
	L1, _ := res1.Graph.BeliefState()
	L100, _ := res100.Graph.BeliefState()
	if L1.Lookup(id1) != L100.Lookup(id100) {
		t.Fatalf("BeliefState differs for 1 vs 100 occurrences: %v vs %v", L1.Lookup(id1), L100.Lookup(id100))
	}
}

func Test_InputOrderInvariant(t *testing.T) {
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-1"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 10, 0)}
	a := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "openat", Path: "/a", Mode: "read"}
	b := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "openat", Path: "/b", Mode: "read"}
	res1, err := BuildGraphFromObservations(meta, []observation.Observation{a, b, a})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	res2, err := BuildGraphFromObservations(meta, []observation.Observation{a, a, b})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// compare proposition sets by number
	if len(res1.AssertionIDs) != len(res2.AssertionIDs) {
		t.Fatalf("expected same AE count, got %d and %d", len(res1.AssertionIDs), len(res2.AssertionIDs))
	}
	L1, _ := res1.Graph.BeliefState()
	L2, _ := res2.Graph.BeliefState()
	for i := range res1.AssertionIDs {
		if L1.Lookup(res1.AssertionIDs[i]) != L2.Lookup(res2.AssertionIDs[i]) {
			t.Fatalf("BeliefStatus mismatch at index %d: %v vs %v", i, L1.Lookup(res1.AssertionIDs[i]), L2.Lookup(res2.AssertionIDs[i]))
		}
	}
}
