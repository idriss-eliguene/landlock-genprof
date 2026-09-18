#!/usr/bin/env bash

# Shared lifecycle helpers for executors launched through `limactl shell`.
# Killing the host-side shell is not proof that the guest process exited.

remote_executor_state() {
  local vm=$1
  local executable=$2
  local status

  # shellcheck disable=SC2016
  if limactl shell "$vm" -- sh -c \
    'ps -eo pid=,args= | awk -v needle="$1" '\''index($0, needle) && $0 !~ /awk/ { found=1 } END { exit(found ? 0 : 42) }'\''' \
    sh "$executable" >/dev/null 2>&1; then
    return 0
  else
    status=$?
    if [ "$status" -eq 42 ]; then return 1; fi
    return 2
  fi
}

wait_remote_executor_exit() {
  local vm=$1
  local executable=$2
  local state

  for _ in $(seq 1 50); do
    if remote_executor_state "$vm" "$executable"; then
      sleep 0.1
    else
      state=$?
      [ "$state" -eq 1 ] && return 0
      sleep 0.1
    fi
  done
  return 1
}

stop_host_executor_shell() {
  local pid=$1

  if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then return 0; fi
  kill -TERM "$pid" 2>/dev/null || true
  for _ in $(seq 1 50); do
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 0.1
  done
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

terminate_remote_executor() {
  local vm=$1
  local executable=$2
  local host_pid=${3:-}
  local state
  local cleanup_failed=0

  if [ -n "$executable" ]; then
    limactl shell "$vm" -- pkill -TERM -f "$executable" >/dev/null 2>&1 || true
    if ! wait_remote_executor_exit "$vm" "$executable"; then
      limactl shell "$vm" -- pkill -KILL -f "$executable" >/dev/null 2>&1 || true
      wait_remote_executor_exit "$vm" "$executable" || cleanup_failed=1
    fi
  fi

  stop_host_executor_shell "$host_pid"

  if [ -n "$executable" ]; then
    if remote_executor_state "$vm" "$executable"; then
      cleanup_failed=1
    else
      state=$?
      [ "$state" -eq 1 ] || cleanup_failed=1
    fi
  fi
  return "$cleanup_failed"
}
