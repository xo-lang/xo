#!/bin/sh
# Measures bench/perf/glob (decision 0195): builds the test binary of
# glob_test.xo with each backend named, runs each test (XO_TEST) 5 times,
# and prints "<test> <backend> <min instructions retired>". Instructions
# come from /usr/bin/time -l (darwin).
#
# usage: bench/perf/glob/measure.sh <xo binary> <out dir> <backend...>
# XO_LLRT_C=glob in the environment builds the C path.
# Decision 0198 deleted the C path of glob (2026-10-13): naming it in
# XO_LLRT_C now builds the Xo path; earlier results files keep the comparisons.
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT"
for be in "$@"; do
	rm -rf "$OUT/src-$be" "$OUT/build-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/glob_test.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && XO_BUILD_DIR="$OUT/build-$be" "$XO" test --no-gc --backend="$be" . >/dev/null)
done
for t in 0 1; do
	for be in "$@"; do
		bin=$(find "$OUT/build-$be" -type f -name "*.test" | head -1)
		best=""
		for k in 1 2 3 4 5; do
			XO_TEST=$t /usr/bin/time -l "$bin" >"$OUT/o.txt" 2>"$OUT/t.txt"
			n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
		done
		echo "$t $be $best"
	done
done
