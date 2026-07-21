// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package tracer capture les événements syscall d'un pod pendant un
// training run, en s'appuyant sur les gadgets Inspektor Gadget existants
// (trace_open, trace_tcpconnect, trace_bind, trace_exec) plutôt que
// d'écrire des programmes eBPF depuis zéro.
//
// Décision d'architecture (voir docs/roadmap.md) : on consomme la sortie
// de gadgets déjà maintenus et testés par la communauté CNCF, ce qui
// réduit fortement le risque d'échec du projet tout en gardant la
// différenciation sur le mapping syscalls → droits Landlock et sur la
// synthèse de policy, qui restent inédits.
package tracer

import "time"

// Event représente un accès observé pendant le training run, avant
// traduction en droits Landlock.
type Event struct {
	Timestamp time.Time
	Syscall   string // ex: "openat", "connect", "bind", "execve"
	Path      string // chemin fichier concerné, si applicable
	Port      int    // port réseau concerné, si applicable
	Mode      string // "read", "write", "read_write", "exec"
}

// Options configure un training run.
type Options struct {
	PodName   string
	Namespace string
	Container string
	Duration  time.Duration
}

// Trace démarre la capture et retourne les événements observés.
//
// TODO(M1, Étudiant A): implémenter en s'appuyant sur les gadgets
// Inspektor Gadget (trace_open / trace_tcpconnect / trace_bind).
// Ne pas écrire de programme eBPF from scratch pour la v1.
func Trace(opts Options) ([]Event, error) {
	panic("not implemented")
}
