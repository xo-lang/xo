#!/bin/sh
# progen-amd64-run.sh: the linux/amd64 half of scripts/progen-amd64.sh,
# run in a container of scripts/docker-linux.sh amd64 (or on any
# linux/amd64 host): each program directory under DIR (written by
# internal/gen TestProgenAmd64Dump, with its Go backend build prog-go) is
# built with XO --backend=amd64, and both binaries are run; each run's
# exit status, stdout and stderr go to go.* and amd64.* in the program's
# directory, a failed build's output to amd64.build.
#
#   scripts/progen-amd64-run.sh XO DIR
set -u
xo=$1
dir=$2
for d in "$dir"/*/; do
  [ -f "$d/main.xo" ] || continue
  rm -f "$d"/go.* "$d"/amd64.*
  if ! (cd "$d" && XO_BACKEND_CACHE=off XO_BIN_CACHE=off "$xo" build --backend=amd64 -o prog-amd64 .) >"$d/amd64.log" 2>&1; then
    cp "$d/amd64.log" "$d/amd64.build"
  fi
  for b in go amd64; do
    [ -x "$d/prog-$b" ] || continue
    (cd "$d" && XO_CRASH_DIR=/tmp timeout 30 "./prog-$b" >"$b.stdout" 2>"$b.stderr"; echo $? >"$b.exit")
  done
  rm -f "$d/prog-amd64"
done
