// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0
//
// Part of the landlock-genprof project.

// Command landlock-genprof observe un pod Kubernetes en fonctionnement
// normal, puis génère un profil Landlock minimal au format PodLock.
//
// Usage prévu (M1) :
//
//	landlock-genprof trace --pod <nom> --namespace <ns> --duration 60s --out profile.yaml
//
// Ce fichier est un squelette. La logique réelle vit dans internal/.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "trace":
		// TODO(M1): brancher internal/k8s pour localiser le pod cible,
		// internal/tracer pour démarrer la capture, internal/policy
		// pour synthétiser le profil en sortie.
		fmt.Println("landlock-genprof trace: not yet implemented")
	case "version":
		fmt.Println("landlock-genprof (dev)")
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `landlock-genprof — génère un profil Landlock par observation

Commandes:
  trace     Démarre un training run sur un pod cible et génère un profil
  version   Affiche la version

Voir README.md pour le détail des options.`)
}
