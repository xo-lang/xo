#!/bin/sh
# progen-amd64.sh: progen's differential tests on Xo's own amd64 backend
# from a Mac (or any host with Docker): the programs and their Go backend
# builds on the host, the amd64 builds and both runs in a linux/amd64
# container (emulated on arm64), the comparison back on the host.
#
#   scripts/progen-amd64.sh WORK KIND FIRST COUNT [BATCH]
#
# WORK is a host directory (mounted at /work), KIND compile, arith or
# defer, seeds FIRST..FIRST+COUNT-1 in batches of BATCH (default 250).
# Before each batch the free disk must be at least $XO_DISK_FLOOR GB
# (default 40); it waits up to 30 minutes for it, then stops (exit 3).
# The linux/amd64 xo is cross built once into WORK/xo (bin/xo and std/,
# with the Go backend, toolchain.md "Linux in Docker"). The comparison is
# internal/gen TestProgenAmd64Compare: a diverging program is kept in
# $XO_PROP_SAVE (default $TMPDIR/xo-prop-failures)/KIND-amd64-SEED/;
# WORK/KIND-SEED directories of passing batches are removed. The summary
# line of each batch goes to WORK/summary.txt. Docker runs at nice 10
# (toolchain.md: outside the heavy lock).
set -u
work=$1
kind=$2
first=$3
count=$4
batch=${5:-250}
floor=${XO_DISK_FLOOR:-40}
root=$(cd "$(dirname "$0")/.." && pwd)
xo=${XO_CLI:-$root/bin/xo}
mkdir -p "$work"
work=$(cd "$work" && pwd)

if [ ! -x "$work/xo/bin/xo" ]; then
  mkdir -p "$work/xo/bin"
  rm -rf "$work/xo/std" && cp -R "$root/std" "$work/xo/std"
  (cd "$root" && "$xo" build --backend=go --targets=linux/amd64 --std=std -o "$work/xo/bin/xo" compiler/xo) || exit 1
fi

wait_floor() {
  w=0
  while :; do
    f=$(df -g "$work" | awk 'NR==2{print $4}')
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
  bdir="$work/batch-$kind-$s"
  rm -rf "$bdir" && mkdir -p "$bdir"
  export XO_PROGEN_AMD64="$bdir" XO_PROGEN_KIND="$kind" XO_PROGEN_FIRST="$s" XO_PROGEN_N="$n"
  (cd "$root" && go test ./internal/gen -run '^TestProgenAmd64Dump$' -count=1 -timeout 120m) >"$bdir.dump.txt" 2>&1 || { echo "dump failed: $bdir.dump.txt"; exit 1; }
  rel=${bdir#"$work"/}
  nice -n 10 "$root/scripts/docker-linux.sh" amd64 "$work" "/xo/scripts/progen-amd64-run.sh /work/xo/bin/xo /work/$rel" >"$bdir.run.txt" 2>&1
  if (cd "$root" && go test ./internal/gen -run '^TestProgenAmd64Compare$' -count=1 -v) >"$bdir.compare.txt" 2>&1; then
    line="ok $kind $s..$((s + n - 1))"
    rm -rf "$bdir"
  else
    line="FAIL $kind $s..$((s + n - 1)): $(grep -o 'diverge \[.*\]' "$bdir.compare.txt" | tail -1) ($bdir.compare.txt)"
  fi
  echo "$line" | tee -a "$work/summary.txt"
  s=$((s + n))
done
