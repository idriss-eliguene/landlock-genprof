// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package podlock

import (
	"testing"
)

// wantFullNginxYAML pins ToYAML's complete byte output for
// mockNginxFilesystemProfile() — Phase 0 of the planned internal/landlock
// kernel extraction (see the architecture review that led to it). The
// existing tests in export_test.go each check one property of the output
// (categories, round-tripping, confidence comments) in isolation; this one
// is the single byte-for-byte signal that the whole exporter's output
// hasn't moved at all once ToProfile is refactored to consume a
// landlock.Candidate instead of profile.FilesystemProfile directly (Phase
// 2). Regenerate deliberately (never hand-edit) if a real, intended output
// change ships.
const wantFullNginxYAML = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  profilesByContainer:
    nginx:
      /usr/sbin/nginx:
        readExec:
          - /usr/sbin # confidence: high
        readOnly:
          - /etc/nginx # confidence: high
          - /usr/share/nginx # confidence: high
        readWrite:
          - /tmp # confidence: medium
          - /var/log/nginx # confidence: low
`

func TestToYAML_FullNginxRun_Golden(t *testing.T) {
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

	if string(out) != wantFullNginxYAML {
		t.Errorf("ToYAML() =\n%s\nwant:\n%s", out, wantFullNginxYAML)
	}
}
