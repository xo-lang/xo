#!/bin/sh
# Measures bench/perf/callbacks (decision 0337): builds the program with
# each backend named, runs every workload 5 times, and prints
# "<workload> <backend> <min instructions retired> <best us> <checksum>".
# Instructions come from /usr/bin/time -l (darwin).
#
# usage: bench/perf/callbacks/measure.sh <xo binary> <out dir> <backend...>
# example: bench/perf/callbacks/measure.sh bin/xo /tmp/cb arm64 llvm
# WORKLOADS overrides the list.
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT"
for be in "$@"; do
	rm -rf "$OUT/src-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/cb-$be" .)
done
for w in ${WORKLOADS:-for any filter fold ifor iany ifold two_for two_any two_fold}; do
	for be in "$@"; do
		best=""
		bt=""
		ck=""
		for k in 1 2 3 4 5; do
			/usr/bin/time -l "$OUT/cb-$be" "$w" >"$OUT/o.txt" 2>"$OUT/t.txt"
			n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
			t=$(awk '{print $3}' "$OUT/o.txt")
			ck=$(awk '{print $2}' "$OUT/o.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
			if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
		done
		echo "$w $be $best $bt $ck"
	done
done
