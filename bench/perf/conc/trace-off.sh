#!/bin/bash
# Off cost of the execution trace's hooks (decision 0240): each conc
# workload built on arm64 by one xo with main's std (base) and the
# trace's std (hooks), run interleaved REPS times at the default threads
# and XO_THREADS=1; median instructions retired (/usr/bin/time -l).
# usage: trace-off.sh SCRATCH XO BASE_STD NEW_STD bench/perf/conc (REPS=11)
set -eu
S=$1; XO=$2; BASE_STD=$3; NEW_STD=$4; CONC=$5; REPS=${REPS:-11}
out=$S/off/bin; mkdir -p "$out"
for w in spawn fanout pingpong mutex; do
  (cd "$CONC/xo/$w" && XO_STD=$BASE_STD XO_BIN_CACHE=off "$XO" build --backend=arm64 -o "$out/$w-base" . >/dev/null)
  (cd "$CONC/xo/$w" && XO_STD=$NEW_STD XO_BIN_CACHE=off "$XO" build --backend=arm64 -o "$out/$w-new" . >/dev/null)
done
ins() { /usr/bin/time -l "$@" 2>&1 >/dev/null | awk '/instructions retired/ {print $1}'; }
for th in "" 1; do
  for w in spawn fanout pingpong mutex; do
    b=(); n=()
    for i in $(seq 1 $REPS); do
      if [ -n "$th" ]; then export XO_THREADS=$th; else unset XO_THREADS; fi
      b+=($(ins "$out/$w-base")); n+=($(ins "$out/$w-new"))
    done
    python3 - "$w" "${th:-default}" "${b[*]}" "${n[*]}" <<'PY'
import sys,statistics
w,th,b,n=sys.argv[1],sys.argv[2],list(map(int,sys.argv[3].split())),list(map(int,sys.argv[4].split()))
mb,mn=statistics.median(b),statistics.median(n)
print(f"{w:9} threads={th:7} base={mb/1e6:10.2f}M hooks={mn/1e6:10.2f}M ratio={mn/mb:.4f}")
PY
  done
done
