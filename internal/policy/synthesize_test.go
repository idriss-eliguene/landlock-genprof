// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import (
	"reflect"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func TestSynthesize_AggregatesByDirectory(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/usr/share/nginx/index.html", Mode: "read"},
		{Syscall: "openat", Path: "/usr/share/nginx/css/style.css", Mode: "read"},
		{Syscall: "openat", Path: "/tmp/nginx.pid", Mode: "write"},
	}

	rules, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	// Pas de règle par fichier individuel : les deux fichiers sous
	// /usr/share/nginx (dont un dans un sous-répertoire css/) doivent
	// fusionner en une seule règle.
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2 (pas une règle par fichier): %+v", len(rules), rules)
	}

	byPath := make(map[string]Rule, len(rules))
	for _, r := range rules {
		byPath[r.Path] = r
	}

	nginx, ok := byPath["/usr/share/nginx"]
	if !ok {
		t.Fatalf("expected a rule for /usr/share/nginx, got: %+v", rules)
	}
	if !reflect.DeepEqual(nginx.Access, []string{"readOnly"}) {
		t.Errorf("/usr/share/nginx Access = %v, want [readOnly]", nginx.Access)
	}
	if nginx.SeenCount != 2 {
		t.Errorf("/usr/share/nginx SeenCount = %d, want 2 (index.html + css/style.css)", nginx.SeenCount)
	}

	tmp, ok := byPath["/tmp"]
	if !ok {
		t.Fatalf("expected a rule for /tmp, got: %+v", rules)
	}
	if !reflect.DeepEqual(tmp.Access, []string{"readWrite"}) {
		t.Errorf("/tmp Access = %v, want [readWrite]", tmp.Access)
	}
}

func TestSynthesize_MockNginxEvents(t *testing.T) {
	rules, err := Synthesize(mockNginxEvents())
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	want := map[string][]string{
		"/usr/sbin":        {"readExec"},
		"/etc/nginx":       {"readOnly"},
		"/usr/share/nginx": {"readOnly"}, // html/index.html tronqué à profondeur 3
		"/var/log/nginx":   {"readWrite"},
		"/tmp":             {"readWrite"},
	}

	// L'event connect (sans Path) ne doit produire aucune règle : le
	// format PodLock actuel (pkg/podlock.BinaryProfile) ne représente pas
	// les droits réseau.
	if len(rules) != len(want) {
		t.Fatalf("len(rules) = %d, want %d: %+v", len(rules), len(want), rules)
	}

	byPath := make(map[string]Rule, len(rules))
	for _, r := range rules {
		byPath[r.Path] = r
	}

	for path, wantAccess := range want {
		got, ok := byPath[path]
		if !ok {
			t.Errorf("missing rule for %s", path)
			continue
		}
		if !reflect.DeepEqual(got.Access, wantAccess) {
			t.Errorf("%s Access = %v, want %v", path, got.Access, wantAccess)
		}
	}
}

func TestSynthesize_EmptyInput(t *testing.T) {
	rules, err := Synthesize(nil)
	if err != nil {
		t.Fatalf("Synthesize(nil) error = %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("len(rules) = %d, want 0", len(rules))
	}
}
