// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0
//
// Part of the landlock-genprof project.

// Package podlock définit les types Go correspondant au schéma CRD
// LandlockProfile du projet PodLock (github.com/flavio/podlock,
// écosystème Kubewarden), afin que landlock-genprof génère des profils
// directement utilisables sans transformation supplémentaire.
//
// TODO(M2): valider ces types face au schéma réel de PodLock au moment
// de l'implémentation (le format peut évoluer) — voir
// https://github.com/flavio/podlock
package podlock

// LandlockProfile miroir du CRD PodLock.
//
// Tags `json`, pas `yaml` : la sérialisation passe par sigs.k8s.io/yaml, qui
// convertit en JSON puis en YAML (comme le fait l'API server Kubernetes) —
// elle ignore silencieusement des tags `yaml:"..."` et retomberait sur le
// nom du champ Go (ex. "APIVersion" au lieu de "apiVersion").
type LandlockProfile struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Metadata   Metadata            `json:"metadata"`
	Spec       LandlockProfileSpec `json:"spec"`
}

type Metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type LandlockProfileSpec struct {
	// ProfilesByContainer: nom du conteneur -> chemin binaire -> restrictions
	ProfilesByContainer map[string]map[string]BinaryProfile `json:"profilesByContainer"`
}

type BinaryProfile struct {
	ReadExec  []string `json:"readExec,omitempty"`
	ReadOnly  []string `json:"readOnly,omitempty"`
	ReadWrite []string `json:"readWrite,omitempty"`
}
