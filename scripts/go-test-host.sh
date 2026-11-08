#!/usr/bin/env bash
# go-test-host.sh PKG RUNREGEX [GO TEST FLAGS...]: `go test -count=1 -v
# -run RUNREGEX` of one package with $XO_CLI, for an offload host's script
# mode (~/xo-lead/agents/amd64-run.sh REV NAME script
# scripts/go-test-host.sh ./cmd/xo '^TestName$', which sets XO_CLI to the
# xo it built at REV; OFFLOAD_ENV=offload-win.env runs it on Windows in
# Git Bash). Its exit status is go test's.
set -u -o pipefail
pkg=${1:?usage: go-test-host.sh PKG RUNREGEX [GO TEST FLAGS...]}
run=${2:?usage: go-test-host.sh PKG RUNREGEX [GO TEST FLAGS...]}
shift 2
: "${XO_CLI:?go-test-host.sh: set XO_CLI to a built xo}"
cd "$(dirname "$0")/.." || exit 1
echo "go-test-host: $(uname -s) $(uname -m), XO_CLI=$XO_CLI"
go test -count=1 -v -run "$run" "$@" "$pkg"
