#!/bin/sh
# Measures bench/perf/portable (decisions 0193 and 0195): builds the
# program with each backend named, runs every workload 5 times, and
# prints "<workload> <backend> <min instructions retired> <best us>
# <checksum>". Instructions come from /usr/bin/time -l (darwin).
#
# usage: bench/perf/portable/measure.sh <xo binary> <out dir> <backend...>
# example: bench/perf/portable/measure.sh .tools/bin/xo /tmp/p arm64 llvm
# XO_LLRT_C=ints (or crypto, glob, ...) in the environment builds the C
# path; WORKLOADS overrides the list; "none" is the setup alone (the
# inputs).
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
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/t-$be" .)
done
for w in ${WORKLOADS:-none arith32 arith8 u64 divmod shift conv str_hash unix checked}; do
	for be in "$@"; do
		best=""
		bt=""
		ck=""
		for k in 1 2 3 4 5; do
			/usr/bin/time -l "$OUT/t-$be" "$w" >"$OUT/o.txt" 2>"$OUT/t.txt"
			n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
			t=$(awk '{print $3}' "$OUT/o.txt")
			ck=$(awk '{print $2}' "$OUT/o.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
			if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
		done
		echo "$w $be $best $bt $ck"
	done
done
