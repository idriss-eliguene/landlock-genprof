// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package profile

import "testing"

func TestFileAccess_HasPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []FilePermission
		check       FilePermission
		want        bool
	}{
		{
			name:        "single permission, matching",
			permissions: []FilePermission{PermissionRead},
			check:       PermissionRead,
			want:        true,
		},
		{
			name:        "single permission, not matching",
			permissions: []FilePermission{PermissionRead},
			check:       PermissionWrite,
			want:        false,
		},
		{
			name:        "multiple permissions, matching one of them",
			permissions: []FilePermission{PermissionWrite, PermissionExecute},
			check:       PermissionExecute,
			want:        true,
		},
		{
			name:        "empty permission set",
			permissions: nil,
			check:       PermissionExecute,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := FileAccess{Path: "/tmp", Permissions: tt.permissions}
			if got := fa.HasPermission(tt.check); got != tt.want {
				t.Errorf("HasPermission(%v) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}
