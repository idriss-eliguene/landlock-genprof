// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWorkbenchLifecycleEndpointsSeparateStartupLivenessReadiness(t *testing.T) {
	srv, _ := newTestWorkbenchServer(t, "default")
	for _, path := range []string{workbenchStartupPath, workbenchLivenessPath, workbenchReadinessPath} {
		t.Run("before-start "+path, func(t *testing.T) {
			res := lifecycleRequest(srv, path)
			if res.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", res.Code)
			}
		})
	}

	srv.lifecycle.markStarted()
	for _, path := range []string{workbenchStartupPath, workbenchLivenessPath, workbenchReadinessPath} {
		t.Run("started "+path, func(t *testing.T) {
			res := lifecycleRequest(srv, path)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Code)
			}
			if res.Body.String() != "ok\n" {
				t.Fatalf("body = %q, want ok", res.Body.String())
			}
		})
	}

	srv.lifecycle.beginDrain()
	if got := lifecycleRequest(srv, workbenchReadinessPath).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("draining readiness = %d, want 503", got)
	}
	if got := lifecycleRequest(srv, workbenchLivenessPath).Code; got != http.StatusOK {
		t.Fatalf("draining liveness = %d, want 200", got)
	}
}

func TestWorkbenchLifecycleEndpointsDoNotBypassProtectedAPI(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	srv.lifecycle.markStarted()
	srv.requestContext = func(*http.Request) (workbenchRequestContext, error) {
		return workbenchRequestContext{}, errors.New("unauthenticated")
	}

	health := lifecycleRequest(srv, workbenchReadinessPath)
	if health.Code != http.StatusOK {
		t.Fatalf("lifecycle status = %d, want 200", health.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v08/operations-context", nil)
	request.Host = host
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("protected API status = %d, want 401", response.Code)
	}
}

func TestWorkbenchLifecyclePathsAreExact(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	srv.lifecycle.markStarted()
	request := httptest.NewRequest(http.MethodGet, "/healthz/../api/v08/operations-context", nil)
	request.Host = host
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, request)
	if response.Code == http.StatusOK {
		t.Fatal("path traversal request unexpectedly reached a lifecycle success")
	}
}

func TestWorkbenchShutdownAllowsAcceptedRequestToDrain(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})
	listener := newPipeListener()
	server := &http.Server{Handler: handler}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	response := make(chan error, 1)
	go func() {
		client, serverConn := net.Pipe()
		listener.accept(serverConn)
		_, err := fmt.Fprintf(client, "GET / HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n")
		if err == nil {
			_, err = io.ReadAll(client)
		}
		_ = client.Close()
		response <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not enter the handler")
	}

	lifecycle := &workbenchLifecycle{}
	lifecycle.markStarted()
	lifecycle.beginDrain()
	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), workbenchShutdownTimeout)
		defer cancel()
		shutdownDone <- server.Shutdown(ctx)
	}()
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown completed before accepted request was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if err := <-response; err != nil {
		t.Fatalf("draining request failed: %v", err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if lifecycle.isReady() {
		t.Fatal("lifecycle remained ready after drain began")
	}
	select {
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			t.Fatalf("Serve() error = %v, want ErrServerClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}

type pipeListener struct {
	conns  chan net.Conn
	done   chan struct{}
	closed chan struct{}
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn, 1), done: make(chan struct{}), closed: make(chan struct{})}
}

func (l *pipeListener) accept(conn net.Conn) { l.conns <- conn }

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	select {
	case <-l.closed:
		return nil
	default:
		close(l.closed)
		close(l.done)
		return nil
	}
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr("pipe") }

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

func lifecycleRequest(srv *workbenchServer, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Host = "not-the-service-host:8080"
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, request)
	return response
}
