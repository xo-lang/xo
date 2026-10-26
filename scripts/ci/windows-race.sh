#!/bin/bash
# scripts/ci/windows-race.sh: windows/amd64 --race with threads (decision
# 0273's Windows addendum), the CI run its Wine checks could not be
# (XO_THREADS=1 there; gotchas/tsan-go-flavor-windows.md). For suite=script
# on the xo-windows runner:
#   gh workflow run ci.yml -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/windows-race.sh"
# ThreadSanitizer's Go flavor (race_windows_amd64.syso, scripts/race-runtime.sh
# windows-amd64: MinGW in Docker, which the runner lacks) comes from
# $XO_RACE_RUNTIME, else ci/race/race_windows_amd64.syso of the seed bucket
# ($XO_SEED_BUCKET, the runner's read only instance profile), checked
# against RT_SHA256. Builds internal/llvm/testdata's tsan_race, race_sync and
# race_stacks with --race --backend=amd64 and runs each with XO_THREADS
# unset (one worker per CPU), 4 and 1, REPS times: the planted race found
# (exit 66), every race_sync case clean when synchronized and reported when
# not (lazy_const and mutex_loop clean both ways), race_stacks' 200 isolated faults caught
# and its race reported with its stacks. $XO_CI_OUT/windows-race.txt has a
# line per run; exit 1 on any mismatch. Each run is bounded by
# RUN_TIMEOUT seconds (default 120; a hung race binary otherwise holds the
# job to the workflow's 90 minute limit): a run that outlives it is
# stopped and counted a FAIL with exit=timeout. After MAX_HANGS (3) hung runs
# the rest are skipped (one SKIP line, exit 1): a systematic hang would
# otherwise cost 234 x RUN_TIMEOUT, past the job limit.
#
# --mutex-loop: only tsan_race and race_sync mutex_loop, 20 runs at each thread count (about 15 minutes with the build)
#
# Release mode (lead, 0.27.0): `windows-race.sh ARCHIVE SHA256` tests a
# release archive as release-smoke.sh does (ARCHIVE a path in the
# checkout, else a name under ci/release/ of the seed bucket, checked
# against SHA256): its bin/xo.exe builds the programs, with the
# ThreadSanitizer runtime the archive ships (lib/xo/race), not the
# bucket's, and no XO_STD, XO_CLI or XO_RACE_RUNTIME of the job.
set -uo pipefail
# --no-prune (lead, 2026-10-23): a backend cache cap no cache reaches, so
# no build prunes (separates the prune from the cache's other costs when
# builds are slow; the dispatch cannot set environment variables)
# --fresh-cache (lead, 2026-10-23): the builds use a new, empty backend
# cache under the job's temporary directory, removed with it (a cold
# build's cost, whatever the runner's cache holds)
fresh=0
only=0
while [ $# -gt 0 ]; do
  case $1 in
  --no-prune)
    export XO_BACKEND_CACHE_MAX=64000000000
    echo "windows-race: --no-prune: XO_BACKEND_CACHE_MAX=$XO_BACKEND_CACHE_MAX" ;;
  --fresh-cache) fresh=1 ;;
  # --mutex-loop (2026-10-26): only the mutex_loop case (and the planted race, so a
  # silent detector is told from a clean run), 20 runs at each thread count, the
  # full stderr of a failing run kept: the owner's Windows tester's report #12
  --mutex-loop) only=1 ;;
  *) break ;;
  esac
  shift
done
archive=${1:-}
want_sum=${2:-}
[ -z "$archive" ] || [ -n "$want_sum" ] || { echo "usage: windows-race.sh [--no-prune] [--fresh-cache] [ARCHIVE SHA256]"; exit 2; }
RT_SHA256=cbab2689849964a299312a7d5de5e2c0553c1b27bf90fca5a5f488fcb7826b11
REPS=${REPS:-3}
RUN_TIMEOUT=${RUN_TIMEOUT:-120}
MAX_HANGS=${MAX_HANGS:-3}
hangs=0
# timeout (MSYS coreutils on the runner) exits 124 when it stopped the
# command, 137 when it needed -k's KILL; without it a run is unbounded.
if command -v timeout >/dev/null; then tmo() { timeout -k 10 "$RUN_TIMEOUT" "$@"; }; else
  echo "windows-race: no timeout command, runs are unbounded"; tmo() { "$@"; }; fi
