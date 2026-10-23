#!/bin/bash
# scripts/ci/windows-netpoll.sh: decision 0324's Windows stage (workers
# dequeue the I/O completion port), the first run of that path on Windows
# (the Wine checks could not: gotchas/wine-box64-harness.md). Correctness
# and counters only, no timing. For suite=script on the xo-windows runner:
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-windows \
#     -f packages="scripts/ci/windows-netpoll.sh"
# Builds the concurrency, timer and network goldens for windows/amd64 with
# $XO_CLI and runs each through scripts/golden-run.sh (want_windows.txt in
# place of want.txt where one exists) with workers polling (the default)
# and with XO_NETPOLL=0 (the control: the poller thread alone, as before),
# at XO_THREADS unset, 4 and 1, REPS times; then bench/perf/conc's
# pingpong and rwmutex with XO_RTSTATS=1, whose counters (inject,
# wake_push, idle_npblock, poll_return, idle_sleep) go to the summary.
# $XO_CI_OUT/windows-netpoll.txt has a line per run; exit 1 on a FAIL.
# A run stops after RUN_TIMEOUT seconds (default 120; golden-run.sh's
# XO_GOLDEN_TIMEOUT), counted a FAIL; after MAX_HANGS (3) of them the
# rest are skipped.
set -uo pipefail
REPS=${REPS:-2}
export XO_GOLDEN_TIMEOUT=${RUN_TIMEOUT:-120}
MAX_HANGS=${MAX_HANGS:-3}
out=${XO_CI_OUT:?XO_CI_OUT}
np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
up() { if command -v cygpath >/dev/null; then cygpath -u "$1"; else echo "$1"; fi; }
o=$(up "$out")
sum="$o/windows-netpoll.txt"
: > "$sum"
"$XO_CLI" version --json | tee -a "$sum"
goldens="conc_channels conc_mutex conc_select conc_stress conc_reader_yield conc_rwlock_handoff clock_timers http_server http_stream http_client http_wire net_echo"
fail=0
mkdir -p "$o/bin" "$o/gold"
for g in $goldens; do
  d=internal/llvm/testdata/golden/$g
  rm -rf "$o/gold/$g"; cp -R "$d" "$o/gold/$g"
  [ -f "$d/want_windows.txt" ] && cp "$d/want_windows.txt" "$o/gold/$g/want.txt"
  if ! "$XO_CLI" build -o "$(np "$o/bin/$g.exe")" "$d" > "$o/build-$g.txt" 2>&1; then
    echo "FAIL build $g: $(tail -3 "$o/build-$g.txt")" | tee -a "$sum"
    fail=1
  fi
done
# a bench program with counters: NAME DIR
bench() {
  "$XO_CLI" build -o "$(np "$o/bin/$1.exe")" "$2" > "$o/build-$1.txt" 2>&1 || { echo "FAIL build $1: $(tail -3 "$o/build-$1.txt")" | tee -a "$sum"; fail=1; return; }
}
bench pingpong bench/perf/conc/xo/pingpong
bench rwmutex bench/perf/conc/xo/rwmutex
hangs=0
for np_mode in on off; do
  for threads in "" 4 1; do
    if [ -n "$threads" ]; then export XO_THREADS=$threads; else unset XO_THREADS; fi
    if [ $np_mode = off ]; then export XO_NETPOLL=0; else unset XO_NETPOLL; fi
    for r in $(seq 1 "$REPS"); do
      for g in $goldens; do
        [ -f "$o/bin/$g.exe" ] || continue
        res=$(sh scripts/golden-run.sh "$(np "$o/bin/$g.exe")" "$o/gold/$g" 2>&1)
        rc=$?
        if [ $rc = 0 ]; then echo "ok   netpoll=$np_mode t=${threads:-cpus} r=$r $g" >> "$sum"; else
          echo "FAIL netpoll=$np_mode t=${threads:-cpus} r=$r $g" | tee -a "$sum"
          echo "$res" | head -25 | sed 's/^/     /' >> "$sum"
          fail=1
          case "$res" in *"exit 124"*) hangs=$((hangs + 1));; esac
          if [ $hangs -ge "$MAX_HANGS" ]; then echo "SKIP the rest: $hangs hangs" | tee -a "$sum"; exit 1; fi
        fi
      done
    done
  done
done
unset XO_THREADS XO_NETPOLL
if command -v timeout >/dev/null; then tmo() { timeout -k 10 "${RUN_TIMEOUT:-120}" "$@"; }; else tmo() { "$@"; }; fi
# counters, once per mode
for np_mode in on off; do
  if [ $np_mode = off ]; then export XO_NETPOLL=0; else unset XO_NETPOLL; fi
  for b in pingpong rwmutex; do
    [ -f "$o/bin/$b.exe" ] || continue
    XO_RTSTATS=1 tmo "$o/bin/$b.exe" > "$o/$b-$np_mode.out" 2> "$o/$b-$np_mode.err"
    echo "bench $b netpoll=$np_mode exit=$?" >> "$sum"
    grep -E "inject|wake_push|idle_npblock|poll_return|idle_sleep" "$o/$b-$np_mode.err" | sed 's/^/     /' >> "$sum"
  done
done
echo "windows-netpoll: $(grep -c '^ok' "$sum") ok, $(grep -c '^FAIL' "$sum") failed"
exit $fail
