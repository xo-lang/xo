#!/bin/sh
# linux-goldens.sh: build every golden program of internal/llvm with a
# given xo and backend and compare each run with its want.txt
# (scripts/golden-run.sh), where the Go tests cannot run: in a container
# of scripts/docker-linux.sh, or any Linux host with an xo and no Go.
#
#   scripts/linux-goldens.sh XO BACKEND OUTDIR [NAME...]
#
# XO is the xo binary (its std at XO/../../std), BACKEND arm64 or llvm,
# OUTDIR gets the binaries, build logs, and summary.txt (one `ok NAME`,
# `FAIL NAME`, or `BUILD NAME` line each). NAMEs restrict the run to
# those goldens. Exits 1 when any failed.
set -u
xo=$1
backend=$2
out=$3
shift 3
here=$(cd "$(dirname "$0")" && pwd)
goldens=$here/../internal/llvm/testdata/golden
mkdir -p "$out/bin" "$out/log"
: >"$out/summary.txt"
names=$*
if [ -z "$names" ]; then
  names=$(cd "$goldens" && ls)
fi
bad=0
for n in $names; do
  d=$goldens/$n
  [ -f "$d/want.txt" ] || continue
  if ! (cd "$d" && "$xo" build --backend="$backend" -o "$out/bin/$n" .) >"$out/log/$n.build" 2>&1; then
    echo "BUILD $n" | tee -a "$out/summary.txt"
    tail -5 "$out/log/$n.build"
    bad=1
    continue
  fi
  if "$here/golden-run.sh" "$out/bin/$n" "$d" >"$out/log/$n.run" 2>&1; then
    echo "ok $n" >>"$out/summary.txt"
  else
    echo "FAIL $n" | tee -a "$out/summary.txt"
    head -30 "$out/log/$n.run"
    bad=1
  fi
done
echo "$(grep -c '^ok ' "$out/summary.txt") ok, $(grep -c '^FAIL ' "$out/summary.txt") failed, $(grep -c '^BUILD ' "$out/summary.txt") did not build"
exit $bad
