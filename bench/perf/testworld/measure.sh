#!/bin/sh
# Measures bench/perf/testworld (the zero-C audit's B14): builds the test
# binary of testworld_test.xo with each backend named, runs each test
# (XO_TEST) 5 times, and prints "<test> <backend> <min instructions>".
# Instructions come from /usr/bin/time -l on darwin (instructions
# retired), from perf stat (instructions:u) on Linux.
#
# usage: bench/perf/testworld/measure.sh <xo binary> <out dir> <backend...>
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT"
for be in "$@"; do
	mkdir -p "$OUT/src-$be"
	cp "$SRC/testworld_test.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && XO_BUILD_DIR="$OUT/build-$be" "$XO" test --no-gc --backend="$be" . >/dev/null)
done
count() {
	if [ "$(uname)" = Darwin ]; then
		XO_TEST=$1 /usr/bin/time -l "$2" >"$OUT/o.txt" 2>"$OUT/t.txt"
		awk '/instructions retired/{print $1}' "$OUT/t.txt"
	else
		XO_TEST=$1 perf stat -x, -e instructions:u -o "$OUT/t.txt" "$2" >"$OUT/o.txt" 2>/dev/null
		awk -F, '/instructions/{print $1}' "$OUT/t.txt"
	fi
}
for t in 0 1 2 3 4 5 6; do
	for be in "$@"; do
		bin=$(find "$OUT/build-$be" -type f -name "*.test" ! -path "*.dSYM/*" | head -1)
		best=""
		for k in 1 2 3 4 5; do
			n=$(count $t "$bin")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
		done
		echo "$t $be $best"
	done
done
