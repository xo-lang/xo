#!/bin/bash
# std-identity.sh: the arm64 assembly of every internal/llvm golden and
# the compiler itself, built by one xo with std at two revisions, compared
# without DWARF sections and source columns (scripts/asmnorm.py): a std
# rewrite meant to lower to the same code (decision 0260 step 3) must
# leave it equal. Byte identity is too strict for such a rewrite: moved
# columns change DWARF and fault locations
# (knowledgebase/gotchas/rewrite-moves-source-columns.md).
#
#   scripts/std-identity.sh XO REV_A REV_B
#
# Run it under the heavy lock (about 25 minutes on the Mac). Prints
# "compared N, failed F, differing M" and each differing directory;
# exits 1 when any differs or fails to build.
set -u
xo=${1:?usage: std-identity.sh XO REV_A REV_B}
ra=${2:?}
rb=${3:?}
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/xo-std-identity.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
for side in a b; do
  rev=$ra; [ $side = b ] && rev=$rb
  mkdir -p "$tmp/$side"
  git -C "$root" archive "$rev" std | tar -x -C "$tmp/$side" || exit 1
done
n=0; fails=0; diffs=0
for d in $(find "$root/internal/llvm/testdata/golden" -mindepth 1 -maxdepth 1 -type d | sort) "$root/compiler/xo"; do
  n=$((n + 1))
  h=
  for side in a b; do
    if (cd "$d" && XO_BACKEND_CACHE=off XO_STD="$tmp/$side/std" "$xo" asm --backend=arm64 --targets=darwin/arm64 . >"$tmp/o.s" 2>"$tmp/err"); then
      h="$h $(python3 "$root/scripts/asmnorm.py" "$tmp/o.s" | shasum | cut -c1-16)"
    else
      h="$h fail"
    fi
  done
  set -- $h
  if [ "$1" = fail ] || [ "$2" = fail ]; then
    fails=$((fails + 1)); echo "FAIL ${d#$root/} ($1 $2)"
  elif [ "$1" != "$2" ]; then
    diffs=$((diffs + 1)); echo "DIFF ${d#$root/}"
  fi
done
echo "compared $n, failed $fails, differing $diffs"
[ "$fails" = 0 ] && [ "$diffs" = 0 ]
