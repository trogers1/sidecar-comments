#!/usr/bin/env bash
# Real race-detector smoke test; the fixture is not production CLI proof.
set -euo pipefail
work=$(mktemp -d "${TMPDIR:-/tmp}/sidecar-race-launcher.XXXXXX")
trap 'rm -rf "$work"' EXIT
go build -race -o "$work/sidecar" ./scripts/testdata/race-subject
cp scripts/race-subject.sh "$work/subject"
chmod 700 "$work/subject"
for expected in 66 77; do
  set +e
  GORACE="exitcode=$expected halt_on_error=1" "$work/subject" > "$work/stdout" 2> "$work/stderr"
  status=$?
  set -e
  [[ $status == "$expected" ]] || { cat "$work/stderr" >&2; echo "race exit $status, expected $expected" >&2; exit 1; }
  grep -Fq 'WARNING: DATA RACE' "$work/stderr"
done
# Corpus children have exact environments. Also test ordinary race shutdown,
# without halt_on_error: reports must survive with the exit sleep disabled.
set +e
env -i "$work/subject" > "$work/stdout" 2> "$work/stderr"
status=$?
set -e
[[ $status == 66 ]] || { cat "$work/stderr" >&2; echo "empty-environment race exit $status, expected 66" >&2; exit 1; }
grep -Fq 'WARNING: DATA RACE' "$work/stderr"
printf '%s\n' 'race launcher preserves race diagnostics and failure exit codes'
