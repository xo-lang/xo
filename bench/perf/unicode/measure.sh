#!/bin/sh
# Measures bench/perf/unicode (decision 0180, 0184): builds the program
# with each backend named, runs every workload 5 times, and prints
# "<workload> <backend> <min instructions retired> <best us> <checksum>".
# Instructions come from /usr/bin/time -l (darwin).
#
# usage: bench/perf/unicode/measure.sh <xo binary> <out dir> <backend...>
# example: bench/perf/unicode/measure.sh .tools/bin/xo /tmp/u arm64 llvm
# (XO_UNICODE=c, the C path of decision 0180, went in decision 0198.)
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
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/u-$be" .)
done
for w in lower upper lower_u upper_u eqlower classify runecase quote; do
	for be in "$@"; do
		best=""
		bt=""
		ck=""
		for k in 1 2 3 4 5; do
			/usr/bin/time -l "$OUT/u-$be" "$w" >"$OUT/o.txt" 2>"$OUT/t.txt"
			n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
			t=$(awk '{print $3}' "$OUT/o.txt")
			ck=$(awk '{print $2}' "$OUT/o.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
			if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
		done
		echo "$w $be $best $bt $ck"
	done
done
