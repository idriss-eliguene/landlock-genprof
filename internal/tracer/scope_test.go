package tracer

import "testing"

func TestCommAdmissionModes(t *testing.T) {
	tests := []struct {
		name     string
		scope    Scope
		expected string
		actual   string
		want     bool
	}{
		{name: "legacy matching", scope: BinaryCommFiltered, expected: "server", actual: "server", want: true},
		{name: "legacy mismatching", scope: BinaryCommFiltered, expected: "server", actual: "worker", want: false},
		{name: "container matching", scope: ContainerScoped, expected: "server", actual: "server", want: true},
		{name: "container mismatching", scope: ContainerScoped, expected: "server", actual: "worker", want: true},
		{name: "container empty comm", scope: ContainerScoped, expected: "server", actual: "", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commAdmits(tt.scope, tt.expected, tt.actual); got != tt.want {
				t.Fatalf("commAdmits(%v, %q, %q) = %v, want %v", tt.scope, tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

func TestExpectedCommForMode(t *testing.T) {
	if got := expectedCommFor(Options{Scope: ContainerScoped, Binary: "/app/server"}); got != "" {
		t.Fatalf("container scope expected no comm predicate, got %q", got)
	}
	if got := expectedCommFor(Options{Scope: BinaryCommFiltered, Binary: "/app/server"}); got != "server" {
		t.Fatalf("legacy scope expected basename predicate, got %q", got)
	}
}
