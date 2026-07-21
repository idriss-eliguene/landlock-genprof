// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import "github.com/idriss-eliguene/landlock-genprof/internal/tracer"

// mockNginxEvents simule les événements d'un training run sur un pod nginx,
// pour développer et tester Synthesize sans dépendre du vrai tracer
// (voir HOW_TO_START.md §6).
func mockNginxEvents() []tracer.Event {
	return []tracer.Event{
		{Syscall: "openat", Path: "/usr/sbin/nginx", Mode: "exec"},
		{Syscall: "openat", Path: "/etc/nginx/nginx.conf", Mode: "read"},
		{Syscall: "openat", Path: "/usr/share/nginx/html/index.html", Mode: "read"},
		{Syscall: "openat", Path: "/var/log/nginx/access.log", Mode: "write"},
		{Syscall: "openat", Path: "/tmp/nginx.pid", Mode: "write"},
		{Syscall: "connect", Port: 80, Mode: "read"},
	}
}
