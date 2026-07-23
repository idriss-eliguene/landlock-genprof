// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package podlock

import (
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

// mockNginxFilesystemProfile mirrors the shape internal/policy.Synthesize
// would produce for a typical nginx training run (see
// internal/policy/testdata_test.go's mockNginxEvents). Built directly as
// IR fixtures, not derived by calling Synthesize: this package tests the
// IR -> PodLock conversion in isolation, the same way
// internal/policy/synthesize_test.go tests events -> IR in isolation —
// neither package needs the other's internals to test its own half of
// the pipeline. Accesses are listed in path-sorted order, matching what
// Synthesize's own deterministic ordering (sort.Strings) would produce.
func mockNginxFilesystemProfile() profile.FilesystemProfile {
	return profile.FilesystemProfile{
		Accesses: []profile.FileAccess{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, Confidence: profile.ConfidenceHigh},
			{Path: "/tmp", Permissions: []profile.FilePermission{profile.PermissionWrite}, Confidence: profile.ConfidenceMedium},
			{Path: "/usr/sbin", Permissions: []profile.FilePermission{profile.PermissionExecute}, Confidence: profile.ConfidenceHigh},
			{Path: "/usr/share/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, Confidence: profile.ConfidenceHigh},
			{Path: "/var/log/nginx", Permissions: []profile.FilePermission{profile.PermissionWrite}, Confidence: profile.ConfidenceLow},
		},
	}
}

func TestToProfile_MockNginxFilesystemProfile(t *testing.T) {
	meta := ProfileMeta{
		Name:      "nginx-demo",
		Namespace: "default",
		Container: "nginx",
		Binary:    "/usr/sbin/nginx",
	}
	result := ToProfile(meta, mockNginxFilesystemProfile())

	if result.APIVersion != "podlock.kubewarden.io/v1alpha1" {
		t.Errorf("APIVersion = %q, want podlock.kubewarden.io/v1alpha1", result.APIVersion)
	}
	if result.Kind != "LandlockProfile" {
		t.Errorf("Kind = %q, want LandlockProfile", result.Kind)
	}
	if result.Metadata.Name != "nginx-demo" || result.Metadata.Namespace != "default" {
		t.Errorf("Metadata = %+v, want {nginx-demo default}", result.Metadata)
	}

	bp, ok := result.Spec.ProfilesByContainer["nginx"]["/usr/sbin/nginx"]
	if !ok {
		t.Fatalf("no Profile for nginx//usr/sbin/nginx, got: %+v", result.Spec.ProfilesByContainer)
	}

	if !reflect.DeepEqual(bp.ReadExec, []string{"/usr/sbin"}) {
		t.Errorf("ReadExec = %v, want [/usr/sbin]", bp.ReadExec)
	}
	if !reflect.DeepEqual(bp.ReadOnly, []string{"/etc/nginx", "/usr/share/nginx"}) {
		t.Errorf("ReadOnly = %v, want [/etc/nginx /usr/share/nginx]", bp.ReadOnly)
	}
	if !reflect.DeepEqual(bp.ReadWrite, []string{"/tmp", "/var/log/nginx"}) {
		t.Errorf("ReadWrite = %v, want [/tmp /var/log/nginx]", bp.ReadWrite)
	}
}

// TestToProfile_CollapsesExecAndWriteIntoReadWriteExec checks the
// category found by validating against PodLock's real schema
// (github.com/flavio/podlock): a directory that's both executed and
// written to is the single distinct category "readWriteExec", not
// "readExec" and "readWrite" reported as two separate entries.
func TestToProfile_CollapsesExecAndWriteIntoReadWriteExec(t *testing.T) {
	fs := profile.FilesystemProfile{
		Accesses: []profile.FileAccess{
			{Path: "/opt/app", Permissions: []profile.FilePermission{profile.PermissionWrite, profile.PermissionExecute}},
		},
	}

	result := ToProfile(ProfileMeta{
		Name:      "app-demo",
		Namespace: "default",
		Container: "app",
		Binary:    "/opt/app/run",
	}, fs)

	bp := result.Spec.ProfilesByContainer["app"]["/opt/app/run"]
	if !reflect.DeepEqual(bp.ReadWriteExec, []string{"/opt/app"}) {
		t.Errorf("ReadWriteExec = %v, want [/opt/app]", bp.ReadWriteExec)
	}
	if len(bp.ReadExec) != 0 || len(bp.ReadWrite) != 0 || len(bp.ReadOnly) != 0 {
		t.Errorf("expected only ReadWriteExec populated, got %+v", bp)
	}
}

func TestToYAML_RoundTrips(t *testing.T) {
	fs := mockNginxFilesystemProfile()
	result := ToProfile(ProfileMeta{
		Name:      "nginx-demo",
		Namespace: "default",
		Container: "nginx",
		Binary:    "/usr/sbin/nginx",
	}, fs)

	out, err := ToYAML(result, fs)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	// Keys must be camelCase (apiVersion, readOnly, ...), not the Go field
	// name (APIVersion, ReadOnly, ...) — that's the guarantee that
	// sigs.k8s.io/yaml reads `json` tags, not `yaml` tags.
	text := string(out)
	for _, want := range []string{"apiVersion:", "profilesByContainer:", "readOnly:", "readWrite:", "readExec:"} {
		if !strings.Contains(text, want) {
			t.Errorf("YAML output missing expected key %q:\n%s", want, text)
		}
	}

	var roundTripped podlock.LandlockProfile
	if err := yaml.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&roundTripped, result) {
		t.Errorf("round-tripped profile = %+v, want %+v", roundTripped, *result)
	}
}

// TestToYAML_AnnotatesConfidence checks the actual point of this
// function's second parameter: each path gets a trailing
// `# confidence: ...` comment matching what fs recorded for it —
// invisible to struct unmarshaling (TestToYAML_RoundTrips already
// covers that the parsed structure is unaffected), but present in the
// raw text a human reviewer (docs/threat-model.md) actually reads.
func TestToYAML_AnnotatesConfidence(t *testing.T) {
	fs := mockNginxFilesystemProfile()
	result := ToProfile(ProfileMeta{
		Name: "nginx-demo", Namespace: "default", Container: "nginx", Binary: "/usr/sbin/nginx",
	}, fs)

	out, err := ToYAML(result, fs)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	for _, line := range []string{
		"- /etc/nginx # confidence: high",
		"- /tmp # confidence: medium",
		"- /var/log/nginx # confidence: low",
		"- /usr/sbin # confidence: high",
	} {
		if !strings.Contains(string(out), line) {
			t.Errorf("YAML output missing expected line %q:\n%s", line, out)
		}
	}
}

// TestToYAML_NoCommentForUnsetConfidence checks that a FileAccess built
// without setting Confidence (the zero value "") — e.g. IR built
// directly rather than through internal/policy.Synthesize — doesn't
// produce a nonsensical bare "# confidence: " comment.
func TestToYAML_NoCommentForUnsetConfidence(t *testing.T) {
	fs := profile.FilesystemProfile{
		Accesses: []profile.FileAccess{
			{Path: "/etc/app", Permissions: []profile.FilePermission{profile.PermissionRead}},
		},
	}
	result := ToProfile(ProfileMeta{
		Name: "app-demo", Namespace: "default", Container: "app", Binary: "/opt/app/run",
	}, fs)

	out, err := ToYAML(result, fs)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	if strings.Contains(string(out), "confidence:") {
		t.Errorf("expected no confidence comment for an unset Confidence, got:\n%s", out)
	}
}
