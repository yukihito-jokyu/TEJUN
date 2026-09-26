#!/bin/sh
set -eu

child_pid=""

terminate_tree() {
  for descendant in $(pgrep -P "$1" 2>/dev/null || true); do
    terminate_tree "$descendant"
  done
  kill -TERM "$1" 2>/dev/null || true
}

cleanup() {
  trap - EXIT INT TERM
  if [ -n "$child_pid" ]; then
    terminate_tree "$child_pid"
    wait "$child_pid" 2>/dev/null || true
  fi
}

trap cleanup EXIT INT TERM
npm run dev -- --host 127.0.0.1 --port 9245 --strictPort &
child_pid=$!
wait "$child_pid"
