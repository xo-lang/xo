#!/bin/bash
# docker-goldens.sh: every golden on Linux in Docker with a Linux xo of
# this checkout (cross built here with the Go backend): the linux/arm64 or
# linux/amd64 run CI did before decision 0225's CI was turned off.
#
#   scripts/docker-goldens.sh DARWIN_XO ARCH BACKEND WORK [NAME...]
#
# DARWIN_XO builds the Linux xo; ARCH arm64 or amd64; BACKEND arm64,
# amd64, or llvm; WORK a scratch directory (WORK/xo the Linux xo and its
# std, WORK/goldens the results, summary.txt there). amd64 runs with no C
# toolchain (XO_CC and XO_CLANG /bin/false: Xo's own assembler and
# linker). Runs outside the heavy lock, niced (toolchain.md "Linux in
# Docker on the Mac"); XO_DOCKER_ROOT=1 as scripts/docker-linux.sh, but
# not for the file system goldens (as root, read only files are writable).
set -eu
xo=$1
arch=$2
be=$3
work=$4
shift 4
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
work=$(cd "$work" && pwd)
rm -rf "$work/xo" "$work/goldens"
mkdir -p "$work/xo/bin"
(cd "$root" && XO_STD="$root/std" "$xo" build --backend=go --targets="linux/$arch" --std=std -o "$work/xo/bin/xo" compiler/xo > "$work/xo-build.txt" 2>&1) || { tail -5 "$work/xo-build.txt"; exit 1; }
cp -R "$root/std" "$work/xo/std"
cc=""
if [ "$arch" = amd64 ]; then
  cc="XO_CC=/bin/false XO_CLANG=/bin/false"
fi
nice -n 10 "$root/scripts/docker-linux.sh" "$arch" "$work" "$cc /xo/scripts/linux-goldens.sh /work/xo/bin/xo $be /work/goldens $*" > "$work/docker.txt" 2>&1 || true
echo "ok $(grep -c '^ok' "$work/goldens/summary.txt") of $(wc -l < "$work/goldens/summary.txt")"
grep -v '^ok' "$work/goldens/summary.txt" || true
