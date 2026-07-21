// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Command landlock-genprof observe un pod Kubernetes en fonctionnement
// normal, puis génère un profil Landlock minimal au format PodLock.
//
// Usage :
//
//	landlock-genprof trace --pod <nom> --namespace <ns> --binary <chemin> --duration 60s --out profile.yaml
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "landlock-genprof",
		Short: "Génère un profil Landlock par observation d'un pod Kubernetes",
		// SilenceErrors : main() affiche déjà l'erreur renvoyée par Execute(),
		// sans ça cobra l'afficherait une seconde fois (préfixée "Error: ").
		SilenceErrors: true,
		// SilenceUsage : le bloc Usage n'a de sens que pour une erreur de
		// flags/arguments, pas pour un échec runtime (cluster injoignable,
		// pod introuvable...) — cobra l'affiche par défaut dans les deux cas.
		SilenceUsage: true,
	}
	root.AddCommand(newTraceCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Affiche la version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "landlock-genprof (dev)")
			return nil
		},
	}
}
