#!/bin/bash
# arm64-identity.sh: the arm64 backend's output with two xo binaries, for
# every golden and the compiler itself, darwin and linux, with the
# checkout's std and no backend cache: a change meant for another target
# (the amd64 backend, decision 0254) must leave arm64's assembly byte
# identical.
#
#   scripts/arm64-identity.sh [--own-std] BASE_XO NEW_XO [BACKEND]
#
# BACKEND is arm64 (default) or amd64 (the targets linux/amd64 and
# darwin/amd64 then). Prints "compared N, differing M" and each differing
# directory; exits 1 when any differs.
#
# --own-std: for a change to std and the compiler together (such as a new
# llrt call only the new compiler folds; gotchas/identity-std-and-compiler.md):
# each xo builds with its own checkout's std (XO/../../std), copied to one
# path so the directories the output names are the same, and the asm is
# compared with source positions (FILE.xo:LINE:COL, which an added std
# line moves) and label-only lines (a folded if's empty join block, which
# assembles to nothing) left out.
set -u
own=0
if [ "${1:-}" = --own-std ]; then
  own=1
  shift
fi
base=$1
new=$2
be=${3:-arm64}
root=$(cd "$(dirname "$0")/.." && pwd)
dirs="$(find "$root/internal/llvm/testdata/golden" -mindepth 1 -maxdepth 1 -type d | sort) $root/compiler/xo"
if [ $own = 0 ]; then
  diffs=0
  n=0
  for d in $dirs; do
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
  exit
fi
tmp=$(mktemp -d)
trap 'rm -rf "${tmp:?}"' EXIT
for side in base new; do
  xo=$([ $side = base ] && echo "$base" || echo "$new")
  rm -rf "${tmp:?}/std"
  cp -R "$(dirname "$(dirname "$xo")")/std" "$tmp/std"
  for d in $dirs; do
    for os in darwin linux; do
      (cd "$d" && XO_BACKEND_CACHE=off XO_STD="$tmp/std" "$xo" asm --backend="$be" --targets="$os/$be" . 2>&1 |
        sed -E -e 's/\.xo:[0-9]+:[0-9]+/.xo:L/g' -e 's/\.xo:[0-9]+/.xo:L/g' | grep -Ev '^\.?L[A-Za-z0-9_]+:$' | shasum) >> "$tmp/$side"
      echo "$os ${d#$root/}" >> "$tmp/names"
    done
  done
done
paste -d' ' "$tmp/base" "$tmp/new" | awk '$1 != $3 {print NR}' > "$tmp/bad"
while read -r i; do
  echo "DIFF $(sed -n "${i}p" "$tmp/names")"
done < "$tmp/bad"
n=$(wc -l < "$tmp/base" | tr -d ' ')
diffs=$(wc -l < "$tmp/bad" | tr -d ' ')
echo "compared $n, differing $diffs"
[ "$diffs" = 0 ]
