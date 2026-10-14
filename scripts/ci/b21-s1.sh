#!/bin/bash
# b21-s1.sh (temporary, B21 diagnosis): the fixed point's stage s1 (the
# branch's xo building selfhost/xo with the arm64 backend, default emit
# jobs) with each XO_B21_C value given (bits: 2 big blocks, 4 chunks, 8
# the lock back in C).
set -uo pipefail
cd "$(dirname "$0")/../.."
root=$PWD
for v in "$@"; do
  d=$(mktemp -d)
  cd "$d"
  t0=$(date +%s)
  XO_B21_C=$v XO_BUILD_DIR="$d/b" "$XO_CLI" build --backend=arm64 --std="$root/std" -o "$d/xo" "$root/selfhost/xo" > "$d/log" 2>&1
  rc=$?
  cd "$root"
  echo "== XO_B21_C=$v rc=$rc secs=$(( $(date +%s) - t0 ))"
  tail -5 "$d/log"
  rm -rf "$d"
done
