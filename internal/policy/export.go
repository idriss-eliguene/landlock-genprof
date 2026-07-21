// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import (
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

// ProfileMeta identifie le pod/conteneur/binaire auquel s'applique un
// ensemble de règles synthétisées — Rule n'a aucune notion de conteneur ou
// de binaire (voir docs/architecture.md), c'est l'appelant (typiquement le
// CLI, à partir de internal/k8s.TargetPod et des options de trace) qui
// fournit ce contexte au moment de l'export.
type ProfileMeta struct {
	Name      string // nom du LandlockProfile généré (généralement le nom du pod)
	Namespace string
	Container string
	Binary    string // chemin du binaire principal du conteneur (point d'entrée observé)
}

// ToProfile assemble des règles synthétisées (Synthesize) en un
// LandlockProfile prêt à être sérialisé, au format consommé par
// l'opérateur PodLock. Une seule entrée profilesByContainer est produite
// (meta.Container -> meta.Binary), puisqu'un training run cible un seul
// conteneur à la fois.
func ToProfile(meta ProfileMeta, rules []Rule) *podlock.LandlockProfile {
	var bp podlock.BinaryProfile
	for _, r := range rules {
		for _, access := range r.Access {
			switch access {
			case "readExec":
				bp.ReadExec = append(bp.ReadExec, r.Path)
			case "readOnly":
				bp.ReadOnly = append(bp.ReadOnly, r.Path)
			case "readWrite":
				bp.ReadWrite = append(bp.ReadWrite, r.Path)
			}
		}
	}

	return &podlock.LandlockProfile{
		APIVersion: "podlock.kubewarden.io/v1alpha1",
		Kind:       "LandlockProfile",
		Metadata: podlock.Metadata{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		},
		Spec: podlock.LandlockProfileSpec{
			ProfilesByContainer: map[string]map[string]podlock.BinaryProfile{
				meta.Container: {
					meta.Binary: bp,
				},
			},
		},
	}
}

// ToYAML sérialise un LandlockProfile au format YAML, tel qu'écrit dans
// profile.yaml par le CLI (voir cmd/landlock-genprof).
func ToYAML(profile *podlock.LandlockProfile) ([]byte, error) {
	return yaml.Marshal(profile)
}
