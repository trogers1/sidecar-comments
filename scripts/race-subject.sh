#!/bin/sh
# Copied beside the race-built sidecar by make test-race. Apply this only to
# short-lived CLI subjects, including corpus children with exact environments.
set -eu
GORACE="${GORACE:+$GORACE }atexit_sleep_ms=0"
export GORACE
exec "${0%/*}/sidecar" "$@"
