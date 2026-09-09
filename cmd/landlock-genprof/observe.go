// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/runtime"
	"github.com/idriss-eliguene/landlock-genprof/internal/observationapp"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func newObserveCmd() *cobra.Command {
	var namespace, pod, container, binary string
	var sourceNames []string
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "observe",
		Short: "Runs a bounded runtime Observation for a running Pod",
		Long:  "Runs the supported v0.7 runtime Observation sources." + kubectlPrefixNote,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			config, err := k8s.RestConfig()
			if err != nil {
				return err
			}
			client, err := kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}
			dynamicClient, err := dynamic.NewForConfig(config)
			if err != nil {
				return err
			}
			prepared, err := observationapp.Prepare(ctx, observationapp.Clients{Core: client, Dynamic: dynamicClient}, observationapp.PrepareRequest{
				Namespace: namespace, Pod: pod, Container: container, Sources: sourceNames, Duration: duration, Requester: "cli",
			})
			if err != nil {
				return err
			}
			executorID, err := obskube.NewExecutorID()
			if err != nil {
				return err
			}
			runner := &runtime.Runner{Store: prepared.Store, Client: client, Cluster: prepared.Cluster, Sources: prepared.Sources, Monitor: runtime.PollingTargetMonitor{Client: client, Cluster: prepared.Cluster, Target: prepared.Spec.Target}}
			runner.Binary = binary
			if err := runner.Run(ctx, namespace, string(prepared.ID), executorID); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), prepared.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "default", "Kubernetes namespace")
	cmd.Flags().StringVar(&pod, "pod", "", "running target Pod")
	cmd.Flags().StringVar(&container, "container", "", "target container")
	cmd.Flags().StringVar(&binary, "binary", "", "optional process basename filter")
	cmd.Flags().StringSliceVar(&sourceNames, "sources", []string{runtime.FilesystemSourceName}, "runtime sources (filesystem, exec, networkConnect, networkBind, capabilities)")
	cmd.Flags().DurationVar(&duration, "duration", time.Minute, "bounded Observation duration")
	_ = cmd.MarkFlagRequired("pod")
	_ = cmd.MarkFlagRequired("container")
	return cmd
}

func observationSource(name string) (runtime.FilesystemSource, error) {
	return observationapp.Source(name)
}
