package main

import (
	"testing"
)

func TestParseEventsOut(t *testing.T) {
	cmd := newTraceCmd()

	cases := []struct {
		name string
		args []string
	}{
		{"A", []string{"--events-out"}},
		{"B", []string{"--events-out=/tmp/test-events.json"}},
		{"C", []string{"--events-out", "/tmp/test-events.json"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Parse into the flag set directly
			if err := cmd.Flags().Parse(c.args); err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			v, err := cmd.Flags().GetString("events-out")
			if err != nil {
				t.Fatalf("GetString failed: %v", err)
			}
			t.Logf("case %s parsed events-out=%q remaining args=%q", c.name, v, cmd.Flags().Args())
		})
	}
}
