// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/policy"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// traceOptions rassemble les flags de `trace`, passés tels quels au reste
// du pipeline (voir runTrace).
type traceOptions struct {
	podName   string
	namespace string
	container string
	binary    string
	duration  time.Duration
	out       string
}

func newTraceCmd() *cobra.Command {
	var opts traceOptions

	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Démarre un training run sur un pod cible et génère un profil Landlock",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrace(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.podName, "pod", "p", "", "Nom du pod cible (requis)")
	flags.StringVarP(&opts.namespace, "namespace", "n", "default", "Namespace Kubernetes")
	flags.StringVarP(&opts.container, "container", "c", "", "Conteneur cible (déduit si le pod n'en a qu'un)")
	flags.StringVar(&opts.binary, "binary", "", "Chemin du binaire principal observé, ex: /usr/sbin/nginx (requis)")
	flags.DurationVarP(&opts.duration, "duration", "d", 60*time.Second, "Durée du training run")
	flags.StringVarP(&opts.out, "out", "o", "profile.yaml", "Fichier de sortie")

	for _, name := range []string{"pod", "binary"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err) // erreur de programmation (flag inexistant), pas une erreur utilisateur
		}
	}

	return cmd
}

// runTrace exécute le pipeline complet : résolution du pod, training run,
// synthèse de policy, export YAML. Voir docs/architecture.md §2 pour le
// diagramme de séquence correspondant.
func runTrace(ctx context.Context, stdout io.Writer, opts traceOptions) error {
	client, err := newKubeClient()
	if err != nil {
		return fmt.Errorf("connexion au cluster: %w", err)
	}

	target, err := k8s.Resolve(ctx, client, opts.namespace, opts.podName, opts.container)
	if err != nil {
		return fmt.Errorf("résolution du pod cible: %w", err)
	}

	events, err := tracer.Trace(tracer.Options{
		PodName:   target.PodName,
		Namespace: target.Namespace,
		Container: target.Container,
		Duration:  opts.duration,
	})
	if err != nil {
		return fmt.Errorf("training run: %w", err)
	}

	rules, err := policy.Synthesize(events)
	if err != nil {
		return fmt.Errorf("synthèse de policy: %w", err)
	}

	profile := policy.ToProfile(policy.ProfileMeta{
		Name:      target.PodName,
		Namespace: target.Namespace,
		Container: target.Container,
		Binary:    opts.binary,
	}, rules)

	yamlBytes, err := policy.ToYAML(profile)
	if err != nil {
		return fmt.Errorf("sérialisation YAML: %w", err)
	}

	if err := os.WriteFile(opts.out, yamlBytes, 0o644); err != nil {
		return fmt.Errorf("écriture de %s: %w", opts.out, err)
	}

	fmt.Fprintf(stdout, "Profil généré : %s\n", opts.out)
	return nil
}

// newKubeClient tente d'abord la config in-cluster (le tracer tournera
// dans le cluster à terme), puis retombe sur le kubeconfig local — utile
// pour lancer la CLI depuis son poste pendant le développement.
func newKubeClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("aucune configuration cluster trouvée (ni in-cluster, ni %s): %w", kubeconfig, err)
		}
	}
	return kubernetes.NewForConfig(config)
}
