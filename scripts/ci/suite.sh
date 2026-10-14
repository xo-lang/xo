#!/bin/bash
# scripts/ci/suite.sh SUITE [PACKAGE...]: the test suites of the on demand
# CI workflow (.github/workflows/ci.yml, decision 0225). Runs the same on
# any machine with Go and clang, from anywhere in the checkout.
#
#   auto         short, then the touched Go packages' full tests, the
#                internal/selfhost merge tier when selfhost/ or std/
#                changed, and the fixed point when the change touches what
#                the self build compiles (the CLAUDE.md test policy)
#   short        go vet and go test -short over every package but runs/
#   vet          go vet only
#   merge        the full tests of PACKAGE... (default: the touched ones)
#   fixed-point  scripts/bootstrap-xo.sh with the checkout's xo (s2 == s3)
#   full         every package with XO_SELFHOST_TIER=full
#
# "Touched" means changed between $XO_CI_BASE (default origin/main) and
# HEAD, from their merge base.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
suite=${1:?usage: scripts/ci/suite.sh SUITE [PACKAGE...]}
shift
base=${XO_CI_BASE:-origin/main}

step() { echo; echo "::group::$*"; }
end() { echo "::endgroup::"; }

all_pkgs() { go list -e ./... | grep -v /runs/; }
changed() { git diff --name-only "$base...HEAD"; }

# Go packages with a changed file (deleted directories are skipped).
touched_pkgs() {
  changed | grep '\.go$' | xargs -r -n1 dirname | sort -u |
    while read -r d; do [ -d "$d" ] && ls "$d"/*.go >/dev/null 2>&1 && echo "./$d"; done
}

# The CLAUDE.md fixed point rule: what the self build compiles or runs.
needs_fixed_point() {
  changed | grep -E '^(selfhost/|std/native/|std/llrt/|internal/xotool/(xotool|chain|seed|lock_[^/]*)\.go$|scripts/xobuild/|scripts/seed-chain\.txt$)' |
    grep -vE '^selfhost/(macho|wasm)/|_test\.xo$' | grep -q .
}

run_short() {
  step "go vet"; go vet $(all_pkgs); end
  step "go test -short"; go test -short $(all_pkgs); end
}

run_merge() {
  [ $# -gt 0 ] || { echo "merge: no packages"; return 0; }
  step "go test (full) $*"; go test -timeout 60m "$@"; end
}

run_fixed_point() {
  step "xo for the fixed point (scripts/xobuild)"; xo=$(go run ./scripts/xobuild); echo "$xo"; end
  step "fixed point (scripts/bootstrap-xo.sh)"
  rc=0; scripts/bootstrap-xo.sh "$xo" .tools/bootstrap || rc=$?
  cat .tools/bootstrap/times.txt 2>/dev/null || true
  [ $rc -eq 0 ] || tail -40 .tools/bootstrap/log.txt
  end; return $rc
}

case "$suite" in
  short) run_short ;;
  vet) step "go vet"; go vet $(all_pkgs); end ;;
  merge) if [ $# -gt 0 ]; then run_merge "$@"; else run_merge $(touched_pkgs); fi ;;
  fixed-point) run_fixed_point ;;
  full) step "full tier"; XO_SELFHOST_TIER=full go test -timeout 120m $(all_pkgs); end ;;
  auto)
    echo "changed since $base:"; changed | sed 's/^/  /'
    run_short
    pkgs=$(touched_pkgs)
    if changed | grep -qE '^(selfhost|std)/'; then pkgs="$pkgs ./internal/selfhost"; fi
    run_merge $(echo $pkgs | tr ' ' '\n' | sort -u)
    if needs_fixed_point; then run_fixed_point; else echo "fixed point: not needed for these paths"; fi
    ;;
  *) echo "unknown suite $suite" >&2; exit 2 ;;
esac
