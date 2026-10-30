#!/bin/sh
# linux-goldens.sh: build every golden program of internal/llvm with a
# given xo and backend and compare each run with its want.txt
# (scripts/golden-run.sh), where the Go tests cannot run: in a container
# of scripts/docker-linux.sh, or any Linux host with an xo and no Go.
#
#   scripts/linux-goldens.sh XO BACKEND OUTDIR [NAME...]
#
# XO is the xo binary (its std at XO/../../std), BACKEND arm64, amd64 or
# llvm, or default (no --backend: the native one, decision 0270),
# OUTDIR gets the binaries, build logs, and summary.txt (one `ok NAME`,
# `FAIL NAME`, `BUILD NAME`, or `skip NAME` line each). NAMEs restrict the run to
# those goldens. Exits 1 when any failed. XO_GOLDEN_JOBS goldens build
# and run at once (default 1; the Go tests run them in parallel too,
# TestGoldenRun's t.Parallel); summary.txt keeps the names' order.
set -u
if [ "${1:-}" = --one ]; then
  one=1
  shift
else
  one=0
fi
xo=$1
backend=$2
out=$3
shift 3
here=$(cd "$(dirname "$0")" && pwd)
goldens=$here/../internal/llvm/testdata/golden
mkdir -p "$out/bin" "$out/log" "$out/res"
names=$*
if [ -z "$names" ]; then
  names=$(cd "$goldens" && ls)
fi
jobs=${XO_GOLDEN_JOBS:-1}
if [ $one = 0 ] && [ "$jobs" -gt 1 ]; then
  : >"$out/summary.txt"
  # each golden in its own `--one` run: its result line in res/NAME and
  # what it printed in res/NAME.out, gathered in order below
  for n in $names; do echo "$n"; done | xargs -P "$jobs" -I{} sh "$0" --one "$xo" "$backend" "$out" {}
  for n in $names; do
    [ -f "$out/res/$n" ] || continue
    cat "$out/res/$n" >>"$out/summary.txt"
    cat "$out/res/$n.out"
  done
  bad=0
  grep -q -v '^ok \|^skip ' "$out/summary.txt" && bad=1
  echo "$(grep -c '^ok ' "$out/summary.txt") ok, $(grep -c '^FAIL ' "$out/summary.txt") failed, $(grep -c '^BUILD ' "$out/summary.txt") did not build, $(grep -c '^skip ' "$out/summary.txt") skipped"
  exit $bad
fi
if [ $one = 1 ]; then
  sum=$out/res/$names
  exec >"$out/res/$names.out" 2>&1
else
  sum=$out/summary.txt
  : >"$sum"
fi
# Goldens that hang under Docker Desktop's Rosetta (linux/amd64 emulated
# on Apple silicon) and pass on Linux and under qemu-x86_64: many
# threads forking children at once leave every thread waiting on
# Rosetta's own locks (gotchas/rosetta-fork-threads-hang.md). Under
# Rosetta they are skipped (`skip NAME` in summary.txt).
rosetta_hangs="proc_children_load"
rosetta=0
if grep -q /run/rosetta /proc/self/maps 2>/dev/null; then
  rosetta=1
fi
bad=0
for n in $names; do
  d=$goldens/$n
  [ -f "$d/want.txt" ] || continue
  if [ $rosetta = 1 ] && case " $rosetta_hangs " in *" $n "*) true ;; *) false ;; esac; then
    echo "skip $n (hangs under Rosetta)" | tee -a "$sum"
    continue
  fi
  if [ "$backend" = default ]; then set -- ; else set -- --backend="$backend"; fi
  if ! (cd "$d" && "$xo" build "$@" -o "$out/bin/$n" .) >"$out/log/$n.build" 2>&1; then
    echo "BUILD $n" | tee -a "$sum"
    tail -5 "$out/log/$n.build"
    bad=1
    continue
  fi
  if "$here/golden-run.sh" "$out/bin/$n" "$d" >"$out/log/$n.run" 2>&1; then
    echo "ok $n" >>"$sum"
  else
    echo "FAIL $n" | tee -a "$sum"
    head -30 "$out/log/$n.run"
    bad=1
  fi
done
[ $one = 1 ] && exit $bad
echo "$(grep -c '^ok ' "$out/summary.txt") ok, $(grep -c '^FAIL ' "$out/summary.txt") failed, $(grep -c '^BUILD ' "$out/summary.txt") did not build, $(grep -c '^skip ' "$out/summary.txt") skipped"
exit $bad
