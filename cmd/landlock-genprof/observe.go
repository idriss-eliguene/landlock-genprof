// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"fmt"
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
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "observe",
		Short: "Runs a bounded filesystem Observation for a running Pod",
		Long:  "Runs the v0.7 filesystem/trace_open Observation vertical only." + kubectlPrefixNote,
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
			spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: target.Instance.Slot}, []string{runtime.FilesystemSourceName}, duration, "cli")
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
			runner := &runtime.Runner{Store: store, Client: client, Cluster: cluster, Source: runtime.GadgetFilesystemSource{}, Monitor: runtime.PollingTargetMonitor{Client: client, Cluster: cluster, Target: spec.Target}}
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
	cmd.Flags().DurationVar(&duration, "duration", time.Minute, "bounded Observation duration")
	_ = cmd.MarkFlagRequired("pod")
	_ = cmd.MarkFlagRequired("container")
	return cmd
}
