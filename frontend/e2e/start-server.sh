#!/bin/sh
set -eu

data_dir=$(mktemp -d "${TMPDIR:-/tmp}/tejun-e2e.XXXXXX")
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
  rm -rf -- "$data_dir"
}

trap cleanup EXIT INT TERM
TEJUN_DATA_DIR="$data_dir" task --dir .. dev &
child_pid=$!
wait "$child_pid"
