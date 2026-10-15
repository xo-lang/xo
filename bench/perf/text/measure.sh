#!/bin/sh
# Measures bench/perf/text (decision 0193): builds the program
# with each backend named, runs every workload 5 times, and prints
# "<workload> <backend> <min instructions retired> <best us> <checksum>".
# Instructions come from /usr/bin/time -l on darwin (instructions
# retired), from perf stat (instructions:u) on Linux.
#
# usage: bench/perf/text/measure.sh <xo binary> <out dir> <backend...>
# example: bench/perf/text/measure.sh .tools/bin/xo /tmp/t arm64 llvm
# WORKLOADS overrides the list; "none" is the setup alone (the inputs).
# Decision 0198 deleted the C paths of show, tz, and report (2026-10-13)
# and decision 0227 that of text (2026-10-14): naming them in XO_LLRT_C
# fails the build; earlier results files keep the comparisons.
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
for w in ${WORKLOADS:-none parse_int dur_fmt dur_parse time_fmt time_parse quote split lines replace trim rune_count utf8 int_fmt float_fmt float_parse base64 hex debug zone_offset zone_fmt zone_civil zone_load}; do
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
