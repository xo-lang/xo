#!/bin/sh
# Measures bench/perf/json (decision 0193): builds the program
# with each backend named, runs every workload 5 times, and prints
# "<workload> <backend> <min instructions retired> <best us> <checksum>".
# Instructions come from /usr/bin/time -l on darwin (instructions
# retired), from perf stat (instructions:u) on Linux.
#
# usage: bench/perf/json/measure.sh <xo binary> <out dir> <backend...>
# example: bench/perf/json/measure.sh .tools/bin/xo /tmp/t arm64 llvm
# (XO_LLRT_C=jsontext, the C path of decision 0193, went in decision 0198.)
# WORKLOADS overrides the list; "none" is the setup alone (the inputs).
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
count() {
	if [ "$(uname)" = Darwin ]; then
		/usr/bin/time -l "$1" "$2" >"$OUT/o.txt" 2>"$OUT/t.txt"
		awk '/instructions retired/{print $1}' "$OUT/t.txt"
	else
		perf stat -x, -e instructions:u -o "$OUT/t.txt" "$1" "$2" >"$OUT/o.txt"
		awk -F, '/instructions/{print $1}' "$OUT/t.txt"
	fi
}
for w in ${WORKLOADS:-none small ints floats strings maps parse invalid enc_small enc_strings enc_floats bytes}; do
	for be in "$@"; do
		best=""
		bt=""
		ck=""
		for k in 1 2 3 4 5; do
			n=$(count "$OUT/t-$be" "$w")
			t=$(awk '{print $3}' "$OUT/o.txt")
			ck=$(awk '{print $2}' "$OUT/o.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
			if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
		done
		echo "$w $be $best $bt $ck"
	done
done
