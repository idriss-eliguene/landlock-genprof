// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"net/http"
	"sync"
)

const (
	workbenchHealthPath    = "/healthz"
	workbenchStartupPath   = "/healthz/startup"
	workbenchLivenessPath  = "/healthz/live"
	workbenchReadinessPath = "/healthz/ready"
)

// workbenchLifecycle is deliberately process-local. It describes only the
// HTTP process lifecycle; it is not a projection or Kubernetes health model.
type workbenchLifecycle struct {
	mu       sync.RWMutex
	started  bool
	draining bool
}

func (l *workbenchLifecycle) markStarted() {
	l.mu.Lock()
	l.started = true
	l.mu.Unlock()
}

func (l *workbenchLifecycle) beginDrain() {
	l.mu.Lock()
	l.draining = true
	l.mu.Unlock()
}

func (l *workbenchLifecycle) isStarted() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.started
}

func (l *workbenchLifecycle) isReady() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.started && !l.draining
}

func isWorkbenchLifecyclePath(path string) bool {
	switch path {
	case workbenchHealthPath, workbenchStartupPath, workbenchLivenessPath, workbenchReadinessPath:
		return true
	default:
		return false
	}
}

func (l *workbenchLifecycle) serveHTTP(w http.ResponseWriter, r *http.Request) {
	ready := false
	switch r.URL.Path {
	case workbenchHealthPath, workbenchStartupPath, workbenchLivenessPath:
		ready = l.isStarted()
	case workbenchReadinessPath:
		ready = l.isReady()
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready\n"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
