#!/bin/sh
# progen-batches.sh: a long run of progen's differential tests on this
# host (internal/gen TestCompileProperty, TestArithProperty,
# TestDeferFaultProperty) in batches, each one heavy run of its own so
# other sessions interleave between batches (lead, 2026-10-18).
#
#   scripts/progen-batches.sh OUT KIND FIRST COUNT [BATCH]
#
# KIND compile, arith or defer; seeds FIRST..FIRST+COUNT-1 in batches of
# BATCH (default 250). Each batch's go test output goes to
# OUT/KIND-FIRSTSEED.txt and one summary line to OUT/summary.txt.
# $XO_HEAVY prefixes each batch's command (scripts/lead/heavy.sh NAME,
# for the machine-wide lock); $XO_DISK_FLOOR GB (default 40) must be free
# before each batch (it waits up to 30 minutes, then stops, exit 3).
# compile batches also build and compare LLVM (XO_COMPILE_PROP_LLVM=1)
# unless XO_COMPILE_PROP_LLVM=0. A failing seed's program is kept and
# triaged by the test itself (internal/gen triage_test.go).
set -u
out=$1
kind=$2
first=$3
count=$4
batch=${5:-250}
floor=${XO_DISK_FLOOR:-40}
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
case $kind in
compile) test=TestCompileProperty ;;
arith) test=TestArithProperty ;;
defer) test=TestDeferFaultProperty ;;
*) echo "progen-batches.sh: KIND is compile, arith or defer"; exit 2 ;;
esac

wait_floor() {
  w=0
  while :; do
    f=$(df -g "$out" | awk 'NR==2{print $4}')
    [ "$f" -ge "$floor" ] && return 0
    [ $w -eq 0 ] && echo "waiting: free disk ${f} GB < ${floor} before seeds $1"
    [ $w -ge 30 ] && { echo "STOP: free disk ${f} GB < ${floor} for 30 minutes before seeds $1"; exit 3; }
    sleep 60
    w=$((w + 1))
  done
}

s=$first
end=$((first + count))
while [ $s -lt $end ]; do
  n=$batch
  [ $((s + n)) -gt $end ] && n=$((end - s))
  wait_floor "$s..$((s + n - 1))"
  log="$out/$kind-$s.txt"
  case $kind in
  compile) env="XO_COMPILE_PROPS=$n XO_COMPILE_PROP_FIRST=$s XO_COMPILE_PROP_LLVM=${XO_COMPILE_PROP_LLVM:-1}" ;;
  arith) env="XO_ARITH_PROPS=$n XO_ARITH_PROP_FIRST=$s" ;;
  defer) env="XO_DEFER_FAULT_PROPS=$n XO_DEFER_FAULT_PROP_FIRST=$s" ;;
  esac
  # shellcheck disable=SC2086
  (cd "$root" && env $env XO_BIN_CACHE=off XO_BACKEND_CACHE=off ${XO_HEAVY:-} go test ./internal/gen -run "^$test\$" -count=1 -timeout 300m -parallel 4) >"$log" 2>&1
  code=$?
  echo "$kind $s..$((s + n - 1)) exit $code fails $(grep -c '^    --- FAIL' "$log") free $(df -g "$out" | awk 'NR==2{print $4}')G" | tee -a "$out/summary.txt"
  s=$((s + n))
done
