#!/bin/bash
# gate.sh: an agent's gate before a push, as named steps (CLAUDE.md
# "Tests"); run it under the heavy lock (scripts/lead/heavy.sh NAME
# scripts/gate.sh ...). It rebuilds WORKTREE's bin/xo, then runs each
# step with that xo and prints one line per step, then GATE-DONE.
#
#   scripts/gate.sh WORKTREE STEP...
#
# Steps:
#   short    go test -short over the repository (runs/ excluded)
#   arm64    the full tests of internal/arm64
#   llvm     the full tests of internal/llvm
#   cmd      the full tests of cmd/xo
#   gen      the full tests of internal/gen
#   alloff   internal/arm64's goldens with every optional pass off
#            (XO_PASSES, compiler/arm64 passes.xo)
#   fp       the self rebuild fixed point (scripts/bootstrap-xo.sh); its
#            output (about 3 GB) is removed after
#
# Logs go to $XO_GATE_LOGS (default $TMPDIR/xo-gate-WORKTREE_NAME), one
# file per step. A step's failure is reported and the next step runs;
# the exit status is 1 when any failed.
set -u
wt=$(cd "${1:?usage: gate.sh WORKTREE STEP...}" && pwd)
shift
logs=${XO_GATE_LOGS:-${TMPDIR:-/tmp}/xo-gate-$(basename "$wt")}
mkdir -p "$logs"
cd "$wt" || exit 1
t0=$(date +%s)
rm -f bin/xo
go run ./scripts/xobuild >"$logs/xobuild.txt" 2>&1 || { echo "XOBUILD-FAIL ($logs/xobuild.txt)"; exit 1; }
echo "xobuild $(($(date +%s) - t0)) s"
export XO_CLI=$wt/bin/xo XO_BACKEND_CACHE=off
failed=0
full() { # NAME PKG [go test args]: one package's full tests
  n=$1; p=$2; shift 2
  if go test -count=1 -timeout 120m "$p" "$@" >"$logs/$n.txt" 2>&1; then
    echo "$n: $(tail -1 "$logs/$n.txt")"
  else
    failed=1
    echo "$n: FAIL ($logs/$n.txt)"
    grep -E '^(    )?--- FAIL' "$logs/$n.txt" | head -10
  fi
}
for step in "$@"; do
  case $step in
  short)
    go test -short -count=1 $(go list ./... | grep -v /runs/) >"$logs/short.txt" 2>&1
    k=$(grep -c '^--- FAIL' "$logs/short.txt")
    [ "$k" -gt 0 ] && failed=1
    echo "short fails: $k" ;;
  arm64) full arm64 ./internal/arm64 ;;
  llvm) full llvm ./internal/llvm ;;
  cmd) full cmd ./cmd/xo ;;
  gen) full gen ./internal/gen ;;
  alloff)
    off=$(sed -n '/^pub fn passes()/,/^}/p' compiler/arm64/passes.xo | grep -o '"[a-z0-9_]*"' | tr -d '"' | sed 's/^/-/' | paste -sd, -)
    XO_PASSES=$off full alloff ./internal/arm64 -run '^TestGoldenRun$' ;;
  fp)
    tmp=$(mktemp -d "${TMPDIR:-/tmp}/xo-gate-fp.XXXXXX")
    cp bin/xo "$tmp/seed"
    if scripts/bootstrap-xo.sh "$tmp/seed" "$tmp/out" >"$logs/fp.txt" 2>&1; then
      echo "fp: $(tail -1 "$logs/fp.txt")"
    else
      failed=1
      echo "fp: FAIL ($logs/fp.txt)"
    fi
    rm -rf "$tmp" ;;
  *) echo "gate.sh: unknown step $step"; failed=1 ;;
  esac
done
echo GATE-DONE
exit $failed
