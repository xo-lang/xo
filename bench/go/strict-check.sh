#!/bin/sh
# Strict Go checks for the A/B benchmark. Usage: strict-check.sh <module dir>
# Runs: go vet, staticcheck, errcheck, nilaway, exhaustive, go test -race.
# Exits non zero if any check reports a problem. Analyzers come from
# $XO_TOOLS_BIN when set, else the repo local .tools/bin.
set -u
dir=${1:?usage: strict-check.sh <module dir>}
tools=${XO_TOOLS_BIN:-"$(cd "$(dirname "$0")/../.." && pwd)/.tools/bin"}
cd "$dir" || exit 2
status=0
run() {
  name=$1; shift
  out=$("$@" 2>&1); rc=$?
  if [ $rc -ne 0 ]; then echo "== $name: FAIL"; echo "$out"; status=1; else echo "== $name: ok"; fi
}
run vet go vet ./...
run staticcheck "$tools/staticcheck" ./...
run errcheck "$tools/errcheck" ./...
run nilaway "$tools/nilaway" -pretty-print=false ./...
run exhaustive "$tools/exhaustive" ./...
run test go test -race -count=1 ./...
exit $status
