#!/bin/sh
# Measures bench/perf/report (decision 0193 step 5): builds the test
# binary of report_test.xo with each backend named, runs the cases of each
# test (XO_TEST, XO_PROP_SEED; then with XO_PROP_BEST, as while
# shrinking), and prints "<test> <backend> <min instructions retired of
# 5 runs, summed over the seeds> <bytes of output>". Instructions come
# from /usr/bin/time -l (darwin).
#
# usage: bench/perf/report/measure.sh <xo binary> <out dir> <backend...>
# XO_LLRT_C=report in the environment builds the C path.
# Decision 0198 deleted the C path of report (2026-10-13): naming it in
# A deleted XO_LLRT_C name now fails the build; earlier results files keep the comparisons.
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT"
for be in "$@"; do
	rm -rf "$OUT/src-$be" "$OUT/build-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/report_test.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && XO_BUILD_DIR="$OUT/build-$be" "$XO" test --no-gc --backend="$be" . >/dev/null) || true
done
for t in 0 0b 1 1b 2; do
	for be in "$@"; do
		bin=$(find "$OUT/build-$be" -type f -name "*.test" ! -path "*.dSYM/*" | head -1)
		idx=${t%b}
		sum=0
		bytes=0
		for seed in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16; do
			best=""
			for k in 1 2 3 4 5; do
				if [ "$idx" != "$t" ]; then
					XO_TEST=$idx XO_PROP_SEED=$seed XO_PROP_BEST=0 /usr/bin/time -l "$bin" >"$OUT/o.txt" 2>"$OUT/t.txt" || true
				else
					XO_TEST=$idx XO_PROP_SEED=$seed /usr/bin/time -l "$bin" >"$OUT/o.txt" 2>"$OUT/t.txt" || true
				fi
				n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
				if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
			done
			sum=$((sum + best))
			bytes=$((bytes + $(wc -c <"$OUT/o.txt")))
		done
		echo "$t $be $sum $bytes"
	done
done
