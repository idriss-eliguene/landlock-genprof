// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package policy agrège des événements de tracing (internal/tracer) en un
// profil Landlock minimal, au format compatible avec le CRD LandlockProfile
// de PodLock (voir pkg/podlock).
package policy

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// Confidence indique le niveau de certitude d'une règle générée, en
// fonction du nombre de training runs où elle a été observée.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"    // vu une seule fois
	ConfidenceMedium Confidence = "medium" // vu sur plusieurs runs, incohérent
	ConfidenceHigh   Confidence = "high"   // vu systématiquement
)

// Rule est une règle candidate avant export au format PodLock.
type Rule struct {
	Path       string
	Access     []string // ex: "readExec", "readOnly", "readWrite"
	Confidence Confidence
	SeenCount  int
}

// maxAggregationDepth plafonne la profondeur de répertoire retenue pour une
// règle. Au-delà, un sous-répertoire est fusionné dans son ancêtre à cette
// profondeur — ex: /usr/share/nginx/html et /usr/share/nginx/css
// deviennent tous deux la règle /usr/share/nginx (voir README §8).
const maxAggregationDepth = 3

// dirAccess accumule les modes observés pour un répertoire donné, avant
// synthèse en catégories d'accès PodLock (readExec/readOnly/readWrite).
type dirAccess struct {
	seenCount int
	read      bool
	write     bool
	exec      bool
}

// Synthesize agrège une liste d'événements (issus d'un training run) en un
// ensemble de règles minimales, une par répertoire — pas par fichier, pour
// éviter le sur-fitting sur des chemins trop spécifiques.
//
// Seuls les événements porteurs d'un chemin fichier (openat/execve) sont
// pris en compte : le format de sortie PodLock (pkg/podlock.BinaryProfile)
// ne représente pas encore les droits réseau, donc les événements
// connect/bind (sans Path) sont ignorés ici.
//
// Heuristique de confiance (v1, un seul run) : basée sur le nombre
// d'événements agrégés dans le répertoire. Le calcul multi-runs évoqué
// dans docs/threat-model.md §2 ("vu à chaque run" vs "vu 1 fois sur 5
// runs") suppose de faire persister les résultats entre plusieurs appels
// de Synthesize, ce qui n'est pas encore câblé (voir roadmap M5).
func Synthesize(events []tracer.Event) ([]Rule, error) {
	byDir := make(map[string]*dirAccess)

	for _, ev := range events {
		if ev.Path == "" {
			continue
		}
		dir := aggregationDir(ev.Path)

		acc, ok := byDir[dir]
		if !ok {
			acc = &dirAccess{}
			byDir[dir] = acc
		}
		acc.seenCount++

		switch ev.Mode {
		case "read":
			acc.read = true
		case "write":
			acc.write = true
		case "read_write":
			acc.read = true
			acc.write = true
		case "exec":
			acc.exec = true
		}
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	rules := make([]Rule, 0, len(dirs))
	for _, dir := range dirs {
		acc := byDir[dir]

		var access []string
		if acc.exec {
			access = append(access, "readExec")
		}
		switch {
		case acc.write:
			access = append(access, "readWrite")
		case acc.read:
			access = append(access, "readOnly")
		}

		rules = append(rules, Rule{
			Path:       dir,
			Access:     access,
			Confidence: confidenceFor(acc.seenCount),
			SeenCount:  acc.seenCount,
		})
	}

	return rules, nil
}

// aggregationDir renvoie le répertoire parent du fichier, tronqué à
// maxAggregationDepth segments depuis la racine.
func aggregationDir(path string) string {
	dir := filepath.Dir(path)
	segments := strings.Split(strings.Trim(dir, "/"), "/")
	if len(segments) > maxAggregationDepth {
		segments = segments[:maxAggregationDepth]
	}
	return "/" + strings.Join(segments, "/")
}

func confidenceFor(seenCount int) Confidence {
	switch {
	case seenCount >= 3:
		return ConfidenceHigh
	case seenCount == 2:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}
