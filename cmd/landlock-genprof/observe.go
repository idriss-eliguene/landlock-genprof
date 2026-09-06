// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/runtime"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			cluster, err := k8s.ResolveClusterIdentity(ctx, client)
			if err != nil {
				return err
			}
			podObject, err := client.CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
			if err != nil {
				return err
			}
			target, err := k8s.ResolveObservationTarget(ctx, client, cluster, podObject, container)
			if err != nil {
				return err
			}
			if len(sourceNames) == 0 {
				sourceNames = []string{runtime.FilesystemSourceName}
			}
			sources := make([]runtime.FilesystemSource, 0, len(sourceNames))
			for _, name := range sourceNames {
				source, sourceErr := observationSource(strings.TrimSpace(name))
				if sourceErr != nil {
					return sourceErr
				}
				sources = append(sources, source)
			}
			spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: target.Instance.Slot}, sourceNames, duration, "cli")
			if err != nil {
				return err
			}
			id, err := domain.NewObservationID()
			if err != nil {
				return err
			}
			observation, err := domain.NewObservation(id, spec)
			if err != nil {
				return err
			}
			store, err := obskube.NewStore(dynamicClient)
			if err != nil {
				return err
			}
			if _, err := store.CreateObservation(ctx, namespace, observation); err != nil {
				return err
			}
			executorID, err := obskube.NewExecutorID()
			if err != nil {
				return err
			}
			runner := &runtime.Runner{Store: store, Client: client, Cluster: cluster, Sources: sources, Monitor: runtime.PollingTargetMonitor{Client: client, Cluster: cluster, Target: spec.Target}}
			runner.Binary = binary
			if err := runner.Run(ctx, namespace, string(id), executorID); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), id)
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
	switch name {
	case runtime.FilesystemSourceName:
		return runtime.GadgetFilesystemSource{}, nil
	case "exec":
		return runtime.GadgetExecSource{}, nil
	case "networkConnect":
		return runtime.GadgetNetworkConnectSource{}, nil
	case "networkBind":
		return runtime.GadgetNetworkBindSource{}, nil
	case "capabilities":
		return runtime.GadgetCapabilitiesSource{}, nil
	default:
		return nil, fmt.Errorf("unsupported observation source %q", name)
	}
}
