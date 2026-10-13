#!/bin/sh
# Compares the native heap in Xo (decision 0207 stage 3,
# std/native/llrt/alloc.xo) with its C path (XO_LLRT_C=alloc): builds a
# program with both on each backend named in $BACKENDS (default "arm64
# llvm"), runs every workload $REPS times (default 5), and prints
# "<workload> <backend> <xo min instructions> <c min instructions>
# <xo/c> <checksum xo> <checksum c>". Instructions come from
# /usr/bin/time -l (darwin). Run under the heavy lock with
# XO_SELF_DRIVER=0.
# Decision 0198 deleted the C path of alloc (2026-10-13): naming it in
# XO_LLRT_C now builds the Xo path; earlier results files keep the comparisons.
#
# usage: bench/perf/alloc/compare.sh <xo binary> <out dir> <program dir> <workload...>
# example: bench/perf/alloc/compare.sh .tools/bin/xo /tmp/a bench/perf/alloc none small mixed
# A workload of "" runs the program with no argument (bench/perf/conc).
set -e
# the Go driver: a Go `xo` would otherwise build xo1 first (switch point A)
export XO_SELF_DRIVER="${XO_SELF_DRIVER:-0}"
XO=$1
OUT=$2
SRC=$(cd "$3" && pwd)
shift 3
mkdir -p "$OUT"
for be in ${BACKENDS:-arm64 llvm}; do
	rm -rf "$OUT/src"
	mkdir -p "$OUT/src"
	cp "$SRC"/*.xo "$SRC/xo.mod" "$OUT/src/"
	(cd "$OUT/src" && "$XO" build --backend="$be" -o "$OUT/b-$be-xo" . >/dev/null)
	(cd "$OUT/src" && XO_LLRT_C=alloc "$XO" build --backend="$be" -o "$OUT/b-$be-c" . >/dev/null)
done
reps=${REPS:-5}
for w in "$@"; do
	for be in ${BACKENDS:-arm64 llvm}; do
		line="$w $be"
		cks=""
		for p in xo c; do
			best=""
			k=0
			while [ $k -lt "$reps" ]; do
				# shellcheck disable=SC2086
				/usr/bin/time -l "$OUT/b-$be-$p" $w >"$OUT/o.txt" 2>"$OUT/t.txt"
				n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
				if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
				k=$((k + 1))
			done
			line="$line $best"
			cks="$cks $(awk '{print $2}' "$OUT/o.txt")"
			eval "v_$p=$best"
		done
		echo "$line $(awk -v a="$v_xo" -v b="$v_c" 'BEGIN{printf "%.3f", a/b}')$cks"
	done
done
