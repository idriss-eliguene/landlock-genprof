// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
	observationexecutor "github.com/idriss-eliguene/landlock-genprof/internal/observation/executor"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func newExecutorCmd() *cobra.Command {
	var namespaces []string
	var binary string
	cmd := &cobra.Command{
		Use:   "executor",
		Short: "Runs the bounded Observation executor worker",
		Long:  "Consumes durable Observation requests with the dedicated executor Kubernetes identity; it has no governance operation authority." + kubectlPrefixNote,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(namespaces) == 0 {
				return fmt.Errorf("at least one --namespace is required")
			}
			config, err := k8s.RestConfig()
			if err != nil {
				return err
			}
			core, err := kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}
			dynamicClient, err := dynamic.NewForConfig(config)
			if err != nil {
				return err
			}
			clean := make([]string, 0, len(namespaces))
			for _, namespace := range namespaces {
				namespace = strings.TrimSpace(namespace)
				if namespace != "" {
					clean = append(clean, namespace)
				}
			}
			logger, metrics, obsConfig, err := observability.NewFromEnv(cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("configuring observability: %w", err)
			}
			metricsServer, err := startExecutorMetricsServer(obsConfig, metrics, logger)
			if err != nil {
				return err
			}
			defer func() {
				if metricsServer != nil {
					_ = metricsServer.Shutdown(context.Background())
				}
			}()
			return observationexecutor.Run(cmd.Context(), observationexecutor.Config{Core: core, Dynamic: dynamicClient, Rest: config, Namespaces: clean, Binary: binary, Logger: logger, Metrics: metrics})
		},
	}
	cmd.Flags().StringSliceVar(&namespaces, "namespace", nil, "authorized target namespace (repeatable via comma-separated values)")
	cmd.Flags().StringVar(&binary, "binary", "", "optional process basename filter")
	return cmd
}

func startExecutorMetricsServer(config observability.Config, metrics *observability.Metrics, logger *observability.Logger) (*http.Server, error) {
	if !config.Metrics {
		return nil, nil
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", config.MetricsPort))
	if err != nil {
		return nil, fmt.Errorf("binding executor metrics listener: %w", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(metrics.ServeHTTP), ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	logger.Info("metrics_listener_ready", map[string]interface{}{"component": "observation_executor", "port": config.MetricsPort})
	return server, nil
}
