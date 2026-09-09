// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

type healthzOptions struct {
	path string
	port int
}

func newHealthzCmd() *cobra.Command {
	opts := healthzOptions{path: workbenchLivenessPath, port: 8080}
	cmd := &cobra.Command{
		Use:    "healthz",
		Short:  "Checks an internal Operations Center lifecycle endpoint",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHealthz(opts)
		},
	}
	cmd.Flags().StringVar(&opts.path, "path", opts.path, "Internal lifecycle endpoint")
	cmd.Flags().IntVar(&opts.port, "port", opts.port, "Operations Center HTTP port")
	return cmd
}

func runHealthz(opts healthzOptions) error {
	if !isWorkbenchLifecyclePath(opts.path) {
		return fmt.Errorf("unsupported lifecycle path %q", opts.path)
	}
	if opts.port < 1 || opts.port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(opts.port) + opts.path)
	if err != nil {
		return fmt.Errorf("lifecycle endpoint unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lifecycle endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