out=${XO_CI_OUT:?XO_CI_OUT}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
# the runtime and the built programs outside XO_CI_OUT: artifacts carry
# text only (ci-out.sh)
. scripts/ci/ci-out.sh
w=$(ci_work windows-race) || exit 2
trap 'rm -rf "${w:?}"' EXIT
if [ $fresh = 1 ]; then
  mkdir -p "$w/fresh-backend"
  export XO_BACKEND_CACHE_DIR
  XO_BACKEND_CACHE_DIR=$(np "$w/fresh-backend")
  echo "windows-race: --fresh-cache: XO_BACKEND_CACHE_DIR=$XO_BACKEND_CACHE_DIR"
fi
if [ -n "$archive" ]; then
  a=$archive
  if [ ! -f "$a" ]; then
    a="$w/$(basename "$archive")"
    aws s3 cp --only-show-errors "s3://${XO_SEED_BUCKET:?XO_SEED_BUCKET}/ci/release/$(basename "$archive")" "$(np "$a")" ||
      { echo "windows-race: $archive is not in the checkout or the bucket (ci/release/)"; exit 2; }
  fi
  got=$(sha256sum "$a" | cut -d' ' -f1)
  [ "$got" = "$want_sum" ] || { echo "windows-race: SHA-256 $got, want $want_sum"; exit 2; }
  mkdir -p "$w/unpack"
  tar -xzf "$a" -C "$w/unpack" || { echo "windows-race: unpack failed"; exit 2; }
  rel=$(echo "$w"/unpack/xo-*)
  unset XO_STD XO_RTPACK XO_RACE_RUNTIME
  export XO_CLI
  XO_CLI=$(np "$rel/bin/xo.exe")
  [ -f "$rel/lib/xo/race/windows-amd64/race_windows_amd64.syso" ] || { echo "windows-race: the archive ships no lib/xo/race/windows-amd64/race_windows_amd64.syso"; exit 2; }
fi
rt=${XO_RACE_RUNTIME:-}
if [ -n "$archive" ]; then
  : # the archive's own runtime, found by its xo
elif [ -z "$rt" ]; then
  rt="$w/race_windows_amd64.syso"
  aws s3 cp --only-show-errors "s3://${XO_SEED_BUCKET:?XO_SEED_BUCKET}/ci/race/race_windows_amd64.syso" "$(np "$rt")" || { echo "windows-race: the runtime is not in the bucket (ci/race/race_windows_amd64.syso)"; exit 2; }
fi
if [ -z "$archive" ]; then
  got=$(sha256sum "$(up "$rt")" | cut -d' ' -f1)
  [ "$got" = "$RT_SHA256" ] || { echo "windows-race: runtime SHA-256 $got, want $RT_SHA256"; exit 2; }
  export XO_RACE_RUNTIME
  XO_RACE_RUNTIME=$(np "$(up "$rt")")
fi
sum="$o/windows-race.txt"
: > "$sum"
# where the time goes (lead, 2026-10-23: main took 44 minutes where the
# 0.27.0 archive took 323 s, and the summary had no times): each build's
# and each run's milliseconds on its line, and the cache's size and its
# shared pack segments before and after the builds
now_ms() { local t=${EPOCHREALTIME/[.,]/}; echo $((t / 1000)); }
cache_note() {
  local c
  if [ -n "${XO_BACKEND_CACHE_DIR:-}" ]; then c=$(up "$XO_BACKEND_CACHE_DIR"); else c=$(up "${LOCALAPPDATA:-$HOME/AppData/Local}")/xo; fi
  if [ -d "$c" ]; then
    echo "cache $1: $(du -sm "$c" 2>/dev/null | cut -f1) MB, $(find "$c" -path '*packs/shared/*' -name 's-*' -type f 2>/dev/null | wc -l | tr -d ' ') shared pack segments" | tee -a "$sum"
  else
    echo "cache $1: no $c" | tee -a "$sum"
  fi
}
cache_note "before the builds"
# each build's cache file operations by area and kind (XO_CACHE_STATS=1)
# and its phases (--phase-times), in the build-*.txt outputs
export XO_CACHE_STATS=1
echo "cap XO_BACKEND_CACHE_MAX=${XO_BACKEND_CACHE_MAX:-default}" >> "$sum"
# which xo, and whether it builds a native program at all without --race:
# separates a broken xo from a --race problem (run 39772751654 failed every
# build with XO1201 and said neither)
"$XO_CLI" version --json | tee -a "$sum"
t0=$(now_ms)
if ! "$XO_CLI" build --phase-times --backend=amd64 -o "$(np "$w/plain_tsan_race.exe")" internal/llvm/testdata/tsan_race > "$o/build-plain.txt" 2>&1; then
  echo "FAIL build tsan_race without --race: $(grep -m 3 ' error ' "$o/build-plain.txt")" | tee -a "$sum"
