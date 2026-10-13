#!/bin/bash
# run.sh: litmus tests of the runtime's atomics (decision 0200).
#
# Usage: bench/litmus/run.sh XO1 [BACKEND...]
#   XO1      the self hosted driver to build with (an xo1 binary)
#   BACKEND  arm64 and/or llvm (default: arm64 on an arm64 host, and llvm)
#   LITMUS_N iterations per test and run (default 100000)
#   LITMUS_RUNS runs per backend (default 20)
#
# The program (main.xo.in) reaches the atomics, which only std's
# implementation modules see (decision 0074), through std/litmus, a test
# only module this script adds to a copy of std/ in a temporary
# directory (litmus.xo.in and native_litmus.xo.in); std/ itself has no
# such module. Prints each run's lines, then per test the sum of the weak
# outcomes over the runs; exits 1 when a run prints FAIL (a forbidden
# outcome or a lost update) or fails.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
xo1=${1:?usage: run.sh XO1 [BACKEND...]}
shift
backends=("$@")
if [ ${#backends[@]} -eq 0 ]; then
  case "$(uname -m)" in
    arm64|aarch64) backends=(arm64 llvm) ;;
    *) backends=(llvm) ;;
  esac
fi
n=${LITMUS_N:-100000}
runs=${LITMUS_RUNS:-20}
tmp=$(mktemp -d "${TMPDIR:-/tmp}/litmus.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
cp -R "$root/std" "$tmp/std"
cp "$here/litmus.xo.in" "$tmp/std/litmus.xo"
cp "$here/native_litmus.xo.in" "$tmp/std/native/litmus.xo"
ln -s "$root/third_party" "$tmp/third_party"
mkdir "$tmp/prog"
cp "$here/main.xo.in" "$tmp/prog/main.xo"
fail=0
for b in "${backends[@]}"; do
  flags=(--backend="$b")
  bin="$tmp/litmus-$b"
  (cd "$tmp/prog" && "$xo1" build --std="$tmp/std" --wd="$tmp/prog" "${flags[@]}" -o "$bin" "$tmp/prog")
  echo "=== $b: $runs runs of n=$n ($(uname -sm))"
  out="$tmp/out-$b"
  : > "$out"
  for r in $(seq 1 "$runs"); do
    if ! "$bin" "$n" >> "$out" 2>&1; then
      fail=1
    fi
  done
  cat "$out"
  echo "=== $b: sums over $runs runs"
  awk '/weak=/ { split($2, a, "="); w[$1] += a[2]; c[$1] += 1 }
       / total=/ { split($2, a, "="); split($3, b, "="); if (a[2] != b[2]) bad[$1] += 1; c[$1] += 1 }
       END { for (k in w) printf "%s weak=%d runs=%d\n", k, w[k], c[k];
             for (k in c) if (!(k in w)) printf "%s lost_update_runs=%d runs=%d\n", k, bad[k] + 0, c[k] }' "$out" | sort
  if grep -q FAIL "$out"; then
    fail=1
  fi
done
exit $fail
