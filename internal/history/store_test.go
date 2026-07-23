// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package history

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func TestRecordName(t *testing.T) {
	if got := RecordName("nginx", "/usr/sbin/nginx"); got != "nginx-nginx" {
		t.Errorf("RecordName() = %q, want nginx-nginx", got)
	}
}

func TestGet_NotFoundReturnsNilNil(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	record, err := Get(context.Background(), client, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if record != nil {
		t.Errorf("Get() = %+v, want nil (no record yet)", record)
	}
}

func TestSave_ThenGet_RoundTrips(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	record := &Record{
		Container:    "nginx",
		Binary:       "/usr/sbin/nginx",
		RunsRecorded: 2,
		FilesystemAccesses: []FileAccessRecord{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, SeenInRuns: 2},
			{Path: "/var/cache/nginx/proxy", Permissions: []profile.FilePermission{profile.PermissionWrite}, SeenInRuns: 1},
		},
		NetworkAccesses: []NetworkAccessRecord{
			{Port: 443, Direction: profile.DirectionEgress, SeenInRuns: 2},
		},
		SyscallAccesses: []SyscallAccessRecord{
			{Name: "openat", SeenInRuns: 2},
			{Name: "brk", SeenInRuns: 1},
		},
		CapabilityAccesses: []CapabilityAccessRecord{
			{Name: "CAP_NET_BIND_SERVICE", SeenInRuns: 2},
		},
	}

	if err := Save(context.Background(), client, "default", "nginx-nginx", record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(got, record) {
		t.Errorf("round-tripped record = %+v, want %+v", got, record)
	}
}

// TestSave_UpdatesExistingRecord checks the Create-vs-Update branch in
// Save: a second Save for the same name must overwrite, not fail on a
// missing/stale resourceVersion or create a duplicate.
func TestSave_UpdatesExistingRecord(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	first := &Record{Container: "nginx", Binary: "/usr/sbin/nginx", RunsRecorded: 1,
		FilesystemAccesses: []FileAccessRecord{{Path: "/etc/nginx", SeenInRuns: 1}}}
	if err := Save(context.Background(), client, "default", "nginx-nginx", first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	second := &Record{Container: "nginx", Binary: "/usr/sbin/nginx", RunsRecorded: 2,
		FilesystemAccesses: []FileAccessRecord{{Path: "/etc/nginx", SeenInRuns: 2}}}
	if err := Save(context.Background(), client, "default", "nginx-nginx", second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.RunsRecorded != 2 {
		t.Errorf("RunsRecorded = %d, want 2 (updated, not left at the first Save's value)", got.RunsRecorded)
	}
}