fi
echo "build plain tsan_race ms=$(($(now_ms) - t0))" | tee -a "$sum"
fail=0
progs="tsan_race race_sync race_stacks"
[ $only = 0 ] || progs="tsan_race race_sync"
for p in $progs; do
  t0=$(now_ms)
  if ! "$XO_CLI" build --phase-times --race --backend=amd64 -o "$(np "$w/$p.exe")" "internal/llvm/testdata/$p" > "$o/build-$p.txt" 2>&1; then
    echo "FAIL build $p: $(tail -3 "$o/build-$p.txt")" | tee -a "$sum"
    fail=1
  fi
  echo "build --race $p ms=$(($(now_ms) - t0))" | tee -a "$sum"
done
cache_note "after the builds"
[ $fail = 0 ] || exit 1
# one run: NAME EXE WANT_EXIT WANT_RACES(+ for at least one, 0 for none) [GREP]
check() {
  local name=$1 exe=$2 want=$3 races=$4 grep=${5:-}
  local t0
  t0=$(now_ms)
  tmo "$w/$exe" > "$o/run.out" 2> "$o/run.err"
  local rc=$? n
  local ms=$(($(now_ms) - t0))
  local hung=0
  if [ "$rc" = 124 ] || [ "$rc" = 137 ]; then hung=1; hangs=$((hangs + 1)); fi
  n=$(grep -c "WARNING: DATA RACE" "$o/run.err")
  local ok=1
  [ $hung = 0 ] || ok=0
  [ "$rc" = "$want" ] || ok=0
  if [ "$races" = 0 ]; then [ "$n" = 0 ] || ok=0; else [ "$n" -ge 1 ] || ok=0; fi
  # (grep the files: with pipefail, cat | grep -q fails on cat's SIGPIPE)
  if [ -n "$grep" ]; then grep -q -- "$grep" "$o/run.out" "$o/run.err" || ok=0; fi
  local line="$name exit=$rc races=$n ms=$ms"
  [ $hung = 0 ] || line="$name exit=timeout(${RUN_TIMEOUT}s) races=$n"
  if [ $ok = 1 ]; then echo "ok   $line" >> "$sum"; else
    echo "FAIL $line (want exit $want, races $races${grep:+, \"$grep\"})" | tee -a "$sum"
    tail -20 "$o/run.err" | sed 's/^/     /' >> "$sum"
    cp "$o/run.err" "$o/fail-$(echo "$name" | tr -c "A-Za-z0-9=" "_").txt"
    fail=1
  fi
  if [ "$hangs" -ge "$MAX_HANGS" ]; then
    echo "SKIP the remaining runs: $hangs timeouts (last: $name)" | tee -a "$sum"
    echo "windows-race: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed, rest skipped"
    exit 1
  fi
}
cases="spawn join scope chan chan_buffered select mutex await_done atomic atomic_add map_grow"
for threads in "" 4 1; do
  if [ -n "$threads" ]; then export XO_THREADS=$threads; else unset XO_THREADS; fi
  t=${threads:-cpus}
  if [ $only = 1 ]; then
    for r in $(seq 1 20); do
      check "t=$t r=$r tsan_race" tsan_race.exe 66 +
      XO_RACE_CASE=mutex_loop XO_RACE_SYNC=1 check "t=$t r=$r race_sync mutex_loop" race_sync.exe 0 0
    done
    continue
  fi
  for r in $(seq 1 "$REPS"); do
    check "t=$t r=$r tsan_race" tsan_race.exe 66 +
    for c in $cases; do
      XO_RACE_CASE=$c XO_RACE_SYNC=1 check "t=$t r=$r race_sync $c sync=1" race_sync.exe 0 0
      XO_RACE_CASE=$c XO_RACE_SYNC=0 check "t=$t r=$r race_sync $c sync=0" race_sync.exe 66 +
    done
    for s in 1 0; do
      XO_RACE_CASE=lazy_const XO_RACE_SYNC=$s check "t=$t r=$r race_sync lazy_const sync=$s" race_sync.exe 0 0
    done
    # a Mutex taken only in a loop (decision 0363's lockish site; the
    # owner's Windows tester, v0.32.0, saw WARNING: DATA RACE every run)
    XO_RACE_CASE=mutex_loop XO_RACE_SYNC=1 check "t=$t r=$r race_sync mutex_loop" race_sync.exe 0 0
    check "t=$t r=$r race_stacks" race_stacks.exe 66 + "faults 200"
    grep -q "touch()" "$o/run.err" && grep -q "created at" "$o/run.err" || { echo "FAIL t=$t r=$r race_stacks: no touch() frame or created at stack" | tee -a "$sum"; fail=1; }
  done
done
ci_offload "$o" "$sum"
echo "windows-race: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed"
exit $fail
