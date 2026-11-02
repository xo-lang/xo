#!/bin/bash
# alloc-build-mac.sh MODE SPECS NAME SEED_XO OUTTAR (the Mac, run as a HEAVY_CLASS=gate job by alloc-split.sh):
# the build half of alloc-build-remote.sh with no host: it cross-builds the linux/arm64 loadgen and servers
# (xo build --targets=linux/arm64, static, no libc) so host 2's queue never delays host 1's lease (Lead, Lang 3,
# 2026-11-02: 36 min wait for pass2 while host 1 sat idle). SPECS is a comma list of REV[+VAR=VALUE...] of THIS
# checkout; MODE last builds every server from the LAST rev's bench/perf/jitter/xo, own from each rev's tree.
# SEED_XO is the darwin xo of the base release (v0.35.0) that builds each rev's xo and the base server.
# Writes OUTTAR with the layout of alloc-build-remote.sh (bin/{loadgen,srv-base,srv-pK,env-pK}, revs.txt).
set -u
mode=$1 a=$2 name=$3 seed=$4 outtar=$5
top=$(git rev-parse --show-toplevel) || exit 2
W=$(mktemp -d "${TMPDIR:-/tmp}/rvmac.XXXXXX") || exit 2
cleanup() { for d in "$W"/w*; do [ -d "$d" ] && git -C "$top" worktree remove --force "$d" 2>/dev/null; done; rm -rf "$W"; }
trap cleanup EXIT
t0=$(date +%s)
specs=(${a//,/ }) revs=() envs=()
for sp in "${specs[@]}"; do revs+=("${sp%%+*}"); e=""; [ "$sp" != "${sp#*+}" ] && e="${sp#*+}"; envs+=("${e//+/ }"); done
for k in "${!revs[@]}"; do git -C "$top" worktree add -q --detach "$W/w$k" "${revs[$k]}" || exit 2; done
B=$W/bin; mkdir -p "$B"
J=$W/w$((${#revs[@]} - 1))/bench/perf/jitter
( cd "$J/loadgen" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$B/loadgen" . ) || exit 1
"$seed" build --targets=linux/arm64 -o "$B/srv-base" "$J/xo" > "$W/xobuild-base.txt" 2>&1 || { tail -5 "$W/xobuild-base.txt"; exit 1; }
: > "$B/revs.txt"
echo "base $("$seed" version | sed "s/ darwin.*//") (cross-built on the Mac)" >> "$B/revs.txt"
for k in "${!revs[@]}"; do
	T=$W/w$k
	( cd "$T" && XO_SEED=$seed go run ./scripts/xobuild ) > "$W/xobuild$k.txt" 2>&1 || { echo "p$k ${revs[$k]}: xobuild failed: $(tail -3 "$W/xobuild$k.txt" | tr '\n' ' ')"; continue; }
	src=$J/xo; [ "$mode" = own ] && src=$T/bench/perf/jitter/xo
	env ${envs[$k]} "$T/bin/xo" build --targets=linux/arm64 -o "$B/srv-p$k" "$src" > "$W/srv$k.txt" 2>&1 || { echo "p$k: server build failed: $(tail -3 "$W/srv$k.txt" | tr '\n' ' ')"; continue; }
	line="p$k $(git -C "$T" rev-parse --short HEAD) $(git -C "$T" log -1 --format=%cs) built"
	[ -n "${envs[$k]}" ] && line="$line env [${envs[$k]}]"
	echo "$line"; echo "$line" >> "$B/revs.txt"
	echo "${envs[$k]}" > "$B/env-p$k"
	rm -rf "$T/bin"
done
tar -C "$B" -cf "$outtar" . && echo "BUILD-DONE $name $(( $(date +%s) - t0 )) s, $(du -h "$outtar" | cut -f1)"
