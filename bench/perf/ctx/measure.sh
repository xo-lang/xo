#!/bin/sh
# Measures bench/perf/ctx (decision 0229, B17): builds main.xo with
# each backend named, runs each workload 5 times, and prints "<workload>
# <backend> <min instructions>". Instructions come from /usr/bin/time -l
# on darwin (instructions retired), from perf stat (instructions:u) on
# Linux. The C baseline is the same script run with an xo whose std is
# an earlier commit's (CI's measure suite runs main's and the branch's).
#
# usage: bench/perf/ctx/measure.sh <xo binary> <out dir> <backend...>
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
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/ctx-$be" . >/dev/null)
done
count() {
	if [ "$(uname)" = Darwin ]; then
		/usr/bin/time -l "$2" "$1" >"$OUT/o.txt" 2>"$OUT/t.txt"
		awk '/instructions retired/{print $1}' "$OUT/t.txt"
	else
		perf stat -x, -e instructions:u -o "$OUT/t.txt" "$2" "$1" >"$OUT/o.txt" 2>/dev/null
		awk -F, '/instructions/{print $1}' "$OUT/t.txt"
	fi
}
for w in ${WORKLOADS:-none defer mutex spawn budget await}; do
	for be in "$@"; do
		best=""
		for k in 1 2 3 4 5; do
			n=$(count "$w" "$OUT/ctx-$be")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
		done
		echo "$w $be $best"
	done
done
