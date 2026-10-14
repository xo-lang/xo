#!/bin/sh
# Measures bench/perf/fsproc (decision 0193 step 6): builds the program
# with each backend named, runs every workload 5 times in a fresh scratch
# directory under the out dir, and prints "<workload> <backend> <min
# instructions retired> <checksum>". Instructions come from /usr/bin/time
# -l (darwin).
#
# usage: bench/perf/fsproc/measure.sh <xo binary> <out dir> <backend...>
# XO_LLRT_C=fspath,procspec in the environment builds the C paths; WORKLOADS
# overrides the list.
# Decision 0198 deleted the C path of fspath and procspec (2026-10-13): naming it in
# A deleted XO_LLRT_C name now fails the build; earlier results files keep the comparisons.
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
for w in ${WORKLOADS:-none mkdir tree remove temp cwd proc_plain proc}; do
	for be in "$@"; do
		best=""
		ck=""
		for k in 1 2 3 4 5; do
			rm -rf "$OUT/scratch"
			mkdir -p "$OUT/scratch"
			(cd "$OUT" && /usr/bin/time -l "$OUT/t-$be" "$w" scratch >"$OUT/o.txt" 2>"$OUT/t.txt")
			n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
			ck=$(awk '{print $2}' "$OUT/o.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
		done
		echo "$w $be $best $ck"
	done
done
rm -rf "$OUT/scratch"
