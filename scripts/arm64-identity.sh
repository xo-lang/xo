#!/bin/bash
# arm64-identity.sh: the arm64 backend's output with two xo binaries, for
# every golden and the compiler itself, darwin and linux, with the
# checkout's std and no backend cache: a change meant for another target
# (the amd64 backend, decision 0254) must leave arm64's assembly byte
# identical.
#
#   scripts/arm64-identity.sh BASE_XO NEW_XO [BACKEND]
#
# BACKEND is arm64 (default) or amd64 (the targets linux/amd64 and
# darwin/amd64 then). Prints "compared N, differing M" and each differing
# directory; exits 1 when any differs.
set -u
base=$1
new=$2
be=${3:-arm64}
root=$(cd "$(dirname "$0")/.." && pwd)
diffs=0
n=0
for d in $(find "$root/internal/llvm/testdata/golden" -mindepth 1 -maxdepth 1 -type d | sort) "$root/compiler/xo"; do
  for os in darwin linux; do
    a=$(cd "$d" && XO_BACKEND_CACHE=off XO_STD="$root/std" "$base" asm --backend="$be" --targets="$os/$be" . 2>&1 | shasum)
    b=$(cd "$d" && XO_BACKEND_CACHE=off XO_STD="$root/std" "$new" asm --backend="$be" --targets="$os/$be" . 2>&1 | shasum)
    n=$((n + 1))
    if [ "$a" != "$b" ]; then
      diffs=$((diffs + 1))
      echo "DIFF $os ${d#$root/}"
    fi
  done
done
echo "compared $n, differing $diffs"
[ "$diffs" = 0 ]
